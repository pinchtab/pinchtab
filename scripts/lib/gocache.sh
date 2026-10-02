#!/usr/bin/env bash

use_repo_go_caches() {
  local repo="$1"
  export GOCACHE="${GOCACHE:-$repo/.gocache}"
  export GOTMPDIR="${GOTMPDIR:-$repo/.tmp/go}"
  mkdir -p "$GOCACHE" "$GOTMPDIR"
}

trim_repo_go_cache() {
  local repo="$1"
  local max_gb="${PINCHTAB_GOCACHE_MAX_GB:-5}"
  [ "$GOCACHE" = "$repo/.gocache" ] || return 0
  [ -d "$GOCACHE" ] || return 0
  local size_kb
  size_kb="$(du -sk "$GOCACHE" | cut -f1)"
  if [ "$size_kb" -gt $((max_gb * 1024 * 1024)) ]; then
    echo "  ${MUTED:-}Go build cache is $((size_kb / 1024)) MB (cap ${max_gb} GB), clearing $GOCACHE${NC:-}"
    go clean -cache
  fi
}
