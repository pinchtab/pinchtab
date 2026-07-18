#!/bin/bash
# compare-basic.sh — pinchtab compare live-vs-staging happy path.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

LIVE="http://fixtures/audit-site/"
STAGING="http://fixtures/audit-site-staging/"
OUT_DIR=$(mktemp -d /tmp/compare-cli.XXXXXX)

# ─────────────────────────────────────────────────────────────────
start_test "compare writes report and annotated diff for changed page"

pt_ok compare "$LIVE" "$STAGING" --pages index.html,clean.html --output-dir "$OUT_DIR"

REPORT="$OUT_DIR/report.json"
if [ -f "$REPORT" ]; then
  pass_assert "report.json exists"
else
  fail_assert "report.json exists"
fi

CLEAN=$(jq '.pages[] | select(.path == "clean.html")' "$REPORT" 2>/dev/null)
if echo "$CLEAN" | jq -e '.diffPercentage == 0' >/dev/null 2>&1; then
  pass_assert "clean.html diffPercentage == 0"
else
  fail_assert "clean.html diffPercentage == 0 (got: $(echo "$CLEAN" | jq '.diffPercentage'))"
fi
if echo "$CLEAN" | jq -e '.drift | length == 0' >/dev/null 2>&1; then
  pass_assert "clean.html has zero data-drift entries"
else
  fail_assert "clean.html has zero data-drift entries (got: $(echo "$CLEAN" | jq -c '.drift'))"
fi

INDEX=$(jq '.pages[] | select(.path == "index.html")' "$REPORT" 2>/dev/null)
if echo "$INDEX" | jq -e '.diffPercentage > 0' >/dev/null 2>&1; then
  pass_assert "index.html diffPercentage > 0 ($(echo "$INDEX" | jq '.diffPercentage'))"
else
  fail_assert "index.html diffPercentage > 0 (got: $(echo "$INDEX" | jq '.diffPercentage'))"
fi

DIFF_IMAGE=$(echo "$INDEX" | jq -r '.diffImagePath // empty')
if [ -n "$DIFF_IMAGE" ] && [ -f "$OUT_DIR/$DIFF_IMAGE" ]; then
  pass_assert "annotated diff image on disk ($DIFF_IMAGE)"
else
  fail_assert "annotated diff image on disk (path: $DIFF_IMAGE)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "--fail-on-diff exits non-zero when pages differ"

pt_fail compare "$LIVE" "$STAGING" --pages index.html --fail-on-diff

end_test

# ─────────────────────────────────────────────────────────────────
start_test "--fail-on-diff exits 0 when pages are identical"

pt_ok compare "$LIVE" "$STAGING" --pages clean.html --fail-on-diff

end_test

rm -rf "$OUT_DIR"
