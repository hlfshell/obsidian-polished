# obsidian-polished

`obsidian-polished` is a standalone Go CLI that exports Obsidian markdown notes into a portable static HTML site.

## What It Does

- Exports either:
  - all notes in a vault, or
  - a root note plus linked notes (recursive traversal)
- Preserves wiki links and embedded media in static HTML output.
- Adds note metadata cards on each page with created/updated timestamps.
- Supports `light`, `dark`, or toggleable `both` theme modes.
- Includes watch mode with debounced regeneration and generation locking.
- Optionally syncs a git-backed vault on a schedule (`fetch + hard reset + clean`).
- Can run in Docker and optionally serve static output via nginx with optional basic auth.

## Build

```bash
go build -o obsidian-polished ./cmd/obsidian-polished
```

## CLI Usage

Basic export:

```bash
./obsidian-polished \
  --vault /path/to/vault \
  --out /tmp/export
```

Export from a single root note:

```bash
./obsidian-polished \
  --vault /path/to/vault \
  --root-note "Home.md" \
  --out /tmp/export
```

Watch mode:

```bash
./obsidian-polished \
  --vault /path/to/vault \
  --out /tmp/export \
  --watch
```

Watch mode with git sync every 5 minutes:

```bash
./obsidian-polished \
  --vault /path/to/vault \
  --out /tmp/export \
  --watch \
  --watch-git-pull \
  --watch-git-pull-interval 5m \
  --watch-git-branch main
```

## Important Flags

- `--vault` Vault root (default `.`)
- `--root-note` Root note filename/path (optional)
- `--out` Output directory (default `./html_export`)
- `--max-depth` Traversal depth from root (`-1` unlimited)
- `--theme` `both|light|dark` (default `both`)
- `--css` Optional CSS override file
- `--zip` Emit zip archive instead of folder
- `--zip-path` Zip destination path (default `<out>.zip`)
- `--watch` Run continuously and regenerate on changes
- `--watch-poll` Watch polling interval (default `2s`)
- `--watch-debounce` Debounce window (default `1s`)
- `--watch-git-pull` Enable periodic git sync for git repos
- `--watch-git-pull-interval` Git sync interval (default `5m`)
- `--watch-git-branch` Force sync branch (default auto `main` then `master`)
- `--watch-git-remote` Remote name (default `origin`)

## Docker

Build image:

```bash
docker build -t obsidian-polished:latest .
```

Run exporter loop:

```bash
docker run --rm \
  -v /path/to/vault:/data/vault \
  -v /path/to/output:/data/out \
  -e OBS_WATCH=true \
  obsidian-polished:latest
```

Run with nginx static serving:

```bash
docker run --rm -p 8080:8080 \
  -v /path/to/vault:/data/vault \
  -v /path/to/output:/data/out \
  -e OBS_SERVE_STATIC=true \
  obsidian-polished:latest
```

Enable basic auth:

```bash
docker run --rm -p 8080:8080 \
  -v /path/to/vault:/data/vault \
  -v /path/to/output:/data/out \
  -e OBS_SERVE_STATIC=true \
  -e OBS_AUTH_ENABLED=true \
  -e OBS_AUTH_USER=notes \
  -e OBS_AUTH_PASSWORD='strong-password' \
  obsidian-polished:latest
```

## Docker Compose

Start:

```bash
docker compose up -d --build
```

Use the included `docker-compose.yml` and adjust:

- `./vault` and `./output` volume paths
- `OBS_GIT_SYNC`/branch/interval settings
- `OBS_AUTH_*` values when enabling auth

## Test

```bash
GOCACHE=/tmp/go-cache go test ./...
```
