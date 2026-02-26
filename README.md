# obshtml

`obshtml` is a standalone Go CLI that exports Obsidian markdown notes into a portable static HTML site.

## Features

- Export from a single root note with recursive wikilink traversal.
- Export all notes when `--root-note` is omitted (nice landing page index).
- Theme modes: `both` (auto-detect + toggle with persistence), `light`, `dark`.
- Optional custom CSS override via `--css`.
- Optional `--max-depth` to limit recursion when traversing from root.
- Optional zip output with configurable destination.

## Build

```bash
go build -o obshtml ./cmd/obshtml
```

Move binary into your `PATH` as needed.

## Usage

```bash
./obshtml \
  --vault /path/to/vault \
  --root-note "Research Summary.md" \
  --out /tmp/export \
  --theme both \
  --max-depth -1
```

Export all notes (no root specified):

```bash
./obshtml --vault /path/to/vault --out /tmp/export-all
```

Create zip instead of folder:

```bash
./obshtml --vault /path/to/vault --root-note Home.md --zip --zip-path /tmp/vault_export.zip
```

Use custom CSS:

```bash
./obshtml --vault /path/to/vault --css /path/to/my_theme.css
```

## Flags

- `--vault` Vault root (default `.`)
- `--root-note` Root note filename/path (optional)
- `--out` Output directory (default `./html_export`)
- `--max-depth` Max traversal depth from root (`-1` unlimited, default `-1`)
- `--theme` `both|light|dark` (default `both`)
- `--css` Optional CSS file override
- `--zip` Emit zip archive instead of folder
- `--zip-path` Zip destination path (default `<out>.zip`)

## Test

```bash
go test ./...
```
