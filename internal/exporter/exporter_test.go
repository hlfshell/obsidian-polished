package exporter

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLinkSpec(t *testing.T) {
	target, alias, anchor := parseLinkSpec("Note Name|Alias")
	if target != "Note Name" || alias != "Alias" || anchor != "" {
		t.Fatalf("unexpected parse: %q %q %q", target, alias, anchor)
	}
	target, alias, anchor = parseLinkSpec("Note#Section")
	if target != "Note" || alias != "" || anchor != "Section" {
		t.Fatalf("unexpected parse: %q %q %q", target, alias, anchor)
	}
}

func TestMaxDepthStopsTraversal(t *testing.T) {
	vault := t.TempDir()
	mustWrite(t, filepath.Join(vault, "A.md"), "# A\n[[B]]")
	mustWrite(t, filepath.Join(vault, "B.md"), "# B\n[[C]]")
	mustWrite(t, filepath.Join(vault, "C.md"), "# C")

	out := filepath.Join(t.TempDir(), "out")
	res, err := Run(Options{VaultRoot: vault, OutDir: out, RootNote: "A.md", MaxDepth: 1, ThemeMode: ThemeBoth})
	if err != nil {
		t.Fatal(err)
	}
	if res.NotesExported != 2 {
		t.Fatalf("expected 2 notes, got %d", res.NotesExported)
	}
	if _, err := os.Stat(filepath.Join(out, "notes", "A.html")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "notes", "B.html")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "notes", "C.html")); !os.IsNotExist(err) {
		t.Fatalf("expected C.html not to exist")
	}
}

func TestNoRootGeneratesLandingForAllNotes(t *testing.T) {
	vault := t.TempDir()
	mustWrite(t, filepath.Join(vault, "Alpha.md"), "# Alpha")
	mustWrite(t, filepath.Join(vault, "folder", "Beta.md"), "# Beta")

	out := filepath.Join(t.TempDir(), "out")
	_, err := Run(Options{VaultRoot: vault, OutDir: out, ThemeMode: ThemeBoth})
	if err != nil {
		t.Fatal(err)
	}
	indexBytes, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	index := string(indexBytes)
	if !strings.Contains(index, "Alpha") || !strings.Contains(index, "Folder") {
		t.Fatalf("index missing root note/folder cards: %s", index)
	}
	if !strings.Contains(index, "theme-toggle") {
		t.Fatalf("index missing theme toggle")
	}
	folderIndexBytes, err := os.ReadFile(filepath.Join(out, "collections", "folder", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(folderIndexBytes), "Beta") {
		t.Fatalf("folder collection page missing nested note")
	}
}

func TestZipOutput(t *testing.T) {
	vault := t.TempDir()
	mustWrite(t, filepath.Join(vault, "Root.md"), "# Root")

	tmp := t.TempDir()
	out := filepath.Join(tmp, "export-dir")
	zipPath := filepath.Join(tmp, "bundle.zip")
	res, err := Run(Options{VaultRoot: vault, OutDir: out, RootNote: "Root.md", Zip: true, ZipPath: zipPath, ThemeMode: ThemeLight})
	if err != nil {
		t.Fatal(err)
	}
	if res.ZipPath == "" {
		t.Fatalf("expected zip path to be set")
	}
	if _, err := os.Stat(zipPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("expected output dir to be removed after zip")
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	foundIndex := false
	for _, f := range zr.File {
		if f.Name == "index.html" {
			foundIndex = true
			break
		}
	}
	if !foundIndex {
		t.Fatalf("zip missing index.html")
	}
}

func TestNotePageIncludesTimestamps(t *testing.T) {
	vault := t.TempDir()
	mustWrite(t, filepath.Join(vault, "Stamp.md"), "# Stamp")

	out := filepath.Join(t.TempDir(), "out")
	_, err := Run(Options{VaultRoot: vault, OutDir: out, ThemeMode: ThemeBoth})
	if err != nil {
		t.Fatal(err)
	}

	pageBytes, err := os.ReadFile(filepath.Join(out, "notes", "Stamp.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(pageBytes)
	if !strings.Contains(page, "Created ") {
		t.Fatalf("note page missing created timestamp")
	}
	if !strings.Contains(page, "Updated ") {
		t.Fatalf("note page missing updated timestamp")
	}
}

func TestNoteBreadcrumbIncludesFolders(t *testing.T) {
	vault := t.TempDir()
	mustWrite(t, filepath.Join(vault, "a", "b", "Deep.md"), "# Deep")

	out := filepath.Join(t.TempDir(), "out")
	_, err := Run(Options{VaultRoot: vault, OutDir: out, ThemeMode: ThemeBoth})
	if err != nil {
		t.Fatal(err)
	}

	pageBytes, err := os.ReadFile(filepath.Join(out, "notes", "a", "b", "Deep.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(pageBytes)
	if !strings.Contains(page, `>Home</a>`) {
		t.Fatalf("breadcrumb missing home link")
	}
	if !strings.Contains(page, `collections/a/index.html`) {
		t.Fatalf("breadcrumb missing folder A link")
	}
	if !strings.Contains(page, `collections/a/b/index.html`) {
		t.Fatalf("breadcrumb missing folder B link")
	}
	if !strings.Contains(page, `class="crumb current">Deep</span>`) {
		t.Fatalf("breadcrumb missing current note")
	}
}

func mustWrite(t *testing.T, p, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
