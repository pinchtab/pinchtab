#!/bin/bash
# audit-cli-basic.sh — pinchtab audit CLI happy path.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

OUT_DIR=$(mktemp -d /tmp/audit-cli.XXXXXX)

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab audit <url> --output-dir writes report and screenshots"

pt_ok audit "${FIXTURES_URL}/audit-site/index.html" --output-dir "$OUT_DIR"

REPORT="$OUT_DIR/report.json"
if [ -f "$REPORT" ]; then
  pass_assert "report.json exists"
else
  fail_assert "report.json exists"
fi

if jq -e '.schemaVersion' "$REPORT" >/dev/null 2>&1; then
  pass_assert "schemaVersion set ($(jq -r '.schemaVersion' "$REPORT"))"
else
  fail_assert "schemaVersion set"
fi

if jq -e '.pages | length == 1' "$REPORT" >/dev/null 2>&1; then
  pass_assert "one page entry"
else
  fail_assert "one page entry (got: $(jq '.pages | length' "$REPORT"))"
fi

if jq -e '.pages[0].browser.screenshotPath' "$REPORT" >/dev/null 2>&1; then
  pass_assert "page entry has screenshotPath"
else
  fail_assert "page entry has screenshotPath"
fi

PNG_COUNT=$(ls "$OUT_DIR/screenshots/"*.png 2>/dev/null | wc -l | tr -d ' ')
if [ "$PNG_COUNT" -ge 1 ]; then
  pass_assert "screenshots/*.png exist ($PNG_COUNT)"
else
  fail_assert "screenshots/*.png exist"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab audit --sitemap audits >=10 discovered pages"

pt_ok audit "${FIXTURES_URL}/audit-site/sitemap.xml" --sitemap --screenshot=false --concurrency 3 --json
PAGE_COUNT=$(echo "$PT_OUT" | jq '.pages | length' 2>/dev/null)
if [ "${PAGE_COUNT:-0}" -ge 10 ] 2>/dev/null; then
  pass_assert "audited $PAGE_COUNT pages from sitemap"
else
  fail_assert "audited >=10 pages from sitemap (got: $PAGE_COUNT)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "run with unreachable URL still exits 0 with error entry"

pt_ok audit "http://fixtures:9/down.html" --screenshot=false --json
ERR=$(echo "$PT_OUT" | jq -r '.pages[0].error // empty' 2>/dev/null)
if [ -n "$ERR" ]; then
  pass_assert "page entry carries error ($ERR)"
else
  fail_assert "page entry carries error"
fi

end_test

rm -rf "$OUT_DIR"
