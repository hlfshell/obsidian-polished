package exporter

import (
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hlfshell/obsidian-polished/internal/uiassets"
	"github.com/hlfshell/obsidian-polished/internal/ziputil"
)

var (
	wikiLinkRE = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	embedRE    = regexp.MustCompile(`!\[\[([^\]]+)\]\]`)
	mdImageRE  = regexp.MustCompile(`(!\[[^\]]*\]\()([^)]+)(\))`)
	mdLinkRE2  = regexp.MustCompile(`(\[[^\]]*\]\()([^)]+)(\))`)
)

var (
	videoExts = map[string]bool{".mp4": true, ".webm": true, ".mov": true, ".m4v": true}
	imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true, ".bmp": true, ".tiff": true, ".ico": true}
)

type ThemeMode string

const (
	ThemeBoth  ThemeMode = "both"
	ThemeLight ThemeMode = "light"
	ThemeDark  ThemeMode = "dark"
)

type Options struct {
	VaultRoot     string
	OutDir        string
	HubIndexPath  string
	RootNote      string
	MaxDepth      int
	ThemeMode     ThemeMode
	CSSPath       string
	Zip           bool
	ZipPath       string
	Incremental   bool
	SkipIndex     bool
	IndexAllNotes bool
}

type Result struct {
	OutputDir     string
	ZipPath       string
	NotesExported int
	AssetsCopied  int
}

type exporter struct {
	vaultRoot string
	outDir    string
	notesOut  string
	assetsOut string
	opts      Options

	mdFiles     []string
	byStem      map[string][]string
	fileByName  map[string][]string
	noteSeen    map[string]bool
	mediaNeeded map[string]bool
	noteDepth   map[string]int
	queue       []string
	gitChecked  bool
	isGitRepo   bool
	createdAt   map[string]time.Time
}

type collectionNode struct {
	segment  string
	relPath  string
	children map[string]*collectionNode
	notes    []string
}

func Run(opts Options) (Result, error) {
	if opts.ThemeMode == "" {
		opts.ThemeMode = ThemeBoth
	}
	if opts.Incremental && opts.Zip {
		return Result{}, fmt.Errorf("zip output is not supported in incremental mode")
	}
	if opts.MaxDepth < -1 {
		return Result{}, fmt.Errorf("max-depth must be -1 or greater")
	}
	if opts.ThemeMode != ThemeBoth && opts.ThemeMode != ThemeLight && opts.ThemeMode != ThemeDark {
		return Result{}, fmt.Errorf("invalid theme mode: %s", opts.ThemeMode)
	}

	vaultRoot, err := filepath.Abs(opts.VaultRoot)
	if err != nil {
		return Result{}, err
	}
	outDir, err := filepath.Abs(opts.OutDir)
	if err != nil {
		return Result{}, err
	}

	e := &exporter{
		vaultRoot:   vaultRoot,
		outDir:      outDir,
		notesOut:    filepath.Join(outDir, "notes"),
		assetsOut:   filepath.Join(outDir, "assets"),
		opts:        opts,
		byStem:      map[string][]string{},
		fileByName:  map[string][]string{},
		noteSeen:    map[string]bool{},
		mediaNeeded: map[string]bool{},
		noteDepth:   map[string]int{},
		createdAt:   map[string]time.Time{},
	}
	if err := e.scanVault(); err != nil {
		return Result{}, err
	}
	if opts.Incremental {
		if err := e.prepareOutputIncremental(); err != nil {
			return Result{}, err
		}
	} else {
		if err := e.prepareOutput(); err != nil {
			return Result{}, err
		}
	}

	if err := e.seedQueue(); err != nil {
		return Result{}, err
	}
	if err := e.processQueue(); err != nil {
		return Result{}, err
	}
	if err := e.copyMedia(); err != nil {
		return Result{}, err
	}
	if err := e.writeStyle(); err != nil {
		return Result{}, err
	}
	if !opts.SkipIndex {
		if err := e.writeIndex(); err != nil {
			return Result{}, err
		}
	}

	res := Result{
		OutputDir:     outDir,
		NotesExported: len(e.noteSeen),
		AssetsCopied:  len(e.mediaNeeded),
	}

	if opts.Zip {
		zipPath := opts.ZipPath
		if zipPath == "" {
			zipPath = outDir + ".zip"
		}
		zipPath, err = filepath.Abs(zipPath)
		if err != nil {
			return Result{}, err
		}
		if err := ziputil.CreateFromDir(outDir, zipPath); err != nil {
			return Result{}, err
		}
		if err := os.RemoveAll(outDir); err != nil {
			return Result{}, err
		}
		res.ZipPath = zipPath
	}

	return res, nil
}

func (e *exporter) scanVault() error {
	return filepath.WalkDir(e.vaultRoot, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(e.vaultRoot, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		relSlash := toSlash(rel)
		if isExcluded(relSlash) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		e.fileByName[strings.ToLower(filepath.Base(relSlash))] = append(e.fileByName[strings.ToLower(filepath.Base(relSlash))], relSlash)

		if strings.EqualFold(filepath.Ext(relSlash), ".md") {
			e.mdFiles = append(e.mdFiles, relSlash)
			e.byStem[strings.ToLower(strings.TrimSuffix(filepath.Base(relSlash), filepath.Ext(relSlash)))] = append(e.byStem[strings.ToLower(strings.TrimSuffix(filepath.Base(relSlash), filepath.Ext(relSlash)))], relSlash)
		}
		return nil
	})
}

func (e *exporter) prepareOutput() error {
	if err := os.RemoveAll(e.outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(e.notesOut, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(e.assetsOut, 0o755)
}

func (e *exporter) prepareOutputIncremental() error {
	if err := os.MkdirAll(e.notesOut, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(e.assetsOut, 0o755)
}

func (e *exporter) seedQueue() error {
	if e.opts.RootNote == "" {
		sorted := append([]string(nil), e.mdFiles...)
		sort.Strings(sorted)
		for _, note := range sorted {
			e.enqueue(note, 0)
		}
		return nil
	}

	root := e.opts.RootNote
	if filepath.Ext(root) == "" {
		root += ".md"
	}
	root = toSlash(root)
	if strings.HasPrefix(root, "/") {
		rel, err := filepath.Rel(e.vaultRoot, root)
		if err != nil {
			return err
		}
		root = toSlash(rel)
	}

	resolved := e.resolveNote(root, root)
	if resolved == "" {
		return fmt.Errorf("root note not found: %s", e.opts.RootNote)
	}
	e.enqueue(resolved, 0)
	return nil
}

func (e *exporter) processQueue() error {
	for len(e.queue) > 0 {
		note := e.queue[0]
		e.queue = e.queue[1:]
		if err := e.renderNote(note); err != nil {
			return err
		}
	}
	return nil
}

func (e *exporter) enqueue(note string, depth int) {
	if e.noteSeen[note] {
		return
	}
	e.noteSeen[note] = true
	e.noteDepth[note] = depth
	e.queue = append(e.queue, note)
}

func (e *exporter) canFollow(current string) bool {
	if e.opts.MaxDepth < 0 {
		return true
	}
	return e.noteDepth[current] < e.opts.MaxDepth
}

func (e *exporter) renderNote(relNote string) error {
	notePath := filepath.Join(e.vaultRoot, filepath.FromSlash(relNote))
	raw, err := os.ReadFile(notePath)
	if err != nil {
		return err
	}
	info, err := os.Stat(notePath)
	if err != nil {
		return err
	}
	processed := e.processMarkdown(string(raw), relNote)
	rendered := renderMarkdown(processed)

	outPath := e.noteHTMLPath(relNote)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	title := strings.TrimSuffix(filepath.Base(relNote), filepath.Ext(relNote))
	updatedAt := info.ModTime()
	createdAt := e.resolveCreatedAt(relNote, updatedAt)
	meta := fmt.Sprintf(`<div class="note-meta"><h1>%s</h1><div class="meta-row"><span>Created %s</span><span>Updated %s</span></div></div>`,
		html.EscapeString(title),
		html.EscapeString(formatTime(createdAt)),
		html.EscapeString(formatTime(updatedAt)),
	)
	breadcrumbs := e.breadcrumbsHTML(relNote, outPath, title)
	page := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  %s
  <link rel="stylesheet" href="%s">
</head>
<body>
  <header>
    <div class="container">
%s
%s
    </div>
  </header>
  <main class="container prose">
%s
%s
  </main>
  %s
</body>
</html>
`, html.EscapeString(title), e.themeInitScript(), e.relHref(outPath, filepath.Join(e.outDir, "style.css")), e.brandLogoHTML(outPath), breadcrumbs, meta, rendered, e.themeToggleScript())

	return os.WriteFile(outPath, []byte(page), 0o644)
}

func (e *exporter) processMarkdown(raw, current string) string {
	text := embedRE.ReplaceAllStringFunc(raw, func(m string) string {
		parts := embedRE.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		target, alias, _ := parseLinkSpec(parts[1])
		note := e.resolveNote(target, current)
		if note != "" {
			if e.canFollow(current) {
				e.enqueue(note, e.noteDepth[current]+1)
			}
			label := alias
			if label == "" {
				label = strings.TrimSuffix(filepath.Base(note), filepath.Ext(note))
			}
			href := e.relHref(e.noteHTMLPath(current), e.noteHTMLPath(note))
			return fmt.Sprintf(`<div class="embed-note">Embedded note: <a href="%s">%s</a></div>`, href, html.EscapeString(label))
		}
		asset := e.resolveAsset(target, current)
		if asset == "" {
			return fmt.Sprintf(`<span class="missing">[Missing embed: %s]</span>`, html.EscapeString(parts[1]))
		}
		e.mediaNeeded[asset] = true
		ext := strings.ToLower(filepath.Ext(asset))
		href := e.relHref(e.noteHTMLPath(current), e.assetOutPath(asset))
		label := alias
		if label == "" {
			label = filepath.Base(asset)
		}
		if imageExts[ext] {
			return fmt.Sprintf(`<figure><img src="%s" alt="%s" loading="lazy"></figure>`, href, html.EscapeString(label))
		}
		if videoExts[ext] {
			return fmt.Sprintf(`<figure><video controls preload="metadata"><source src="%s"></video><figcaption>%s</figcaption></figure>`, href, html.EscapeString(label))
		}
		if ext == ".pdf" {
			return fmt.Sprintf(`<div class="pdf-embed"><a href="%s" target="_blank" rel="noopener">Open PDF: %s</a></div>`, href, html.EscapeString(label))
		}
		return fmt.Sprintf(`<a href="%s">%s</a>`, href, html.EscapeString(label))
	})

	text = wikiLinkRE.ReplaceAllStringFunc(text, func(m string) string {
		parts := wikiLinkRE.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		target, alias, anchor := parseLinkSpec(parts[1])
		note := e.resolveNote(target, current)
		if note != "" {
			if e.canFollow(current) {
				e.enqueue(note, e.noteDepth[current]+1)
			}
			href := e.relHref(e.noteHTMLPath(current), e.noteHTMLPath(note))
			if anchor != "" {
				href += "#" + anchorSlug(anchor)
			}
			label := alias
			if label == "" {
				if anchor != "" {
					label = anchor
				} else {
					label = strings.TrimSuffix(filepath.Base(note), filepath.Ext(note))
				}
			}
			return fmt.Sprintf("[%s](%s)", label, href)
		}

		asset := e.resolveAsset(target, current)
		if asset != "" {
			e.mediaNeeded[asset] = true
			href := e.relHref(e.noteHTMLPath(current), e.assetOutPath(asset))
			label := alias
			if label == "" {
				label = filepath.Base(asset)
			}
			return fmt.Sprintf("[%s](%s)", label, href)
		}

		return fmt.Sprintf("`[[%s]]`", parts[1])
	})

	rewrite := func(urlText string, current string) string {
		clean := strings.Fields(strings.TrimSpace(urlText))
		if len(clean) == 0 {
			return urlText
		}
		target := clean[0]
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") {
			return urlText
		}
		asset := e.resolveAsset(target, current)
		if asset == "" {
			return urlText
		}
		e.mediaNeeded[asset] = true
		return e.relHref(e.noteHTMLPath(current), e.assetOutPath(asset))
	}

	text = mdImageRE.ReplaceAllStringFunc(text, func(m string) string {
		p := mdImageRE.FindStringSubmatch(m)
		if len(p) != 4 {
			return m
		}
		return p[1] + rewrite(p[2], current) + p[3]
	})
	text = mdLinkRE2.ReplaceAllStringFunc(text, func(m string) string {
		p := mdLinkRE2.FindStringSubmatch(m)
		if len(p) != 4 {
			return m
		}
		return p[1] + rewrite(p[2], current) + p[3]
	})

	return text
}

func parseLinkSpec(spec string) (target, alias, anchor string) {
	main := strings.TrimSpace(spec)
	if pipe := strings.Index(main, "|"); pipe >= 0 {
		alias = strings.TrimSpace(main[pipe+1:])
		main = strings.TrimSpace(main[:pipe])
	}
	if hash := strings.Index(main, "#"); hash >= 0 {
		anchor = strings.TrimSpace(main[hash+1:])
		main = strings.TrimSpace(main[:hash])
	}
	return main, alias, anchor
}

func (e *exporter) resolveNote(target, current string) string {
	if strings.TrimSpace(target) == "" {
		return ""
	}
	target = toSlash(target)
	currentDir := path.Dir(toSlash(current))
	if currentDir == "." {
		currentDir = ""
	}

	candidates := make([]string, 0, 4)
	explicit := path.Clean(path.Join(currentDir, target))
	if !strings.HasSuffix(strings.ToLower(explicit), ".md") {
		candidates = append(candidates, explicit+".md")
	}
	candidates = append(candidates, explicit)

	rootExplicit := path.Clean(target)
	if !strings.HasSuffix(strings.ToLower(rootExplicit), ".md") {
		candidates = append(candidates, rootExplicit+".md")
	}
	candidates = append(candidates, rootExplicit)

	for _, c := range candidates {
		if contains(e.mdFiles, c) {
			return c
		}
	}

	stem := strings.ToLower(strings.TrimSuffix(path.Base(target), path.Ext(target)))
	matches := e.byStem[stem]
	if len(matches) == 0 {
		return ""
	}
	if len(matches) == 1 {
		return matches[0]
	}
	for _, m := range matches {
		if path.Dir(m) == currentDir {
			return m
		}
	}
	return matches[0]
}

func (e *exporter) resolveAsset(target, current string) string {
	if strings.TrimSpace(target) == "" {
		return ""
	}
	target = toSlash(target)
	currentDir := path.Dir(toSlash(current))
	if currentDir == "." {
		currentDir = ""
	}
	candidates := []string{path.Clean(path.Join(currentDir, target)), path.Clean(target)}
	for _, c := range candidates {
		if fileExists(filepath.Join(e.vaultRoot, filepath.FromSlash(c))) {
			return c
		}
	}

	name := strings.ToLower(path.Base(target))
	matches := e.fileByName[name]
	if len(matches) == 0 {
		return ""
	}
	if len(matches) == 1 {
		return matches[0]
	}
	for _, m := range matches {
		if strings.HasPrefix(m, "assets/") {
			return m
		}
	}
	return matches[0]
}

func (e *exporter) noteHTMLPath(relNote string) string {
	base := strings.TrimSuffix(relNote, filepath.Ext(relNote)) + ".html"
	return filepath.Join(e.notesOut, filepath.FromSlash(base))
}

func (e *exporter) assetOutPath(relAsset string) string {
	return filepath.Join(e.assetsOut, filepath.FromSlash(relAsset))
}

func (e *exporter) relHref(fromPath, toPath string) string {
	rel, err := filepath.Rel(filepath.Dir(fromPath), toPath)
	if err != nil {
		return "#"
	}
	parts := strings.Split(toSlash(rel), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func anchorSlug(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	lastDash := false
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func (e *exporter) copyMedia() error {
	assets := make([]string, 0, len(e.mediaNeeded))
	for a := range e.mediaNeeded {
		assets = append(assets, a)
	}
	sort.Strings(assets)
	for _, rel := range assets {
		src := filepath.Join(e.vaultRoot, filepath.FromSlash(rel))
		dst := e.assetOutPath(rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func (e *exporter) writeStyle() error {
	stylePath := filepath.Join(e.outDir, "style.css")
	brandingDir := filepath.Join(e.outDir, "assets", "branding")
	if err := os.MkdirAll(brandingDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(brandingDir, "logo.png"), uiassets.LogoPNG(), 0o644); err != nil {
		return err
	}
	if e.opts.CSSPath != "" {
		data, err := os.ReadFile(e.opts.CSSPath)
		if err != nil {
			return err
		}
		return os.WriteFile(stylePath, data, 0o644)
	}
	return os.WriteFile(stylePath, []byte(uiassets.NoteCSS()+"\n"), 0o644)
}

func (e *exporter) writeIndex() error {
	notes := make([]string, 0, len(e.noteSeen))
	if e.opts.IndexAllNotes {
		notes = append(notes, e.mdFiles...)
	} else {
		for n := range e.noteSeen {
			notes = append(notes, n)
		}
	}
	sort.Strings(notes)
	tree := buildCollectionTree(notes)
	if err := e.writeCollectionPage(tree, filepath.Join(e.outDir, "index.html")); err != nil {
		return err
	}
	return e.writeNestedCollectionPages(tree)
}

func buildCollectionTree(notes []string) *collectionNode {
	root := &collectionNode{
		children: map[string]*collectionNode{},
	}
	for _, note := range notes {
		dir := path.Dir(toSlash(note))
		if dir == "." {
			dir = ""
		}
		node := root
		acc := ""
		if dir != "" {
			for _, seg := range strings.Split(dir, "/") {
				if seg == "" || seg == "." {
					continue
				}
				if acc == "" {
					acc = seg
				} else {
					acc = path.Join(acc, seg)
				}
				next, ok := node.children[seg]
				if !ok {
					next = &collectionNode{
						segment:  seg,
						relPath:  acc,
						children: map[string]*collectionNode{},
					}
					node.children[seg] = next
				}
				node = next
			}
		}
		node.notes = append(node.notes, note)
	}
	return root
}

func (e *exporter) writeNestedCollectionPages(root *collectionNode) error {
	var walk func(node *collectionNode) error
	walk = func(node *collectionNode) error {
		keys := sortedChildKeys(node.children)
		for _, key := range keys {
			child := node.children[key]
			outPath := e.collectionIndexPath(child.relPath)
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return err
			}
			if err := e.writeCollectionPage(child, outPath); err != nil {
				return err
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

func (e *exporter) writeCollectionPage(node *collectionNode, outPath string) error {
	var cards strings.Builder
	keys := sortedChildKeys(node.children)
	for _, key := range keys {
		child := node.children[key]
		href := e.relHref(outPath, e.collectionIndexPath(child.relPath))
		cards.WriteString(fmt.Sprintf(`<a class="note-card folder-card" href="%s"><strong>%s</strong><span>%s/</span></a>`,
			href,
			html.EscapeString(displayName(key)),
			html.EscapeString(child.relPath),
		))
	}

	if len(node.notes) > 0 {
		sort.Strings(node.notes)
		for _, n := range node.notes {
			title := strings.TrimSuffix(filepath.Base(n), filepath.Ext(n))
			href := e.relHref(outPath, e.noteHTMLPath(n))
			cards.WriteString(fmt.Sprintf(`<a class="note-card" href="%s"><strong>%s</strong><span>%s</span></a>`,
				href,
				html.EscapeString(title),
				html.EscapeString(n),
			))
		}
	}

	pageTitle := "Vault Home"
	if node.relPath != "" {
		pageTitle = "Folder: " + displayName(node.segment)
	}
	breadcrumbs := e.collectionBreadcrumbHTML(node.relPath, outPath)

	page := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  %s
  <link rel="stylesheet" href="%s">
</head>
<body>
  <header>
    <div class="container">
%s
%s
    </div>
  </header>
  <main class="container landing">
    <h1>%s</h1>
    <section class="card-grid">%s</section>
  </main>
  %s
</body>
</html>
`, html.EscapeString(pageTitle), e.themeInitScript(), e.relHref(outPath, filepath.Join(e.outDir, "style.css")), e.brandLogoHTML(outPath), breadcrumbs, html.EscapeString(pageTitle), cards.String(), e.themeToggleScript())

	return os.WriteFile(outPath, []byte(page), 0o644)
}

func (e *exporter) collectionIndexPath(relFolder string) string {
	relFolder = strings.Trim(toSlash(relFolder), "/")
	if relFolder == "" {
		return filepath.Join(e.outDir, "index.html")
	}
	return filepath.Join(e.outDir, "collections", filepath.FromSlash(relFolder), "index.html")
}

func (e *exporter) collectionBreadcrumbHTML(relFolder, outPath string) string {
	var items []string

	if strings.TrimSpace(e.opts.HubIndexPath) != "" {
		hubHref := e.relHref(outPath, e.opts.HubIndexPath)
		items = append(items, fmt.Sprintf(`<a class="crumb" href="%s" title="Notebooks">📚</a>`, hubHref))
	}
	homeHref := e.relHref(outPath, filepath.Join(e.outDir, "index.html"))
	items = append(items, fmt.Sprintf(`<a class="crumb home" href="%s" title="Home">🏠</a>`, homeHref))

	acc := ""
	parts := strings.Split(strings.Trim(toSlash(relFolder), "/"), "/")
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		if acc == "" {
			acc = p
		} else {
			acc = path.Join(acc, p)
		}
		href := e.relHref(outPath, e.collectionIndexPath(acc))
		items = append(items, fmt.Sprintf(`<a class="crumb" href="%s">%s</a>`, href, html.EscapeString(displayName(p))))
	}
	return `<nav class="breadcrumbs">` + strings.Join(items, `<span class="sep">|</span>`) + `</nav>`
}

func (e *exporter) breadcrumbsHTML(relNote, outPath, noteTitle string) string {
	var items []string
	if strings.TrimSpace(e.opts.HubIndexPath) != "" {
		hubHref := e.relHref(outPath, e.opts.HubIndexPath)
		items = append(items, fmt.Sprintf(`<a class="crumb" href="%s" title="Notebooks">📚</a>`, hubHref))
	}
	homeHref := e.relHref(outPath, filepath.Join(e.outDir, "index.html"))
	items = append(items, fmt.Sprintf(`<a class="crumb home" href="%s" title="Home">🏠</a>`, homeHref))

	dir := path.Dir(toSlash(relNote))
	if dir != "." && dir != "" {
		acc := ""
		parts := strings.Split(dir, "/")
		for _, p := range parts {
			if p == "" || p == "." {
				continue
			}
			if acc == "" {
				acc = p
			} else {
				acc = path.Join(acc, p)
			}
			href := e.relHref(outPath, e.collectionIndexPath(acc))
			items = append(items, fmt.Sprintf(`<a class="crumb" href="%s">%s</a>`, href, html.EscapeString(displayName(p))))
		}
	}
	items = append(items, fmt.Sprintf(`<span class="crumb current">%s</span>`, html.EscapeString(noteTitle)))
	return `<nav class="breadcrumbs">` + strings.Join(items, `<span class="sep">|</span>`) + `</nav>`
}

func sortedChildKeys(children map[string]*collectionNode) []string {
	keys := make([]string, 0, len(children))
	for k := range children {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})
	return keys
}

func displayName(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	parts := strings.Fields(s)
	for i := range parts {
		r := []rune(parts[i])
		if len(r) == 0 {
			continue
		}
		first := strings.ToUpper(string(r[0]))
		rest := ""
		if len(r) > 1 {
			rest = strings.ToLower(string(r[1:]))
		}
		parts[i] = first + rest
	}
	if len(parts) == 0 {
		return s
	}
	return strings.Join(parts, " ")
}

func (e *exporter) resolveCreatedAt(relNote string, fallback time.Time) time.Time {
	if v, ok := e.createdAt[relNote]; ok {
		return v
	}
	if !e.gitChecked {
		e.gitChecked = true
		cmd := exec.Command("git", "-C", e.vaultRoot, "rev-parse", "--is-inside-work-tree")
		out, err := cmd.Output()
		e.isGitRepo = err == nil && strings.TrimSpace(string(out)) == "true"
	}
	if !e.isGitRepo {
		e.createdAt[relNote] = fallback
		return fallback
	}
	cmd := exec.Command("git", "-C", e.vaultRoot, "log", "--diff-filter=A", "--follow", "--format=%aI", "--", filepath.FromSlash(relNote))
	out, err := cmd.Output()
	if err != nil {
		e.createdAt[relNote] = fallback
		return fallback
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		e.createdAt[relNote] = fallback
		return fallback
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[len(lines)-1]))
	if err != nil {
		e.createdAt[relNote] = fallback
		return fallback
	}
	e.createdAt[relNote] = t
	return t
}

func formatTime(t time.Time) string {
	return t.Local().Format("Jan 2, 2006 3:04 PM MST")
}

func (e *exporter) themeInitScript() string {
	mode := string(e.opts.ThemeMode)
	if mode == "" {
		mode = string(ThemeBoth)
	}
	if mode == string(ThemeBoth) {
		return `<script>(function(){var t=localStorage.getItem('theme');if(!t){t=window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light';}document.documentElement.setAttribute('data-theme',t);})();</script>`
	}
	return fmt.Sprintf(`<script>document.documentElement.setAttribute('data-theme','%s');</script>`, html.EscapeString(mode))
}

func (e *exporter) themeToggleScript() string {
	if e.opts.ThemeMode != ThemeBoth {
		return ""
	}
	return `<button id="theme-toggle" class="theme-toggle" aria-label="Toggle theme"></button>
<script>(function(){var b=document.getElementById('theme-toggle');if(!b){return;}function icon(){var t=document.documentElement.getAttribute('data-theme');b.textContent=t==='dark'?'☀':'🌙';}icon();b.addEventListener('click',function(){var t=document.documentElement.getAttribute('data-theme')==='dark'?'light':'dark';document.documentElement.setAttribute('data-theme',t);localStorage.setItem('theme',t);icon();});})();</script>`
}

func (e *exporter) brandLogoHTML(outPath string) string {
	href := e.relHref(outPath, filepath.Join(e.outDir, "index.html"))
	src := e.relHref(outPath, filepath.Join(e.outDir, "assets", "branding", "logo.png"))
	return fmt.Sprintf(`<a class="brand-mark" href="%s" title="Home"><img src="%s" alt="obsidian-polished logo"></a>`, href, src)
}

func isExcluded(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, p := range parts {
		if p == ".git" || p == ".obsidian" || p == "tmp" {
			return true
		}
	}
	return false
}

func toSlash(p string) string {
	return strings.ReplaceAll(filepath.Clean(p), "\\", "/")
}

func contains(items []string, v string) bool {
	for _, i := range items {
		if i == v {
			return true
		}
	}
	return false
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
