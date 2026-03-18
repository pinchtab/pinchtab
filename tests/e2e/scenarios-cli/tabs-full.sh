#!/bin/bash
# tabs-full.sh — CLI advanced tab scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab back (no history)"

pt_ok back
assert_output_json "back returns JSON"
assert_output_contains "tabId" "response contains tabId"
assert_output_contains "url" "response contains url"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab back (navigate two pages then back)"

pt_ok nav "${FIXTURES_URL}/index.html"
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')
URL_A=$(echo "$PT_OUT" | jq -r '.url')

pt_ok nav "${FIXTURES_URL}/form.html" --tab "$TAB_ID"
URL_B=$(echo "$PT_OUT" | jq -r '.url')

pt_ok back --tab "$TAB_ID"
assert_output_json "back returns JSON"
assert_output_contains "index.html" "back returned to index.html"
assert_output_contains "$TAB_ID" "back response contains correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab forward"

pt_ok forward --tab "$TAB_ID"
assert_output_json "forward returns JSON"
assert_output_contains "form.html" "forward returned to form.html"
assert_output_contains "$TAB_ID" "forward response contains correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab reload"

pt_ok reload --tab "$TAB_ID"
assert_output_json "reload returns JSON"
assert_output_contains "form.html" "reload kept same page"
assert_output_contains "$TAB_ID" "reload response contains correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab (list)"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok tab
assert_output_json

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab returns valid JSON array"

pt_ok tab
assert_output_json "tabs output is valid JSON"
assert_output_contains "tabs" "response contains tabs field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab new + close roundtrip"

pt_ok nav "${FIXTURES_URL}/index.html"
assert_output_json
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')

pt_ok tab close "$TAB_ID"

pt_ok tab
assert_output_not_contains "$TAB_ID" "closed tab no longer in list"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab close with no args → error"

pt_fail tab close

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab close nonexistent → error"

pt_fail tab close "nonexistent_tab_id_12345"

end_test

MAX_TABS=10

# ─────────────────────────────────────────────────────────────────
start_test "tab eviction: open tabs up to limit"

TAB_IDS=()
for i in $(seq 1 $MAX_TABS); do
  pt_ok nav "${FIXTURES_URL}/index.html?t=$i"
  TAB_IDS+=($(echo "$PT_OUT" | jq -r '.tabId'))
done

pt_ok tab
TAB_COUNT=$(echo "$PT_OUT" | jq '.tabs | length')
if [ "$TAB_COUNT" -ge "$MAX_TABS" ]; then
  echo -e "  ${GREEN}✓${NC} $TAB_COUNT tabs open (>= $MAX_TABS)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected >= $MAX_TABS tabs, got $TAB_COUNT"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab eviction: new tab evicts oldest"

FIRST_TAB="${TAB_IDS[0]}"
sleep 1
pt_ok nav "${FIXTURES_URL}/index.html?t=overflow"

pt_ok tab
assert_output_not_contains "$FIRST_TAB" "oldest tab evicted (LRU)"

end_test
