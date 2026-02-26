#!/bin/sh
set -eu

is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

OBS_VAULT="${OBS_VAULT:-/data/vault}"
OBS_OUT="${OBS_OUT:-/data/out}"
OBS_THEME="${OBS_THEME:-both}"
OBS_MAX_DEPTH="${OBS_MAX_DEPTH:--1}"
OBS_ROOT_NOTE="${OBS_ROOT_NOTE:-}"
OBS_CSS="${OBS_CSS:-}"
OBS_WATCH="${OBS_WATCH:-true}"
OBS_WATCH_POLL="${OBS_WATCH_POLL:-2s}"
OBS_WATCH_DEBOUNCE="${OBS_WATCH_DEBOUNCE:-1s}"
OBS_GIT_SYNC="${OBS_GIT_SYNC:-false}"
OBS_GIT_PULL_INTERVAL="${OBS_GIT_PULL_INTERVAL:-5m}"
OBS_GIT_REMOTE="${OBS_GIT_REMOTE:-origin}"
OBS_GIT_BRANCH="${OBS_GIT_BRANCH:-}"
OBS_SERVE_STATIC="${OBS_SERVE_STATIC:-false}"
OBS_HTTP_PORT="${OBS_HTTP_PORT:-8080}"
OBS_AUTH_ENABLED="${OBS_AUTH_ENABLED:-false}"
OBS_AUTH_USER="${OBS_AUTH_USER:-}"
OBS_AUTH_PASSWORD="${OBS_AUTH_PASSWORD:-}"

mkdir -p "$OBS_OUT"

set -- /usr/local/bin/obsidian-polished \
  --vault "$OBS_VAULT" \
  --out "$OBS_OUT" \
  --theme "$OBS_THEME" \
  --max-depth "$OBS_MAX_DEPTH"

if [ -n "$OBS_ROOT_NOTE" ]; then
  set -- "$@" --root-note "$OBS_ROOT_NOTE"
fi
if [ -n "$OBS_CSS" ]; then
  set -- "$@" --css "$OBS_CSS"
fi
if is_true "$OBS_WATCH"; then
  set -- "$@" --watch --watch-poll "$OBS_WATCH_POLL" --watch-debounce "$OBS_WATCH_DEBOUNCE"
  if is_true "$OBS_GIT_SYNC"; then
    set -- "$@" --watch-git-pull --watch-git-pull-interval "$OBS_GIT_PULL_INTERVAL" --watch-git-remote "$OBS_GIT_REMOTE"
    if [ -n "$OBS_GIT_BRANCH" ]; then
      set -- "$@" --watch-git-branch "$OBS_GIT_BRANCH"
    fi
  fi
fi

if is_true "$OBS_SERVE_STATIC"; then
  AUTH_BLOCK=""
  if is_true "$OBS_AUTH_ENABLED"; then
    if [ -z "$OBS_AUTH_USER" ] || [ -z "$OBS_AUTH_PASSWORD" ]; then
      echo "OBS_AUTH_ENABLED=true requires OBS_AUTH_USER and OBS_AUTH_PASSWORD" >&2
      exit 1
    fi
    HASH="$(openssl passwd -apr1 "$OBS_AUTH_PASSWORD")"
    printf '%s:%s\n' "$OBS_AUTH_USER" "$HASH" > /etc/nginx/.htpasswd
    AUTH_BLOCK='auth_basic "Restricted Notes";\n    auth_basic_user_file /etc/nginx/.htpasswd;'
  fi

  sed "s|__PORT__|$OBS_HTTP_PORT|g; s|__ROOT__|$OBS_OUT|g; s|__AUTH_BLOCK__|$AUTH_BLOCK|g" \
    /etc/nginx/http.d/default.conf.template > /etc/nginx/http.d/default.conf

  "$@" &
  app_pid=$!

  cleanup() {
    kill "$app_pid" 2>/dev/null || true
  }
  trap cleanup INT TERM EXIT

  exec nginx -g 'daemon off;'
fi

exec "$@"
