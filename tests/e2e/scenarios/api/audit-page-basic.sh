#!/bin/bash
# audit-page-basic.sh — POST /audit/page single-page browser enrichment.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "broken-assets page: broken entries and network requests"

pt_post /audit/page -d "{\"url\":\"${FIXTURES_URL}/audit-site/broken-assets.html\"}"
assert_ok "audit broken-assets.html"
assert_json_length "$RESULT" '.brokenAssets' 4 "4 broken entries (3 assets + fetch)"
assert_json_length_gte "$RESULT" '.networkRequests' 1 "non-empty networkRequests"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "console-errors page: error and warning console entries"

pt_post /audit/page -d "{\"url\":\"${FIXTURES_URL}/audit-site/console-errors.html\"}"
assert_ok "audit console-errors.html"

for level in error warning; do
  if echo "$RESULT" | jq -e --arg level "$level" '.consoleLogs[] | select(.level == $level)' >/dev/null 2>&1; then
    pass_assert "consoleLogs contain a $level entry"
  else
    fail_assert "consoleLogs contain a $level entry (got: $(echo "$RESULT" | jq -c '[.consoleLogs[]?.level]'))"
  fi
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "a11y-issues page: accessibility score below 100"

pt_post /audit/page -d "{\"url\":\"${FIXTURES_URL}/audit-site/a11y-issues.html\"}"
assert_ok "audit a11y-issues.html"

SCORE=$(echo "$RESULT" | jq '.accessibilityScore')
if [ "$SCORE" -lt 100 ] 2>/dev/null; then
  pass_assert "accessibilityScore < 100 (got: $SCORE)"
else
  fail_assert "accessibilityScore < 100 (got: $SCORE)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "clean page: full collector set populated"

pt_post /audit/page -d "{\"url\":\"${FIXTURES_URL}/audit-site/clean.html\"}"
assert_ok "audit clean.html"

BROKEN=$(echo "$RESULT" | jq '.brokenAssets | length')
if [ "$BROKEN" -eq 0 ]; then
  pass_assert "zero brokenAssets"
else
  fail_assert "zero brokenAssets (got: $BROKEN)"
fi
assert_json_eq "$RESULT" '.accessibilityScore' "100" "accessibility score 100"

LOAD_MS=$(echo "$RESULT" | jq '.timingMetrics.loadMs')
if echo "$RESULT" | jq -e '.timingMetrics.loadMs > 0' >/dev/null 2>&1; then
  pass_assert "timingMetrics.loadMs > 0 (got: $LOAD_MS)"
else
  fail_assert "timingMetrics.loadMs > 0 (got: $LOAD_MS)"
fi

assert_json_length_gte "$RESULT" '.interactiveElements' 1 "at least one interactiveElement"

SCREENSHOT_LEN=$(echo "$RESULT" | jq '.screenshot | length')
if [ "$SCREENSHOT_LEN" -gt 0 ] 2>/dev/null; then
  pass_assert "non-empty screenshot (${SCREENSHOT_LEN} base64 chars)"
else
  fail_assert "non-empty screenshot (got length: $SCREENSHOT_LEN)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "options toggles omit screenshot and network fields"

pt_post /audit/page -d "{\"url\":\"${FIXTURES_URL}/audit-site/clean.html\",\"options\":{\"screenshot\":false,\"network\":false}}"
assert_ok "audit with disabled collectors"

for absent in screenshot networkRequests; do
  if echo "$RESULT" | jq -e "has(\"$absent\") | not" >/dev/null 2>&1; then
    pass_assert "$absent omitted"
  else
    fail_assert "$absent omitted (got: $(echo "$RESULT" | jq -c ".$absent | length"))"
  fi
done

assert_json_eq "$RESULT" '.accessibilityScore' "100" "other collectors still run"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "unreachable URL: structured error entry, server healthy"

pt_post /audit/page -d "{\"url\":\"http://fixtures:9/unreachable.html\"}"
assert_ok "audit unreachable URL returns 200"
assert_json_exists "$RESULT" '.error' "has error field"
assert_json_contains "$RESULT" '.url' "http://fixtures:9/unreachable.html" "echoes the url"

pt_get /health
assert_ok "server healthy after failed audit"

end_test
