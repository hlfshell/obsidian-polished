# obsidian-polished

![obsidian-polished logo](internal/uiassets/logo.png)

`obsidian-polished` exports Obsidian vaults into portable static HTML.

It supports:
- single-vault local export
- watch mode
- multi-notebook hosting from a `settings.yml`
- per-notebook git repos with periodic pull/reset/clean sync
- a hub `index.html` that links to each notebook

## Build

```bash
go build -o obsidian-polished ./cmd/obsidian-polished
```

## CLI

No arguments now shows help (same as `-h` / `--help`).

Single notebook:

```bash
./obsidian-polished \
  --vault /path/to/vault \
  --out /tmp/export
```

Watch with git pull:

```bash
./obsidian-polished \
  --vault /path/to/vault \
  --out /tmp/export \
  --watch \
  --watch-git-pull \
  --watch-git-pull-interval 5m
```

## Settings File

Run with config:

```bash
./obsidian-polished settings.yml
```

Or:

```bash
./obsidian-polished --config settings.yml
```

Flags override settings values.

Example `settings.yml`:

```yaml
out: ./site
watch: true
watch_poll: 2s
watch_debounce: 1s
theme: both

notebooks:
  - name: Team Notes
    description: Team docs and architecture notes
    git_repo: git@github.com:org/team-notes.git
    git_branch: main
    image: ./images/team-cover.jpg
    root_note: Home.md

  - name: Personal Vault
    description: Personal research and planning notes
    vault: /Users/you/Obsidian/Personal
    image: ./images/personal.jpg
```

Notebook metadata (`name`, optional `description`, `image`) is shown on the hub `index.html`.

If a notebook has `git_repo` and no `vault`, `obsidian-polished` clones it locally into `<out>/.repos/<slug>` and watches from there.
If `root_note` is omitted, that notebook exports from the vault root (all notes).

## Important Flags

- `--vault` vault root (single notebook mode)
- `--root-note` root note (omit to export all notes)
- `--out` output directory
- `--watch` run continuous watch mode
- `--watch-poll` watch polling interval
- `--watch-debounce` debounce window
- `--watch-git-pull` enable periodic git sync
- `--watch-git-pull-interval` git sync interval
- `--watch-git-branch` sync branch (`main`/`master` auto when empty)
- `--watch-git-remote` remote name (default `origin`)
- `--theme` `both|light|dark`
- `--css` custom stylesheet for notebook pages
- `--zip` / `--zip-path` zip output (single notebook only)

## Test

```bash
GOCACHE=/tmp/go-cache go test ./...
```
