package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/keith/obsidian-html-sharer/internal/exporter"
)

func main() {
	var (
		vault    = flag.String("vault", ".", "Obsidian vault root")
		rootNote = flag.String("root-note", "", "Root note to export (optional). If omitted, all notes are exported.")
		outDir   = flag.String("out", "./html_export", "Output directory")
		maxDepth = flag.Int("max-depth", -1, "Maximum link depth from root note (-1 means unlimited)")
		theme    = flag.String("theme", "both", "Theme mode: both|light|dark")
		cssPath  = flag.String("css", "", "Optional CSS file to override default styling")
		zipOut   = flag.Bool("zip", false, "Output as zip archive instead of folder")
		zipPath  = flag.String("zip-path", "", "Destination zip file path (default: <out>.zip)")
	)
	flag.Parse()

	result, err := exporter.Run(exporter.Options{
		VaultRoot: *vault,
		OutDir:    *outDir,
		RootNote:  *rootNote,
		MaxDepth:  *maxDepth,
		ThemeMode: exporter.ThemeMode(*theme),
		CSSPath:   *cssPath,
		Zip:       *zipOut,
		ZipPath:   *zipPath,
	})
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
