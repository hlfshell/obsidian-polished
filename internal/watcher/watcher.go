package watcher

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hlfshell/obsidian-polished/internal/exporter"
)

type Options struct {
	ExportOptions   exporter.Options
	PollInterval    time.Duration
	Debounce        time.Duration
	EnableGitPull   bool
	GitPullInterval time.Duration
	GitBranch       string
	GitRemote       string
	GitSSHKey       string
	GitSSHAcceptNew bool
	NotebookName    string
	AfterExport     func(notebook string, res exporter.Result)
}

type fileState struct {
	mod  int64
	size int64
}

type eventType int

const (
	eventAdded eventType = iota
	eventModified
	eventDeleted
)

type changeEvent struct {
	typ eventType
}

func Run(opts Options) error {
	if opts.PollInterval <= 0 {
		opts.PollInterval = 2 * time.Second
	}
	if opts.Debounce <= 0 {
		opts.Debounce = time.Second
	}
	if opts.GitPullInterval <= 0 {
		opts.GitPullInterval = 5 * time.Minute
	}
	if opts.GitRemote == "" {
		opts.GitRemote = "origin"
	}

	res, err := exporter.Run(opts.ExportOptions)
	if err != nil {
		return err
	}
	fmt.Printf("Initial export complete: %s\n", res.OutputDir)
	fmt.Printf("Notes exported: %d\n", res.NotesExported)
	fmt.Printf("Assets copied: %d\n", res.AssetsCopied)
	if opts.AfterExport != nil {
		opts.AfterExport(opts.NotebookName, res)
	}

	if opts.EnableGitPull {
		if gitIsRepo(opts.ExportOptions.VaultRoot) {
			fmt.Printf("Git sync enabled (remote=%s, interval=%s)\n", opts.GitRemote, opts.GitPullInterval)
		} else {
			fmt.Println("Git sync requested but vault is not a git repository; continuing without git sync")
			opts.EnableGitPull = false
		}
	}

	prev, err := scanFiles(opts.ExportOptions.VaultRoot, opts.ExportOptions.OutDir)
	if err != nil {
		return err
	}

	pollTicker := time.NewTicker(opts.PollInterval)
	defer pollTicker.Stop()

	var gitTicker *time.Ticker
	if opts.EnableGitPull {
		gitTicker = time.NewTicker(opts.GitPullInterval)
		defer gitTicker.Stop()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	pending := map[string]changeEvent{}
	lastEvent := time.Time{}

	var generationMu sync.Mutex

	for {
		select {
		case <-sigCh:
			fmt.Println("Stopping watcher")
			return nil
		case <-pollTicker.C:
			next, changes, err := diffScan(opts.ExportOptions.VaultRoot, opts.ExportOptions.OutDir, prev)
			if err != nil {
				fmt.Printf("scan error: %v\n", err)
				continue
			}
			prev = next
			if len(changes) > 0 {
				for k, v := range changes {
					pending[k] = v
				}
				lastEvent = time.Now()
			}
		case <-func() <-chan time.Time {
			if gitTicker == nil {
				return nil
			}
			return gitTicker.C
		}():
			generationMu.Lock()
			changed, err := syncGit(opts.ExportOptions.VaultRoot, opts.GitRemote, opts.GitBranch, opts.GitSSHKey, opts.GitSSHAcceptNew)
			generationMu.Unlock()
			if err != nil {
				fmt.Printf("git sync failed: %v\n", err)
				continue
			}
			if changed {
				fmt.Println("Git sync pulled new commit; running full export")
				generationMu.Lock()
				res, err := exporter.Run(opts.ExportOptions)
				generationMu.Unlock()
				if err != nil {
					fmt.Printf("export failed: %v\n", err)
					continue
				}
				if opts.AfterExport != nil {
					opts.AfterExport(opts.NotebookName, res)
				}
				next, err := scanFiles(opts.ExportOptions.VaultRoot, opts.ExportOptions.OutDir)
				if err == nil {
					prev = next
				}
			}
		default:
			if len(pending) > 0 && !lastEvent.IsZero() && time.Since(lastEvent) >= opts.Debounce {
				batch := pending
				pending = map[string]changeEvent{}
				generationMu.Lock()
				res, err := processBatch(opts.ExportOptions, batch)
				generationMu.Unlock()
				if err != nil {
					fmt.Printf("watch export failed: %v\n", err)
					continue
				}
				if opts.AfterExport != nil {
					opts.AfterExport(opts.NotebookName, res)
				}
			}
			time.Sleep(80 * time.Millisecond)
		}
	}
}

func processBatch(base exporter.Options, batch map[string]changeEvent) (exporter.Result, error) {
	paths := make([]string, 0, len(batch))
	for p := range batch {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	full := false
	changedRoots := make([]string, 0)
	for _, p := range paths {
		ev := batch[p]
		if strings.EqualFold(filepath.Ext(p), ".md") {
			if ev.typ == eventDeleted {
				full = true
				break
			}
			changedRoots = append(changedRoots, p)
		}
	}
	if base.RootNote != "" {
		full = true
	}

	if full {
		res, err := exporter.Run(base)
		return res, err
	}

	for _, rel := range paths {
		ev := batch[rel]
		if strings.EqualFold(filepath.Ext(rel), ".md") {
			continue
		}
		if err := syncAsset(base.VaultRoot, base.OutDir, rel, ev.typ); err != nil {
			return exporter.Result{}, err
		}
	}

	var lastRes exporter.Result
	for _, root := range changedRoots {
		opts := base
		opts.RootNote = root
		opts.MaxDepth = -1
		opts.Incremental = true
		opts.IndexAllNotes = true
		res, err := exporter.Run(opts)
		if err != nil {
			return exporter.Result{}, err
		}
		lastRes = res
	}

	if len(changedRoots) == 0 {
		lastRes = exporter.Result{OutputDir: base.OutDir}
	}
	return lastRes, nil
}

func syncAsset(vaultRoot, outDir, rel string, ev eventType) error {
	src := filepath.Join(vaultRoot, filepath.FromSlash(rel))
	dst := filepath.Join(outDir, "assets", filepath.FromSlash(rel))
	if ev == eventDeleted {
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return copyFile(src, dst)
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

func diffScan(vaultRoot, outDir string, prev map[string]fileState) (map[string]fileState, map[string]changeEvent, error) {
	next, err := scanFiles(vaultRoot, outDir)
	if err != nil {
		return nil, nil, err
	}
	changes := map[string]changeEvent{}

	for p, old := range prev {
		n, ok := next[p]
		if !ok {
			changes[p] = changeEvent{typ: eventDeleted}
			continue
		}
		if n.mod != old.mod || n.size != old.size {
			changes[p] = changeEvent{typ: eventModified}
		}
	}
	for p := range next {
		if _, ok := prev[p]; !ok {
			changes[p] = changeEvent{typ: eventAdded}
		}
	}
	return next, changes, nil
}

func scanFiles(vaultRoot, outDir string) (map[string]fileState, error) {
	res := map[string]fileState{}
	vaultAbs, err := filepath.Abs(vaultRoot)
	if err != nil {
		return nil, err
	}
	outAbs, err := filepath.Abs(outDir)
	if err != nil {
		return nil, err
	}

	err = filepath.WalkDir(vaultAbs, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == outAbs || strings.HasPrefix(p, outAbs+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(vaultAbs, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = toSlash(rel)
		if isExcluded(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		res[rel] = fileState{mod: fi.ModTime().UnixNano(), size: fi.Size()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func syncGit(vaultRoot, remote, preferredBranch, sshKey string, sshAcceptNew bool) (bool, error) {
	before, err := gitOutput(vaultRoot, sshKey, sshAcceptNew, "rev-parse", "HEAD")
	if err != nil {
		return false, err
	}
	if _, err := gitOutput(vaultRoot, sshKey, sshAcceptNew, "fetch", remote, "--prune"); err != nil {
		return false, err
	}

	branch := preferredBranch
	if branch == "" {
		branch, err = pickBranch(vaultRoot, remote, sshKey, sshAcceptNew)
		if err != nil {
			return false, err
		}
	}

	if _, err := gitOutput(vaultRoot, sshKey, sshAcceptNew, "checkout", "-B", branch, remote+"/"+branch); err != nil {
		return false, err
	}
	if _, err := gitOutput(vaultRoot, sshKey, sshAcceptNew, "reset", "--hard", remote+"/"+branch); err != nil {
		return false, err
	}
	if _, err := gitOutput(vaultRoot, sshKey, sshAcceptNew, "clean", "-fd"); err != nil {
		return false, err
	}
	after, err := gitOutput(vaultRoot, sshKey, sshAcceptNew, "rev-parse", "HEAD")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(before) != strings.TrimSpace(after), nil
}

func pickBranch(vaultRoot, remote, sshKey string, sshAcceptNew bool) (string, error) {
	candidates := []string{"main", "master"}
	for _, c := range candidates {
		_, err := gitOutput(vaultRoot, sshKey, sshAcceptNew, "show-ref", "--verify", "--quiet", "refs/remotes/"+remote+"/"+c)
		if err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find %s/main or %s/master", remote, remote)
}

func gitIsRepo(vaultRoot string) bool {
	out, err := gitOutput(vaultRoot, "", false, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

func gitOutput(vaultRoot, sshKey string, sshAcceptNew bool, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", vaultRoot}, args...)...)
	applyGitSSHKey(cmd, sshKey, sshAcceptNew)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func applyGitSSHKey(cmd *exec.Cmd, sshKey string, sshAcceptNew bool) {
	if strings.TrimSpace(sshKey) == "" && !sshAcceptNew {
		return
	}
	parts := []string{"ssh", "-o BatchMode=yes"}
	if strings.TrimSpace(sshKey) != "" {
		escaped := strings.ReplaceAll(sshKey, `'`, `'"'"'`)
		parts = append(parts, "-i '"+escaped+"'", "-o IdentitiesOnly=yes")
	}
	if sshAcceptNew {
		parts = append(parts, "-o StrictHostKeyChecking=accept-new", "-o UserKnownHostsFile=/tmp/obsidian_polished_known_hosts")
	}
	cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND="+strings.Join(parts, " "))
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
