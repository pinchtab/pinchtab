#!/bin/bash
# Test: Navigation history (back, forward, reload)
# Tests for `POST /back`, `POST /forward`, `POST /reload` commands
source "$(dirname "$0")/common.sh"

# --- T1: Navigate back to previous page ---
start_test "Navigate back to previous page"

# Navigate to page A (index.html)
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate to index.html"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId // empty')
assert_tab_id "get tab ID"
show_tab "TAB_ID" "$TAB_ID"
URL_A=$(echo "$RESULT" | jq -r '.url // empty')
echo -e "  ${MUTED}URL_A: $URL_A${NC}"

# Navigate same tab to page B (form.html) — must pass tabId to reuse tab
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "navigate to form.html"
URL_B=$(echo "$RESULT" | jq -r '.url // empty')
echo -e "  ${MUTED}URL_B: $URL_B${NC}"

# Go back — should return to page A
pt_post "/back?tabId=${TAB_ID}" ''
assert_ok "POST /back"
RESULT_URL=$(echo "$RESULT" | jq -r '.url // empty')
assert_json_contains "$RESULT" ".url" "index.html" "back returned to index.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "back response contains tabId"

end_test

# --- T2: Navigate forward after back ---
start_test "Navigate forward after back"

# Go forward on same tab to page B
pt_post "/forward?tabId=${TAB_ID}" ''
assert_ok "POST /forward"
RESULT_URL=$(echo "$RESULT" | jq -r '.url // empty')
assert_json_contains "$RESULT" ".url" "form.html" "forward returned to form.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "forward response contains tabId"

end_test

# --- T3: Reload page stays on same URL ---
start_test "Reload page stays on same URL"

# Reload the same tab (should still be on form.html from T2)
pt_post "/reload?tabId=${TAB_ID}" ''
assert_ok "POST /reload"
RESULT_URL=$(echo "$RESULT" | jq -r '.url // empty')
assert_json_contains "$RESULT" ".url" "form.html" "reload stayed on form.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "reload response contains tabId"

end_test

# --- T4: Back with tabId parameter ---
start_test "Back with tabId parameter"

# Navigate tab to page A
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "navigate to index.html with tabId"

# Navigate to page B
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "navigate to form.html with tabId"

# Go back using tabId query parameter
pt_post "/back?tabId=${TAB_ID}" ''
assert_ok "POST /back?tabId=<id>"
assert_json_contains "$RESULT" ".url" "index.html" "back with tabId returned to index.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "back with tabId response contains correct tabId"

end_test

# --- T5: Tab-scoped back using path parameter ---
start_test "Tab-scoped back using path parameter"

# Navigate to form.html
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "navigate to form.html"

# Go back using path-scoped endpoint /tabs/{id}/back
pt_post "/tabs/${TAB_ID}/back" ''
assert_ok "POST /tabs/{id}/back"
assert_json_contains "$RESULT" ".url" "index.html" "path-scoped back returned to index.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "path-scoped back response contains correct tabId"

end_test

# --- T6: Back with no history stays on same page ---
start_test "Back with no history stays on same page"

# Create a new tab and navigate to one page only
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons.html"
NEW_TAB_ID=$(echo "$RESULT" | jq -r '.tabId // empty')
URL_SINGLE=$(echo "$RESULT" | jq -r '.url // empty')
show_tab "NEW_TAB_ID" "$NEW_TAB_ID"

# Try to go back with no history — should stay on same page
pt_post "/back?tabId=${NEW_TAB_ID}" ''
assert_ok "POST /back with no history (should stay on same page)"
RESULT_URL=$(echo "$RESULT" | jq -r '.url // empty')
assert_json_contains "$RESULT" ".url" "buttons.html" "back with no history stayed on same page"
assert_json_contains "$RESULT" ".tabId" "$NEW_TAB_ID" "back with no history response contains correct tabId"

end_test
