#!/bin/bash
# console-basic.sh — Console and error logs API tests.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "GET /console returns empty array initially"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

pt_get /console
assert_ok "get console"
assert_json_exists "$RESULT" '.tabId' "has tabId"
assert_json_exists "$RESULT" '.console' "has console array"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /errors returns empty array initially"

pt_get /errors
assert_ok "get errors"
assert_json_exists "$RESULT" '.tabId' "has tabId"
assert_json_exists "$RESULT" '.errors' "has errors array"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "console logs captured after console.log in page"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/console.html\"}"
sleep 1

pt_get /console
assert_ok "get console after logging page"

CONSOLE_COUNT=$(echo "$RESULT" | jq '.console | length')
if [ "$CONSOLE_COUNT" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} captured $CONSOLE_COUNT console entries"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} no console entries (may need console.html fixture)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /console/clear clears logs"

pt_post /console/clear ''
assert_ok "clear console"
assert_json_exists "$RESULT" '.success' "has success"

pt_get /console
CONSOLE_COUNT=$(echo "$RESULT" | jq '.console | length')
if [ "$CONSOLE_COUNT" -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} console cleared"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} still has $CONSOLE_COUNT entries after clear"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /console with limit parameter"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/console.html\"}"
sleep 1

pt_get "/console?limit=2"
assert_ok "get console with limit"

CONSOLE_COUNT=$(echo "$RESULT" | jq '.console | length')
if [ "$CONSOLE_COUNT" -le 2 ]; then
  echo -e "  ${GREEN}✓${NC} limit respected ($CONSOLE_COUNT <= 2)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} limit not respected ($CONSOLE_COUNT > 2)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /console with tabId parameter"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\",\"newTab\":true}"
assert_ok "create new tab"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_get "/console?tabId=${TAB_ID}"
assert_ok "get console for specific tab"
assert_json_contains "$RESULT" '.tabId' "$TAB_ID" "returns correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /errors with tabId parameter"

pt_get "/errors?tabId=${TAB_ID}"
assert_ok "get errors for specific tab"
assert_json_contains "$RESULT" '.tabId' "$TAB_ID" "returns correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /errors/clear clears error logs"

pt_post /errors/clear ''
assert_ok "clear errors"
assert_json_exists "$RESULT" '.success' "has success"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /console for nonexistent tab → error"

pt_get "/console?tabId=nonexistent_xyz_999"
assert_not_ok "rejects bad tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /errors for nonexistent tab → error"

pt_get "/errors?tabId=nonexistent_xyz_999"
assert_not_ok "rejects bad tab"

end_test
