set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

default: help

bin_dir := "bin"
compose_file := "docker-compose.yml"
docker_image := "obsidian-polished:latest"

help:
    @echo "🧰 Available just recipes:"
    @echo "  help                     - 📖 Show this help"
    @echo "  build-go                 - 🛠️  Build all Go binaries from cmd/* into bin/"
    @echo "  build-docker [tag]       - 🐳 Build Docker image (default tag: {{docker_image}})"
    @echo "  build                    - 🚀 Build Go binaries and Docker image"
    @echo "  compose-build [file]     - 🏗️  Build services from a compose file"
    @echo "  compose-up [file] [args] - ▶️  Start compose services with optional extra args"
    @echo "  compose-down [file] [args] - ⏹️  Stop compose services with optional extra args"
    @echo "  compose [file] [args]    - 🔧 Pass-through to docker compose"
    @echo "  docker-publish [repo]    - 🚢 Build/push repo:<git-tag> (fails if HEAD is untagged)"

build-go:
    @mkdir -p {{bin_dir}}
    @mkdir -p /tmp/go-cache
    @mkdir -p /tmp/go-mod-cache
    @mkdir -p /tmp/go
    @for d in cmd/*; do \
      if [ -d "$d" ]; then \
        name="$(basename "$d")"; \
        echo "🛠️  building $name -> {{bin_dir}}/$name"; \
        GOCACHE=/tmp/go-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go go build -o "{{bin_dir}}/$name" "./$d"; \
      fi; \
    done

build-docker tag=docker_image:
    @echo "🐳 building image {{tag}}"
    docker build -f Dockerfile -t "{{tag}}" .

build: build-go build-docker

compose-build file="{{compose_file}}":
    docker compose -f "{{file}}" build

compose-up file="{{compose_file}}" *args:
    docker compose -f "{{file}}" up -d {{args}}

compose-down file="{{compose_file}}" *args:
    docker compose -f "{{file}}" down {{args}}

compose file="{{compose_file}}" *args:
    docker compose -f "{{file}}" {{args}}

docker-publish repo="":
    @if [ -z "{{repo}}" ]; then \
      echo "❌ missing Docker Hub repo. Usage: just docker-publish <dockerhub-user/repo>"; \
      exit 1; \
    fi
    @tag="$$(git describe --tags --exact-match 2>/dev/null || true)"; \
      if [ -z "$$tag" ]; then \
        echo "❌ HEAD is not on a git tag. Tag this commit, then retry."; \
        exit 1; \
      fi; \
      image="{{repo}}:$$tag"; \
      echo "🏷️  tag: $$tag"; \
      echo "🐳 building $$image"; \
      docker build -f Dockerfile -t "$$image" .; \
      echo "🚢 pushing $$image"; \
      docker push "$$image"
