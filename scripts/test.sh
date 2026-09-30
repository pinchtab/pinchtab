#!/bin/bash
set -e

# test.sh — Run Go tests with optional scope
# Usage: test.sh [unit|all]
# Default: all

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
MUTED=$'\033[38;2;90;100;128m'
NC=$'\033[0m'

SYSTEM_REGEX='^(TestProxy_InstanceIsolation|TestOrchestrator_(HealthCheck|HashBasedIDs|PortAllocation|PortReuse|ListInstances|FirstRequestLazyChrome|AggregateTabsEndpoint|StopNonexistent|InstanceCleanup))$'

ok()   { echo -e "  ${SUCCESS}✓${NC} $1"; }
fail() { echo -e "  ${ERROR}✗${NC} $1"; }

section() {
  echo ""
  echo -e "  ${ACCENT}${BOLD}$1${NC}"
}

# print_skipped_tests lists every skipped test with the reason it gave, so the
# skip list is reviewable: "skipped because Windows-only" and "skipped because we
# could not find a browser that is in fact installed" otherwise look identical.
# The reason is the last file:line log line the test emitted before it skipped,
# which is exactly what t.Skip writes.
print_skipped_tests() {
  local json_file="$1"

  [ ! -s "$json_file" ] && return

  local lines
  lines="$(
  jq -r '
    select(.Test != null and (.Action == "skip" or .Action == "output"))
    | [.Package, .Test, .Action, ((.Output // "") | gsub("\r?\n$"; ""))] | @tsv
  ' "$json_file" \
    | awk -F'\t' '
        {
          key = $1 "\t" $2
          if ($3 == "output") {
            line = $4
            sub(/^[[:space:]]+/, "", line)
            if (line ~ /\.go:[0-9]+: /) { reason[key] = line }
          } else if ($3 == "skip") {
            skipped[key] = 1
          }
        }
        END {
          for (k in skipped) {
            r = reason[k]
            sub(/^[^ ]*\.go:[0-9]+: /, "", r)
            if (r == "") { r = "(no reason given)" }
            split(k, parts, "\t")
            printf "      - %s %s — %s\n", parts[1], parts[2], r
          }
        }' \
    | sort

  )"

  [ -z "$lines" ] && return

  echo ""
  echo -e "    ${ACCENT}Skipped tests:${NC}"
  printf '%s\n' "$lines"
}

SCOPE="${1:-all}"
TMPDIR_TEST=$(mktemp -d)
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# ── Unit tests ───────────────────────────────────────────────────────

if [ "$SCOPE" = "all" ] || [ "$SCOPE" = "unit" ]; then
  section "test:🔬:go unit"

  UNIT_JSON="$TMPDIR_TEST/unit.json"
  # Hides the skipped section only because print_skipped_tests renders it below with
  # each skip reason attached. Never the output section: TestNoGotestsumInvocationHides-
  # TheOutputSummary enforces that for every invocation in the repo.
  if ! go tool -modfile=tools/go.mod gotestsum --format=pkgname --hide-summary=skipped --jsonfile "$UNIT_JSON" -- -p 1 -count=1 ./...; then
    fail "test:🔬:go unit"
    print_skipped_tests "$UNIT_JSON"
    exit 1
  fi
  print_skipped_tests "$UNIT_JSON"
  ok "test:🔬:go unit"
fi

# ── Dashboard ────────────────────────────────────────────────────────

if [ "$SCOPE" = "all" ] || [ "$SCOPE" = "dashboard" ]; then
  section "test:🔬:dashboard"
  ./scripts/test-dashboard.sh
fi

# ── Plugin ───────────────────────────────────────────────────────────

if [ "$SCOPE" = "all" ] || [ "$SCOPE" = "plugin" ]; then
  section "test:🔬:plugin"
  cd plugins/openclaw && npx tsx --test *.test.ts
  cd ../..
  ok "test:🔬:plugin"
fi

# ── Summary ──────────────────────────────────────────────────────────

section "Summary"
echo ""
echo -e "  ${SUCCESS}${BOLD}All tests passed!${NC}"
echo ""
