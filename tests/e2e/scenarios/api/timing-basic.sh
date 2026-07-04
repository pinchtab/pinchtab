#!/bin/bash
# timing-basic.sh — GET /timing navigation timing and Core Web Vitals tests.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

assert_json_number_gt() {
  local json="$1"
  local path="$2"
  local floor="$3"
  local desc="${4:-$path > $floor}"
  local actual
  actual=$(echo "$json" | jq "$path")
  if echo "$json" | jq -e "($path | type == \"number\") and ($path > $floor)" >/dev/null 2>&1; then
    pass_assert "$desc (got: $actual)"
  else
    fail_assert "$desc (got: $actual)"
  fi
}

TIMING_FIELDS=(ttfbMs fcpMs lcpMs cls domContentLoadedMs loadMs resourceCount transferSizeBytes)

# ─────────────────────────────────────────────────────────────────
start_test "GET /timing returns all metric fields after navigation"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/audit-site/clean.html\"}"
assert_ok "navigate to clean.html"

pt_get /timing
assert_ok "get timing"
assert_json_exists "$RESULT" '.tabId' "has tabId"
for field in "${TIMING_FIELDS[@]}"; do
  assert_json_exists "$RESULT" ".${field}" "has ${field}"
done

assert_json_number_gt "$RESULT" '.ttfbMs' 0 "ttfbMs > 0"
assert_json_number_gt "$RESULT" '.loadMs' 0 "loadMs > 0"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /timing on a page with an image reports paint metrics"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/audit-site/a11y-issues.html\"}"
assert_ok "navigate to a11y-issues.html"

pt_get /timing
assert_ok "get timing"
assert_json_number_gt "$RESULT" '.lcpMs' 0 "lcpMs > 0"
assert_json_exists "$RESULT" '.cls' "cls present"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /timing with tabId parameter"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/audit-site/clean.html\",\"newTab\":true}"
assert_ok "create new tab"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_get "/timing?tabId=${TAB_ID}"
assert_ok "get timing for specific tab"
assert_json_contains "$RESULT" '.tabId' "$TAB_ID" "returns correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /timing for nonexistent tab → error"

pt_get "/timing?tabId=nonexistent_xyz_999"
assert_not_ok "rejects bad tab"

end_test
