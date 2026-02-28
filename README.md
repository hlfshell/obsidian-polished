# obsidian-polished

![obsidian-polished logo](internal/uiassets/logo.png)

`obsidian-polished` is a <span style="font-weight:bold; background: linear-gradient(90deg, red, orange, yellow, green, blue, indigo, violet); background-size: 400% 400%; -webkit-background-clip: text; -webkit-text-fill-color: transparent; animation: rainbow-text-animate 3s ease-in-out infinite;">vibe coded</span> project - so take that as you will. It is a small set of tooling to export your Obsidian vaults into portable static HTML. It also has the ability to watch for changes locally or remotely via git so a site can stay up to date with your notes.

<style>
@keyframes rainbow-text-animate {
  0% {
    background-position: 0% 50%;
  }
  50% {
    background-position: 100% 50%;
  }
  100% {
    background-position: 0% 50%;
  }
}
</style>

## Install

Install `just` first:

```bash
# macOS
brew install just

# Debian/Ubuntu
sudo apt-get install just
```

Then run:

```bash
just help
```

## Build

With `just`:

```bash
just build-go
just build
```

Manually:

```bash
mkdir -p bin
go build -o bin/obsidian-polished ./cmd/obsidian-polished
docker build -f Dockerfile -t hlfshell/obsidian-polished:latest .
```

## Just Recipes

Use `just help` to see all commands.

Build Go binaries from every `cmd/*` package to `bin/<cmd-name>`:

```bash
just build-go
```

Build Docker image:

```bash
just build-docker
just build-docker hlfshell/obsidian-polished:dev
```

Build and publish a Docker Hub image from the current git tag (fails if `HEAD` is not exactly tagged):

```bash
just docker-publish
```

Build both Go binaries and Docker image:

```bash
just build
```

Docker Compose helpers (default file is `docker-compose.yml`):

```bash
just compose-build
just compose-up
just compose-down
```

Use a different compose file and pass extra args:

```bash
just compose-up docker-compose.yml --pull always
just compose-down docker-compose.yml --remove-orphans
just compose docker-compose.yml logs -f
```

If you use private SSH git remotes in Docker/Compose, make sure the container has:
- `ssh` client available (installed in this image)
- `OBS_GIT_SSH_KEY` set to an in-container key path (for example `/root/.ssh/id_ed25519`)
- SSH key and `known_hosts` mounted read-only into the container

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
  --watch-git-pull-interval 5m \
  --git-ssh-key ~/.ssh/id_ed25519
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
git_ssh_key: ~/.ssh/id_ed25519

notebooks:
  - name: Team Notes
    description: Team docs and architecture notes
    git_repo: git@github.com:org/team-notes.git
    git_branch: main
    git_ssh_key: ~/.ssh/id_team_notes
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
- `--git-ssh-key` SSH private key file for git clone/fetch/pull
- `--theme` `both|light|dark`
- `--css` custom stylesheet for notebook pages
- `--zip` / `--zip-path` zip output (single notebook only)

## Test

```bash
GOCACHE=/tmp/go-cache go test ./...
```
