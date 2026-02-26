package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hlfshell/obsidian-polished/internal/exporter"
	"github.com/hlfshell/obsidian-polished/internal/watcher"
)

func main() {
	var (
		vault                = flag.String("vault", ".", "Obsidian vault root")
		rootNote             = flag.String("root-note", "", "Root note to export (optional). If omitted, all notes are exported.")
		outDir               = flag.String("out", "./html_export", "Output directory")
		maxDepth             = flag.Int("max-depth", -1, "Maximum link depth from root note (-1 means unlimited)")
		theme                = flag.String("theme", "both", "Theme mode: both|light|dark")
		cssPath              = flag.String("css", "", "Optional CSS file to override default styling")
		zipOut               = flag.Bool("zip", false, "Output as zip archive instead of folder")
		zipPath              = flag.String("zip-path", "", "Destination zip file path (default: <out>.zip)")
		watchMode            = flag.Bool("watch", false, "Watch vault and regenerate exports on changes")
		watchPoll            = flag.String("watch-poll", "2s", "Watch polling interval (e.g. 2s)")
		watchDebounce        = flag.String("watch-debounce", "1s", "Watch debounce duration for rapid file changes")
		watchGitPull         = flag.Bool("watch-git-pull", false, "If vault is a git repo, periodically sync from remote")
		watchGitPullInterval = flag.String("watch-git-pull-interval", "5m", "Git sync interval when watch-git-pull is enabled")
		watchGitBranch       = flag.String("watch-git-branch", "", "Git branch to stay on (default: auto main/master)")
		watchGitRemote       = flag.String("watch-git-remote", "origin", "Git remote used for sync")
	)
	flag.Parse()

	baseOpts := exporter.Options{
		VaultRoot: *vault,
		OutDir:    *outDir,
		RootNote:  *rootNote,
		MaxDepth:  *maxDepth,
		ThemeMode: exporter.ThemeMode(*theme),
		CSSPath:   *cssPath,
		Zip:       *zipOut,
		ZipPath:   *zipPath,
	}

	if *watchMode {
		poll, err := time.ParseDuration(*watchPoll)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: invalid --watch-poll:", err)
			os.Exit(1)
		}
		debounce, err := time.ParseDuration(*watchDebounce)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: invalid --watch-debounce:", err)
			os.Exit(1)
		}
		gitInterval, err := time.ParseDuration(*watchGitPullInterval)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: invalid --watch-git-pull-interval:", err)
			os.Exit(1)
		}
		if baseOpts.Zip {
			fmt.Fprintln(os.Stderr, "error: --zip is not supported with --watch")
			os.Exit(1)
		}
		if err := watcher.Run(watcher.Options{
			ExportOptions:   baseOpts,
			PollInterval:    poll,
			Debounce:        debounce,
			EnableGitPull:   *watchGitPull,
			GitPullInterval: gitInterval,
			GitBranch:       *watchGitBranch,
			GitRemote:       *watchGitRemote,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	result, err := exporter.Run(baseOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if result.ZipPath != "" {
		fmt.Printf("Export complete: %s\n", result.ZipPath)
	} else {
		fmt.Printf("Export complete: %s\n", result.OutputDir)
		fmt.Printf("Open: %s\n", filepath.Join(result.OutputDir, "index.html"))
	}
	fmt.Printf("Notes exported: %d\n", result.NotesExported)
	fmt.Printf("Assets copied: %d\n", result.AssetsCopied)
}
