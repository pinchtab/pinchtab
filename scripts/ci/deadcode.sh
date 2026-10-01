#!/usr/bin/env bash
set -euo pipefail

deadcode_module="golang.org/x/tools/cmd/deadcode@v0.50.0"

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
allow_file="$repo_root/scripts/ci/deadcode-allow.txt"
cd "$repo_root"

report=$(go run "$deadcode_module" ./cmd/...)
found=$(printf '%s\n' "$report" | sed -nE 's/^([^:]+):[0-9]+:[0-9]+: unreachable func: (.+)$/\1:\2/p' | sort -u)
allowed=$(grep -v '^[[:space:]]*$' "$allow_file" | sort -u)

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### Unreachable funcs ($(printf '%s' "$found" | grep -c . || true))"
    echo '```'
    printf '%s\n' "$report"
    echo '```'
  } >> "$GITHUB_STEP_SUMMARY"
fi

new=$(comm -23 <(printf '%s\n' "$found") <(printf '%s\n' "$allowed") | grep . || true)
stale=$(comm -13 <(printf '%s\n' "$found") <(printf '%s\n' "$allowed") | grep . || true)

status=0
if [ -n "$new" ]; then
  echo "deadcode: unreachable funcs not in scripts/ci/deadcode-allow.txt; delete them, or call them from production code:" >&2
  printf '  %s\n' $new >&2
  status=1
fi
if [ -n "$stale" ]; then
  echo "deadcode: scripts/ci/deadcode-allow.txt lists funcs that are no longer unreachable; delete these entries so the list keeps shrinking:" >&2
  printf '  %s\n' $stale >&2
  status=1
fi
exit "$status"
