#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
source scripts/lib/term.sh
source scripts/lib/gocache.sh

ROOT="$(repo_cache_root "$PWD")"
WORKTREES_DIR="$ROOT/.tmp/worktrees"
MAX_AGE_MINUTES=1440

usage() {
  cat <<'USAGE'
Usage:
  ./dev worktree <name> [rev]   Detached worktree of rev (default HEAD) under .tmp/worktrees/
  ./dev worktree prune          Remove worktrees created more than 24h ago
USAGE
}

create_worktree() {
  local name="$1" rev="${2:-HEAD}"
  if [[ ! "$name" =~ ^[A-Za-z0-9._-]+$ ]] || [[ "$name" == .* ]]; then
    echo "  ${ERROR}Worktree name must match [A-Za-z0-9._-]+ and not start with a dot: $name${NC}" >&2
    exit 1
  fi
  local wt="$WORKTREES_DIR/$name"
  mkdir -p "$WORKTREES_DIR"
  git -C "$ROOT" worktree add --detach "$wt" "$rev" >&2
  if [ -d "$ROOT/dashboard/node_modules" ] && [ ! -e "$wt/dashboard/node_modules" ]; then
    ln -s "$ROOT/dashboard/node_modules" "$wt/dashboard/node_modules"
  fi
  use_repo_go_caches "$ROOT"
  ok "worktree ready; run Go through ./dev there, or export the shared caches first:" >&2
  echo "cd '$wt' && export GOCACHE='$GOCACHE' GOTMPDIR='$GOTMPDIR' GOLANGCI_LINT_CACHE='$GOLANGCI_LINT_CACHE'"
}

prune_worktrees() {
  [ -d "$WORKTREES_DIR" ] || return 0
  local marker wt
  while IFS= read -r marker; do
    wt="$(dirname "$marker")"
    git -C "$ROOT" worktree remove --force "$wt"
    ok "removed $wt"
  done < <(find "$WORKTREES_DIR" -mindepth 2 -maxdepth 2 -name .git -type f -mmin +"$MAX_AGE_MINUTES")
  git -C "$ROOT" worktree prune
}

case "${1:-}" in
  ""|-h|--help|help) usage ;;
  prune) prune_worktrees ;;
  *) create_worktree "$@" ;;
esac
