#!/usr/bin/env bash

DEFAULT_GOCACHE_MAX_GB=5

repo_cache_root() {
  local dir toplevel common
  dir="$(cd "$1" && pwd -P)"
  toplevel="$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null || true)"
  if [ -n "$toplevel" ] && [ "$(cd "$toplevel" && pwd -P)" = "$dir" ]; then
    common="$(git -C "$dir" rev-parse --path-format=absolute --git-common-dir)"
    (cd "$(dirname "$common")" && pwd -P)
  else
    echo "$dir"
  fi
}

use_repo_go_caches() {
  local root
  root="$(repo_cache_root "$1")"
  export GOCACHE="${GOCACHE:-$root/.gocache}"
  export GOTMPDIR="${GOTMPDIR:-$root/.tmp/go}"
  export GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-$root/.cache/golangci-lint}"
  mkdir -p "$GOCACHE" "$GOTMPDIR" "$GOLANGCI_LINT_CACHE"
}

repo_go_cache_max_gb() {
  local max_gb="${PINCHTAB_GOCACHE_MAX_GB:-$DEFAULT_GOCACHE_MAX_GB}"
  if [[ "$max_gb" =~ ^[0-9]+$ ]] && [ "$max_gb" -gt 0 ]; then
    echo "$max_gb"
    return
  fi
  echo "  ${MUTED:-}PINCHTAB_GOCACHE_MAX_GB=$max_gb is not a positive whole number, using $DEFAULT_GOCACHE_MAX_GB${NC:-}" >&2
  echo "$DEFAULT_GOCACHE_MAX_GB"
}

trim_repo_go_cache() {
  local root
  root="$(repo_cache_root "$1")"
  [ "$GOCACHE" = "$root/.gocache" ] || return 0
  [ -d "$GOCACHE" ] || return 0
  local max_gb size_kb
  max_gb="$(repo_go_cache_max_gb)"
  size_kb="$(du -sk "$GOCACHE" | cut -f1)"
  if [ "$size_kb" -gt $((max_gb * 1024 * 1024)) ]; then
    echo "  ${MUTED:-}Go build cache is $((size_kb / 1024)) MB (cap ${max_gb} GB), clearing $GOCACHE${NC:-}"
    # The dashboard's module cache under .gocache/mod is read-only and go clean -cache skips it.
    chmod -R u+w "${GOCACHE:?}"
    rm -rf "${GOCACHE:?}"
    mkdir -p "$GOCACHE"
  fi
}
