#!/usr/bin/env bash

DEFAULT_GOCACHE_MAX_GB=5

use_repo_go_caches() {
  local repo="$1"
  export GOCACHE="${GOCACHE:-$repo/.gocache}"
  export GOTMPDIR="${GOTMPDIR:-$repo/.tmp/go}"
  mkdir -p "$GOCACHE" "$GOTMPDIR"
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
  local repo="$1"
  [ "$GOCACHE" = "$repo/.gocache" ] || return 0
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
