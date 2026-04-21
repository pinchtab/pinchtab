#!/bin/bash
# tabs-extended.sh — CLI advanced tab scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab back (no history)"

pt_ok back
# Terse mode outputs just the URL (or "OK" if no URL)
assert_output_contains "http" "back returns URL or OK"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab back (navigate two pages then back)"

pt_ok nav "${FIXTURES_URL}/index.html"
TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

pt_ok nav "${FIXTURES_URL}/form.html" --tab "$TAB_ID"

pt_ok back --tab "$TAB_ID"
assert_output_contains "index.html" "back returned to index.html"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab forward"

pt_ok forward --tab "$TAB_ID"
assert_output_contains "form.html" "forward returned to form.html"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab reload"

pt_ok reload --tab "$TAB_ID"
# Reload outputs "OK" in terse mode, pt_ok already asserts exit 0
assert_output_contains "OK" "reload outputs OK"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab (list)"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok tab --json
assert_output_json

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab returns valid JSON array"

pt_ok tab --json
assert_output_json "tabs output is valid JSON"
assert_output_contains "tabs" "response contains tabs field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab new + close roundtrip"

pt_ok nav "${FIXTURES_URL}/index.html"
TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

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
  pt_ok nav --new-tab "${FIXTURES_URL}/index.html?t=$i"
  TAB_IDS+=($(echo "$PT_OUT" | tr -d '[:space:]'))
done

pt_ok tab --json
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
pt_ok nav --new-tab "${FIXTURES_URL}/index.html?t=overflow"

pt_ok tab
assert_output_not_contains "$FIRST_TAB" "oldest tab evicted (LRU)"

end_test
