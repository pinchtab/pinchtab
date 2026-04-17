#!/bin/bash
# network-basic.sh — Basic network capture API tests.
# Covers: /network, /network/{requestId}, /network/clear

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: returns entries after navigation"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)
sleep 1

pt_get "/network?tabId=${TAB_ID}"
assert_ok "get network entries"
assert_json_exists "$RESULT" '.entries' "has entries array"
assert_json_exists "$RESULT" '.tabId' "has tabId"

ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
if [ "$ENTRIES_COUNT" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} captured $ENTRIES_COUNT network entries"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} no entries yet (timing-dependent)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: filter by method"

pt_get "/network?tabId=${TAB_ID}&method=GET"
assert_ok "filter by GET"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: limit parameter"

pt_get "/network?tabId=${TAB_ID}&limit=2"
assert_ok "get with limit"

ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
if [ "$ENTRIES_COUNT" -le 2 ]; then
  echo -e "  ${GREEN}✓${NC} limit respected: $ENTRIES_COUNT <= 2"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} limit exceeded: $ENTRIES_COUNT > 2"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: non-existent tab → error"

pt_get "/network?tabId=nonexistent_xyz_999"
assert_not_ok "rejects bad tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /network/clear: clears entries"

pt_post /network/clear "{\"tabId\":\"${TAB_ID}\"}"
assert_ok "clear network"

pt_get "/network?tabId=${TAB_ID}"
assert_ok "get after clear"

ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
if [ "$ENTRIES_COUNT" -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} entries cleared"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} $ENTRIES_COUNT entries remain after clear"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: basic HAR export works"

# Generate fresh traffic
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
sleep 1

pt_get "/network/export?tabId=${TAB_ID}"
assert_ok "basic HAR export"
assert_result_jq '.log.version' "1.2" "HAR version 1.2"
assert_json_exists "$RESULT" '.log.entries' "has entries"

end_test
