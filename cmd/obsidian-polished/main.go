package main

import (
	"errors"
	"flag"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hlfshell/obsidian-polished/internal/exporter"
	"github.com/hlfshell/obsidian-polished/internal/watcher"
	"gopkg.in/yaml.v3"
)

type stringOpt struct {
	value string
	set   bool
}

func (o *stringOpt) String() string { return o.value }
func (o *stringOpt) Set(v string) error {
	o.value = v
	o.set = true
	return nil
}

type stringListOpt struct {
	values []string
}

func (o *stringListOpt) String() string { return strings.Join(o.values, ",") }
func (o *stringListOpt) Set(v string) error {
	o.values = append(o.values, v)
	return nil
}

type intOpt struct {
	value int
	set   bool
}

func (o *intOpt) String() string { return fmt.Sprintf("%d", o.value) }
func (o *intOpt) Set(v string) error {
	var x int
	if _, err := fmt.Sscanf(v, "%d", &x); err != nil {
		return fmt.Errorf("invalid int %q", v)
	}
	o.value = x
	o.set = true
	return nil
}

type boolOpt struct {
	value bool
	set   bool
}

func (o *boolOpt) String() string {
	if o.value {
		return "true"
	}
	return "false"
}
func (o *boolOpt) Set(v string) error {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		o.value = true
	case "0", "false", "no", "off":
		o.value = false
	default:
		return fmt.Errorf("invalid bool %q", v)
	}
	o.set = true
	return nil
}
func (o *boolOpt) IsBoolFlag() bool { return true }

type Settings struct {
	Name            string             `yaml:"name"`
	Vault           string             `yaml:"vault"`
	RootNote        string             `yaml:"root_note"`
	Image           string             `yaml:"image"`
	GitRepo         string             `yaml:"git_repo"`
	GitBranch       string             `yaml:"git_branch"`
	GitRemote       string             `yaml:"git_remote"`
	WatchGitPull    *bool              `yaml:"watch_git_pull"`
	GitPullInterval string             `yaml:"git_pull_interval"`
	Out             string             `yaml:"out"`
	Theme           string             `yaml:"theme"`
	CSS             string             `yaml:"css"`
	MaxDepth        *int               `yaml:"max_depth"`
	Watch           *bool              `yaml:"watch"`
	WatchPoll       string             `yaml:"watch_poll"`
	WatchDebounce   string             `yaml:"watch_debounce"`
	CacheDir        string             `yaml:"cache_dir"`
	Notebooks       []NotebookSettings `yaml:"notebooks"`
}

type NotebookSettings struct {
	Name            string `yaml:"name"`
	Vault           string `yaml:"vault"`
	RootNote        string `yaml:"root_note"`
	Image           string `yaml:"image"`
	GitRepo         string `yaml:"git_repo"`
	GitBranch       string `yaml:"git_branch"`
	GitRemote       string `yaml:"git_remote"`
	WatchGitPull    *bool  `yaml:"watch_git_pull"`
	GitPullInterval string `yaml:"git_pull_interval"`
	Theme           string `yaml:"theme"`
	CSS             string `yaml:"css"`
	MaxDepth        *int   `yaml:"max_depth"`
}

type notebookRuntime struct {
	slug            string
	name            string
	vault           string
	rootNote        string
	image           string
	gitRepo         string
	gitBranch       string
	gitRemote       string
	gitPullInterval time.Duration
	watchGitPull    bool
	theme           exporter.ThemeMode
	css             string
	maxDepth        int
	outDir          string
	indexHref       string
}

type runConfig struct {
	outRoot      string
	cacheDir     string
	watch        bool
	watchPoll    time.Duration
	watchDebonce time.Duration
	zip          bool
	zipPath      string
	notebooks    []notebookRuntime
	configDir    string
}

func main() {
	if len(os.Args) == 1 {
		printUsage()
		return
	}

	cfgArg, args := extractConfigArg(os.Args[1:])

	var (
		fConfig          = stringOpt{}
		fVaults          = stringListOpt{}
		fRoot            = stringOpt{}
		fOut             = stringOpt{value: "./html_export"}
		fMaxDepth        = intOpt{value: -1}
		fTheme           = stringOpt{value: string(exporter.ThemeBoth)}
		fCSS             = stringOpt{}
		fZip             = boolOpt{}
		fZipPath         = stringOpt{}
		fWatch           = boolOpt{}
		fWatchPoll       = stringOpt{value: "2s"}
		fWatchDebounce   = stringOpt{value: "1s"}
		fWatchGitPull    = boolOpt{}
		fWatchGitPullInt = stringOpt{value: "5m"}
		fWatchGitBranch  = stringOpt{}
		fWatchGitRemote  = stringOpt{value: "origin"}
		fHelp            = boolOpt{}
	)

	fs := flag.NewFlagSet("obsidian-polished", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Var(&fConfig, "config", "Settings YAML file path")
	fs.Var(&fVaults, "vault", "Obsidian vault root (repeat for multiple notebooks)")
	fs.Var(&fRoot, "root-note", "Root note to export (optional)")
	fs.Var(&fOut, "out", "Output directory")
	fs.Var(&fMaxDepth, "max-depth", "Maximum link depth from root note (-1 means unlimited)")
	fs.Var(&fTheme, "theme", "Theme mode: both|light|dark")
	fs.Var(&fCSS, "css", "Optional CSS file override")
	fs.Var(&fZip, "zip", "Output as zip archive instead of folder")
	fs.Var(&fZipPath, "zip-path", "Destination zip file path (default: <out>.zip)")
	fs.Var(&fWatch, "watch", "Watch vault and regenerate on changes")
	fs.Var(&fWatchPoll, "watch-poll", "Watch polling interval (e.g. 2s)")
	fs.Var(&fWatchDebounce, "watch-debounce", "Watch debounce interval")
	fs.Var(&fWatchGitPull, "watch-git-pull", "Enable periodic git sync")
	fs.Var(&fWatchGitPullInt, "watch-git-pull-interval", "Git sync interval")
	fs.Var(&fWatchGitBranch, "watch-git-branch", "Git branch to sync (default main/master)")
	fs.Var(&fWatchGitRemote, "watch-git-remote", "Git remote name")
	fs.Var(&fHelp, "h", "Show help")
	fs.Var(&fHelp, "help", "Show help")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage()
			return
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if fHelp.value {
		printUsage()
		return
	}

	if cfgArg == "" && fConfig.set {
		cfgArg = fConfig.value
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected positional argument: %s\n", fs.Arg(0))
		os.Exit(1)
	}

	settings := Settings{}
	configDir := ""
	if cfgArg != "" {
		var err error
		settings, configDir, err = loadSettings(cfgArg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	rc, err := buildRunConfig(settings, configDir, flagOverrides{
		vaults:          fVaults.values,
		rootNote:        fRoot,
		out:             fOut,
		maxDepth:        fMaxDepth,
		theme:           fTheme,
		css:             fCSS,
		zip:             fZip,
		zipPath:         fZipPath,
		watch:           fWatch,
		watchPoll:       fWatchPoll,
		watchDebounce:   fWatchDebounce,
		watchGitPull:    fWatchGitPull,
		gitPullInterval: fWatchGitPullInt,
		gitBranch:       fWatchGitBranch,
		gitRemote:       fWatchGitRemote,
	}, cfgArg != "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	printBanner(rc)
	if err := prepareGitNotebooks(&rc); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(rc.notebooks) > 1 {
		if err := writeHubIndex(rc); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	if rc.watch {
		if err := runWatch(rc); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runOnce(rc); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type flagOverrides struct {
	vaults          []string
	rootNote        stringOpt
	out             stringOpt
	maxDepth        intOpt
	theme           stringOpt
	css             stringOpt
	zip             boolOpt
	zipPath         stringOpt
	watch           boolOpt
	watchPoll       stringOpt
	watchDebounce   stringOpt
	watchGitPull    boolOpt
	gitPullInterval stringOpt
	gitBranch       stringOpt
	gitRemote       stringOpt
}

func buildRunConfig(settings Settings, configDir string, fo flagOverrides, fromConfig bool) (runConfig, error) {
	theme := string(exporter.ThemeBoth)
	if settings.Theme != "" {
		theme = settings.Theme
	}
	if fo.theme.set {
		theme = fo.theme.value
	}
	if !isValidTheme(theme) {
		return runConfig{}, fmt.Errorf("invalid theme mode: %s", theme)
	}

	maxDepth := -1
	if settings.MaxDepth != nil {
		maxDepth = *settings.MaxDepth
	}
	if fo.maxDepth.set {
		maxDepth = fo.maxDepth.value
	}
	if maxDepth < -1 {
		return runConfig{}, fmt.Errorf("max-depth must be -1 or greater")
	}

	outRoot := "./html_export"
	if settings.Out != "" {
		outRoot = settings.Out
	}
	if fo.out.set {
		outRoot = fo.out.value
	}
	outAbs, err := filepath.Abs(outRoot)
	if err != nil {
		return runConfig{}, err
	}
	cacheDir := filepath.Join(outAbs, ".repos")
	if settings.CacheDir != "" {
		cacheDir, err = resolvePath(settings.CacheDir, configDir)
		if err != nil {
			return runConfig{}, err
		}
	}

	watchMode := false
	if settings.Watch != nil {
		watchMode = *settings.Watch
	}
	if fo.watch.set {
		watchMode = fo.watch.value
	}

	watchPoll := "2s"
	if settings.WatchPoll != "" {
		watchPoll = settings.WatchPoll
	}
	if fo.watchPoll.set {
		watchPoll = fo.watchPoll.value
	}
	pollDur, err := time.ParseDuration(watchPoll)
	if err != nil {
		return runConfig{}, fmt.Errorf("invalid watch poll duration %q", watchPoll)
	}

	watchDebounce := "1s"
	if settings.WatchDebounce != "" {
		watchDebounce = settings.WatchDebounce
	}
	if fo.watchDebounce.set {
		watchDebounce = fo.watchDebounce.value
	}
	debounceDur, err := time.ParseDuration(watchDebounce)
	if err != nil {
		return runConfig{}, fmt.Errorf("invalid watch debounce duration %q", watchDebounce)
	}

	defaultGitPull := false
	if settings.WatchGitPull != nil {
		defaultGitPull = *settings.WatchGitPull
	}
	if fo.watchGitPull.set {
		defaultGitPull = fo.watchGitPull.value
	}

	gitPullInterval := "5m"
	if settings.GitPullInterval != "" {
		gitPullInterval = settings.GitPullInterval
	}
	if fo.gitPullInterval.set {
		gitPullInterval = fo.gitPullInterval.value
	}
	gitPullDur, err := time.ParseDuration(gitPullInterval)
	if err != nil {
		return runConfig{}, fmt.Errorf("invalid git pull interval %q", gitPullInterval)
	}

	gitRemote := "origin"
	if settings.GitRemote != "" {
		gitRemote = settings.GitRemote
	}
	if fo.gitRemote.set {
		gitRemote = fo.gitRemote.value
	}
	gitBranch := settings.GitBranch
	if fo.gitBranch.set {
		gitBranch = fo.gitBranch.value
	}

	css := settings.CSS
	if fo.css.set {
		css = fo.css.value
	}

	nbDefs := make([]NotebookSettings, 0)
	if len(settings.Notebooks) > 0 {
		nbDefs = append(nbDefs, settings.Notebooks...)
	} else {
		nbDefs = append(nbDefs, NotebookSettings{
			Name:            settings.Name,
			Vault:           settings.Vault,
			RootNote:        settings.RootNote,
			Image:           settings.Image,
			GitRepo:         settings.GitRepo,
			GitBranch:       settings.GitBranch,
			GitRemote:       settings.GitRemote,
			WatchGitPull:    settings.WatchGitPull,
			GitPullInterval: settings.GitPullInterval,
			Theme:           settings.Theme,
			CSS:             settings.CSS,
			MaxDepth:        settings.MaxDepth,
		})
	}
	if len(fo.vaults) > 1 {
		nbDefs = make([]NotebookSettings, 0, len(fo.vaults))
		for _, v := range fo.vaults {
			nbDefs = append(nbDefs, NotebookSettings{Vault: v})
		}
	}

	if len(nbDefs) > 1 && fo.rootNote.set {
		return runConfig{}, fmt.Errorf("--root-note is only valid with a single notebook")
	}
	if len(nbDefs) > 1 && fo.zip.set && fo.zip.value {
		return runConfig{}, fmt.Errorf("--zip is only supported with a single notebook")
	}

	notebooks := make([]notebookRuntime, 0, len(nbDefs))
	slugSeen := map[string]int{}
	for idx, nb := range nbDefs {
		curTheme := theme
		if nb.Theme != "" {
			curTheme = nb.Theme
		}
		if fo.theme.set {
			curTheme = fo.theme.value
		}
		if !isValidTheme(curTheme) {
			return runConfig{}, fmt.Errorf("invalid theme for notebook %q: %s", nb.Name, curTheme)
		}

		curMaxDepth := maxDepth
		if nb.MaxDepth != nil {
			curMaxDepth = *nb.MaxDepth
		}
		if fo.maxDepth.set {
			curMaxDepth = fo.maxDepth.value
		}

		curCSS := css
		if nb.CSS != "" {
			curCSS = nb.CSS
		}
		if fo.css.set {
			curCSS = fo.css.value
		}

		curGitPull := defaultGitPull
		if nb.WatchGitPull != nil {
			curGitPull = *nb.WatchGitPull
		}
		if fo.watchGitPull.set {
			curGitPull = fo.watchGitPull.value
		}

		curGitPullDur := gitPullDur
		if nb.GitPullInterval != "" {
			d, err := time.ParseDuration(nb.GitPullInterval)
			if err != nil {
				return runConfig{}, fmt.Errorf("invalid git pull interval for notebook %q: %s", nb.Name, nb.GitPullInterval)
			}
			curGitPullDur = d
		}
		if fo.gitPullInterval.set {
			curGitPullDur = gitPullDur
		}

		curGitRemote := gitRemote
		if nb.GitRemote != "" {
			curGitRemote = nb.GitRemote
		}
		if fo.gitRemote.set {
			curGitRemote = fo.gitRemote.value
		}

		curGitBranch := gitBranch
		if nb.GitBranch != "" {
			curGitBranch = nb.GitBranch
		}
		if fo.gitBranch.set {
			curGitBranch = fo.gitBranch.value
		}

		vault := nb.Vault
		if len(fo.vaults) == 1 {
			vault = fo.vaults[0]
		}
		if vault == "" {
			vault = "."
		}

		rootNote := nb.RootNote
		if fo.rootNote.set {
			rootNote = fo.rootNote.value
		}

		name := strings.TrimSpace(nb.Name)
		if name == "" {
			name = inferNotebookName(vault, nb.GitRepo)
		}

		slug := makeSlug(name)
		if slug == "" {
			slug = fmt.Sprintf("notebook-%d", idx+1)
		}
		slugSeen[slug]++
		if slugSeen[slug] > 1 {
			slug = fmt.Sprintf("%s-%d", slug, slugSeen[slug])
		}

		outDir := outAbs
		indexHref := "index.html"
		if len(nbDefs) > 1 {
			outDir = filepath.Join(outAbs, "notebooks", slug)
			indexHref = filepath.ToSlash(filepath.Join("notebooks", slug, "index.html"))
		}

		if nb.GitRepo != "" && nb.WatchGitPull == nil && !fo.watchGitPull.set {
			curGitPull = true
		}

		notebooks = append(notebooks, notebookRuntime{
			slug:            slug,
			name:            name,
			vault:           vault,
			rootNote:        rootNote,
			image:           nb.Image,
			gitRepo:         nb.GitRepo,
			gitBranch:       curGitBranch,
			gitRemote:       curGitRemote,
			gitPullInterval: curGitPullDur,
			watchGitPull:    curGitPull,
			theme:           exporter.ThemeMode(curTheme),
			css:             curCSS,
			maxDepth:        curMaxDepth,
			outDir:          outDir,
			indexHref:       indexHref,
		})
	}

	if len(notebooks) == 1 && notebooks[0].gitRepo == "" && !fromConfig && len(fo.vaults) == 0 {
		notebooks[0].vault = "."
	}

	if len(notebooks) == 1 && (fo.zip.set && fo.zip.value) && watchMode {
		return runConfig{}, fmt.Errorf("--zip is not supported with --watch")
	}

	rc := runConfig{
		outRoot:      outAbs,
		cacheDir:     cacheDir,
		watch:        watchMode,
		watchPoll:    pollDur,
		watchDebonce: debounceDur,
		zip:          fo.zip.set && fo.zip.value,
		zipPath:      fo.zipPath.value,
		notebooks:    notebooks,
		configDir:    configDir,
	}
	if fo.zip.set && !fo.zip.value {
		rc.zip = false
	}
	return rc, nil
}

func extractConfigArg(args []string) (string, []string) {
	if len(args) == 0 {
		return "", args
	}
	first := args[0]
	if strings.HasPrefix(first, "-") {
		return "", args
	}
	if !strings.HasSuffix(strings.ToLower(first), ".yml") && !strings.HasSuffix(strings.ToLower(first), ".yaml") {
		return "", args
	}
	return first, args[1:]
}

func loadSettings(path string) (Settings, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Settings{}, "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Settings{}, "", err
	}
	cfg := Settings{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Settings{}, "", fmt.Errorf("parse settings yaml: %w", err)
	}
	return cfg, filepath.Dir(abs), nil
}

func prepareGitNotebooks(rc *runConfig) error {
	cacheRoot := rc.cacheDir
	if rc.configDir != "" {
		if err := os.MkdirAll(rc.outRoot, 0o755); err != nil {
			return err
		}
	}
	for i := range rc.notebooks {
		nb := &rc.notebooks[i]
		if nb.gitRepo == "" {
			vault, err := resolvePath(nb.vault, rc.configDir)
			if err != nil {
				return err
			}
			nb.vault = vault
			continue
		}
		if nb.vault != "" {
			vault, err := resolvePath(nb.vault, rc.configDir)
			if err != nil {
				return err
			}
			nb.vault = vault
		} else {
			nb.vault = filepath.Join(cacheRoot, nb.slug)
		}

		if err := ensureRepo(nb.vault, nb.gitRepo); err != nil {
			return fmt.Errorf("%s: %w", nb.name, err)
		}
		if err := syncGitRepo(nb.vault, nb.gitRemote, nb.gitBranch); err != nil {
			return fmt.Errorf("%s: %w", nb.name, err)
		}
	}
	return nil
}

func ensureRepo(path, repo string) error {
	if fi, err := os.Stat(filepath.Join(path, ".git")); err == nil && fi.IsDir() {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return readErr
		}
		if len(entries) > 0 {
			return fmt.Errorf("%s exists and is not a git repository", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", repo, path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func syncGitRepo(vaultRoot, remote, preferredBranch string) error {
	if remote == "" {
		remote = "origin"
	}
	if _, err := gitOutput(vaultRoot, "fetch", remote, "--prune"); err != nil {
		return err
	}
	branch := preferredBranch
	if branch == "" {
		var err error
		branch, err = pickBranch(vaultRoot, remote)
		if err != nil {
			return err
		}
	}
	if _, err := gitOutput(vaultRoot, "checkout", "-B", branch, remote+"/"+branch); err != nil {
		return err
	}
	if _, err := gitOutput(vaultRoot, "reset", "--hard", remote+"/"+branch); err != nil {
		return err
	}
	if _, err := gitOutput(vaultRoot, "clean", "-fd"); err != nil {
		return err
	}
	return nil
}

func runOnce(rc runConfig) error {
	if len(rc.notebooks) == 1 {
		nb := rc.notebooks[0]
		opts := exporter.Options{
			VaultRoot: nb.vault,
			OutDir:    nb.outDir,
			RootNote:  nb.rootNote,
			MaxDepth:  nb.maxDepth,
			ThemeMode: nb.theme,
			CSSPath:   nb.css,
			Zip:       rc.zip,
			ZipPath:   rc.zipPath,
		}
		res, err := exporter.Run(opts)
		if err != nil {
			return err
		}
		printNotebookResult(nb, res)
		return nil
	}

	for _, nb := range rc.notebooks {
		opts := exporter.Options{
			VaultRoot: nb.vault,
			OutDir:    nb.outDir,
			RootNote:  nb.rootNote,
			MaxDepth:  nb.maxDepth,
			ThemeMode: nb.theme,
			CSSPath:   nb.css,
		}
		res, err := exporter.Run(opts)
		if err != nil {
			return fmt.Errorf("%s: %w", nb.name, err)
		}
		printNotebookResult(nb, res)
	}
	if err := writeHubIndex(rc); err != nil {
		return err
	}
	fmt.Printf("[ok] Hub index: %s\n", filepath.Join(rc.outRoot, "index.html"))
	return nil
}

func runWatch(rc runConfig) error {
	if len(rc.notebooks) == 1 {
		nb := rc.notebooks[0]
		return watcher.Run(watcher.Options{
			ExportOptions: exporter.Options{
				VaultRoot: nb.vault,
				OutDir:    nb.outDir,
				RootNote:  nb.rootNote,
				MaxDepth:  nb.maxDepth,
				ThemeMode: nb.theme,
				CSSPath:   nb.css,
			},
			PollInterval:    rc.watchPoll,
			Debounce:        rc.watchDebonce,
			EnableGitPull:   nb.watchGitPull,
			GitPullInterval: nb.gitPullInterval,
			GitBranch:       nb.gitBranch,
			GitRemote:       nb.gitRemote,
			NotebookName:    nb.name,
		})
	}

	var mu sync.Mutex
	writeHub := func() {
		mu.Lock()
		defer mu.Unlock()
		_ = writeHubIndex(rc)
	}

	errCh := make(chan error, len(rc.notebooks))
	for _, nb := range rc.notebooks {
		n := nb
		go func() {
			errCh <- watcher.Run(watcher.Options{
				ExportOptions: exporter.Options{
					VaultRoot: n.vault,
					OutDir:    n.outDir,
					RootNote:  n.rootNote,
					MaxDepth:  n.maxDepth,
					ThemeMode: n.theme,
					CSSPath:   n.css,
				},
				PollInterval:    rc.watchPoll,
				Debounce:        rc.watchDebonce,
				EnableGitPull:   n.watchGitPull,
				GitPullInterval: n.gitPullInterval,
				GitBranch:       n.gitBranch,
				GitRemote:       n.gitRemote,
				NotebookName:    n.name,
				AfterExport: func(_ string, _ exporter.Result) {
					writeHub()
				},
			})
		}()
	}

	for i := 0; i < len(rc.notebooks); i++ {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

func writeHubIndex(rc runConfig) error {
	if len(rc.notebooks) <= 1 {
		return nil
	}
	if err := os.MkdirAll(rc.outRoot, 0o755); err != nil {
		return err
	}

	type hubCard struct {
		Name  string
		Href  string
		Image string
		Vault string
	}
	cards := make([]hubCard, 0, len(rc.notebooks))
	for _, nb := range rc.notebooks {
		img, err := resolveNotebookImage(nb, rc.outRoot, rc.configDir)
		if err != nil {
			return err
		}
		cards = append(cards, hubCard{Name: nb.name, Href: nb.indexHref, Image: img, Vault: nb.vault})
	}
	sort.Slice(cards, func(i, j int) bool {
		return strings.ToLower(cards[i].Name) < strings.ToLower(cards[j].Name)
	})

	var b strings.Builder
	for _, c := range cards {
		b.WriteString(`<a class="book" href="` + html.EscapeString(c.Href) + `">`)
		if c.Image != "" {
			b.WriteString(`<img src="` + html.EscapeString(c.Image) + `" alt="` + html.EscapeString(c.Name) + `" loading="lazy">`)
		} else {
			b.WriteString(`<div class="placeholder">` + html.EscapeString(initials(c.Name)) + `</div>`)
		}
		b.WriteString(`<div class="meta"><h2>` + html.EscapeString(c.Name) + `</h2><p>` + html.EscapeString(c.Vault) + `</p></div></a>`)
	}

	page := `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>obsidian-polished</title>
  <style>
:root{--bg:#f6f3eb;--ink:#1f252e;--muted:#4c5b6c;--card:#fffffff2;--stroke:#c8d0d8;--shadow:0 18px 40px rgba(31,37,46,.12);--accent:#d96f32;--accent2:#2f6f94}
*{box-sizing:border-box}
body{margin:0;font-family:"IBM Plex Sans", "Avenir Next", "Segoe UI", sans-serif;color:var(--ink);background:radial-gradient(circle at 12% 14%,#f4e7ce 0,#f6f3eb 42%,#eef2f5 100%)}
main{max-width:1100px;margin:0 auto;padding:56px 20px 80px}
.hero{display:flex;justify-content:space-between;gap:20px;align-items:flex-end;margin-bottom:26px}
.hero h1{margin:0;font-size:clamp(2rem,4vw,3rem);letter-spacing:-.02em}
.hero p{margin:.5rem 0 0;color:var(--muted);font-size:1.05rem}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));gap:16px}
.book{display:flex;flex-direction:column;gap:12px;padding:14px;border:1px solid var(--stroke);border-radius:16px;background:var(--card);text-decoration:none;color:inherit;box-shadow:var(--shadow);transform:translateY(0);transition:transform .18s ease,box-shadow .18s ease,border-color .18s ease}
.book:hover{transform:translateY(-4px);border-color:var(--accent2);box-shadow:0 22px 38px rgba(47,111,148,.22)}
.book img,.placeholder{width:100%;aspect-ratio:16/10;object-fit:cover;border-radius:12px;border:1px solid #d8dee4;background:linear-gradient(135deg,#fbe0c4,#d9e7f0)}
.placeholder{display:grid;place-items:center;font-size:2.1rem;font-weight:700;color:#23384a}
.meta h2{margin:0 0 6px;font-size:1.08rem}
.meta p{margin:0;color:var(--muted);font-family:"IBM Plex Mono","Menlo",monospace;font-size:.8rem;overflow-wrap:anywhere}
.tag{font-size:.78rem;color:#65422e;background:#fcefdc;border:1px solid #ebb889;padding:4px 8px;border-radius:999px}
@media (max-width:700px){main{padding-top:34px}.hero{flex-direction:column;align-items:flex-start}}
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <div>
        <h1>Notebook Library</h1>
        <p>Choose an Obsidian notebook export.</p>
      </div>
      <div class="tag">obsidian-polished</div>
    </section>
    <section class="grid">` + b.String() + `</section>
  </main>
</body>
</html>`

	return os.WriteFile(filepath.Join(rc.outRoot, "index.html"), []byte(page), 0o644)
}

func resolveNotebookImage(nb notebookRuntime, outRoot, configDir string) (string, error) {
	img := strings.TrimSpace(nb.image)
	if img == "" {
		return "", nil
	}
	if strings.HasPrefix(img, "http://") || strings.HasPrefix(img, "https://") {
		return img, nil
	}
	resolved, err := resolvePath(img, configDir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(resolved); err != nil {
		alt := filepath.Join(nb.vault, img)
		if _, err2 := os.Stat(alt); err2 == nil {
			resolved = alt
		} else {
			return "", err
		}
	}

	dstDir := filepath.Join(outRoot, "assets", "notebooks", nb.slug)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dstDir, filepath.Base(resolved))
	if err := copyFile(resolved, dst); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("assets", "notebooks", nb.slug, filepath.Base(resolved))), nil
}

func printNotebookResult(nb notebookRuntime, res exporter.Result) {
	fmt.Printf("[ok] %s\n", nb.name)
	if res.ZipPath != "" {
		fmt.Printf("     archive: %s\n", res.ZipPath)
	} else {
		fmt.Printf("     output: %s\n", res.OutputDir)
		fmt.Printf("     open: %s\n", filepath.Join(res.OutputDir, "index.html"))
	}
	fmt.Printf("     notes: %d | assets: %d\n", res.NotesExported, res.AssetsCopied)
}

func printBanner(rc runConfig) {
	mode := "export"
	if rc.watch {
		mode = "watch"
	}
	fmt.Printf("obsidian-polished | mode=%s | notebooks=%d\n", mode, len(rc.notebooks))
}

func printUsage() {
	fmt.Print(`obsidian-polished

Usage:
  obsidian-polished [flags]
  obsidian-polished settings.yml [flags]

Behavior:
  Running with no args shows this help.
  When a settings file is provided, flags override settings values.

Core flags:
  --vault path                   Vault root (repeat for multiple notebooks)
  --root-note string             Root note; omitted means export all notes
  --out string                   Output directory (default ./html_export)
  --watch                        Regenerate on changes
  --theme string                 both|light|dark
  --watch-git-pull               Enable periodic git sync
  --watch-git-pull-interval dur  Git sync interval (default 5m)
  --config path.yml              Settings file path (alternative to positional)
  -h, --help                     Show help

Example settings.yml:
  out: ./site
  watch: true
  notebooks:
    - name: Team Notes
      git_repo: git@github.com:org/team-notes.git
      git_branch: main
      image: ./images/team-cover.jpg
      root_note: Home.md
    - name: Personal Vault
      vault: /Users/you/Obsidian/Personal
`)
}

func inferNotebookName(vault, gitRepo string) string {
	if gitRepo != "" {
		base := strings.TrimSuffix(filepath.Base(gitRepo), filepath.Ext(gitRepo))
		if base != "" && base != "." {
			return strings.ReplaceAll(base, "-", " ")
		}
	}
	base := filepath.Base(vault)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "Notebook"
	}
	return strings.ReplaceAll(base, "-", " ")
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func makeSlug(in string) string {
	s := strings.ToLower(strings.TrimSpace(in))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func initials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "NB"
	}
	if len(parts) == 1 {
		r := []rune(parts[0])
		if len(r) >= 2 {
			return strings.ToUpper(string(r[:2]))
		}
		return strings.ToUpper(parts[0])
	}
	return strings.ToUpper(string([]rune(parts[0])[0]) + string([]rune(parts[1])[0]))
}

func isValidTheme(v string) bool {
	return v == string(exporter.ThemeBoth) || v == string(exporter.ThemeLight) || v == string(exporter.ThemeDark)
}

func resolvePath(p, base string) (string, error) {
	if p == "" {
		return "", nil
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	if base != "" {
		return filepath.Abs(filepath.Join(base, p))
	}
	return filepath.Abs(p)
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o644)
}

func pickBranch(vaultRoot, remote string) (string, error) {
	candidates := []string{"main", "master"}
	for _, c := range candidates {
		_, err := gitOutput(vaultRoot, "show-ref", "--verify", "--quiet", "refs/remotes/"+remote+"/"+c)
		if err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find %s/main or %s/master", remote, remote)
}

func gitOutput(vaultRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", vaultRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
