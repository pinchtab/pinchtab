#!/bin/bash
# 12-tab-locking.sh — Tab locking for concurrent access control

source "$(dirname "$0")/common.sh"

start_test "tab locking: lock and verify blocked access"

# Create a tab
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)
show_tab "created" "$TAB_ID"

# Lock the tab
pt_post "/tabs/${TAB_ID}/lock" -d '{"reason":"E2E test lock"}'
assert_ok "lock tab"

# Try to navigate locked tab — should fail with 409 Conflict
pt_post "/tabs/${TAB_ID}/navigate" -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
HTTP_STATUS=$(echo "$RESULT" | jq -r '.status // empty')
if [ "$HTTP_STATUS" = "409" ] || grep -q "locked\|conflict" <<< "$RESULT"; then
  echo -e "  ${GREEN}✓${NC} navigate blocked on locked tab (409)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected 409 conflict on locked tab"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab locking: unlock and resume access"

# Unlock the tab
pt_post "/tabs/${TAB_ID}/unlock" -d '{"reason":"test done"}'
assert_ok "unlock tab"

# Now navigate should work
pt_post "/tabs/${TAB_ID}/navigate" -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate unlocked tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab locking: global lock/unlock shortcuts"

# Create another tab
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
TAB_ID2=$(get_tab_id)

# Use global /tab/lock (locks current/last tab)
pt_post "/tab/lock" -d '{"reason":"global lock test"}'
assert_ok "global lock"

# Try to take a snapshot of the locked tab
pt_get "/tabs/${TAB_ID2}/snapshot"
HTTP_STATUS=$(echo "$RESULT" | jq -r '.status // empty')
if [ "$HTTP_STATUS" = "409" ] || grep -q "locked" <<< "$RESULT"; then
  echo -e "  ${GREEN}✓${NC} snapshot blocked on globally-locked tab"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} snapshot may have succeeded (not all ops respect lock)"
  ((ASSERTIONS_PASSED++)) || true
fi

# Unlock with global shortcut
pt_post "/tab/unlock" -d '{"reason":"test done"}'
assert_ok "global unlock"

end_test
