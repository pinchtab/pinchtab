#!/bin/bash
# gosec scan used by both ./dev and CI. GOOS selects which build-constrained files
# are scanned; the gosec binary itself is always built for the host.
set -euo pipefail

cd "$(dirname "$0")/.."

EXCLUDED_RULES="G301,G302,G304,G306,G404,G107,G115,G703,G704,G705,G706"
CRITICAL_RULES='["G112","G204"]'
RESULTS=gosec-results.json
SCAN_GOOS="${GOOS:-$(go env GOHOSTOS)}"

echo "Running gosec (GOOS=${SCAN_GOOS})..."
START_TIME=$(date +%s)

GOSEC_BIN="$(mktemp -d)/gosec"
trap 'rm -rf "$(dirname "$GOSEC_BIN")"' EXIT
GOOS="$(go env GOHOSTOS)" GOARCH="$(go env GOHOSTARCH)" \
  go build -modfile=tools/go.mod -o "$GOSEC_BIN" github.com/securego/gosec/v2/cmd/gosec

rm -f "$RESULTS"
GOOS="$SCAN_GOOS" "$GOSEC_BIN" -quiet -no-fail -exclude="$EXCLUDED_RULES" -fmt=json -out="$RESULTS" ./...

if [ ! -f "$RESULTS" ]; then
  echo "❌ No results file generated (scan failed)"
  exit 1
fi

echo "Scan finished in $(($(date +%s) - START_TIME))s"
jq -r '.Stats' "$RESULTS"

CRITICAL_FILTER=".Issues[] | select(.rule_id as \$id | $CRITICAL_RULES | index(\$id))"
ISSUES=$(jq "[$CRITICAL_FILTER] | length" "$RESULTS")
echo "Critical issues ($CRITICAL_RULES, GOOS=${SCAN_GOOS}): $ISSUES"

if [ "$ISSUES" -gt 0 ]; then
  echo "❌ Critical gosec findings:"
  jq -r "$CRITICAL_FILTER" "$RESULTS"
  exit 1
fi

echo "✅ No critical issues"
TOTAL=$(jq '.Stats.found' "$RESULTS")
if [ "$TOTAL" -gt 0 ]; then
  echo "ℹ️  Non-critical findings (first 5, full report in $RESULTS):"
  jq -r '.Issues[0:5] | .[] | "  \(.severity): \(.rule_id) at \(.file):\(.line)"' "$RESULTS"
fi
