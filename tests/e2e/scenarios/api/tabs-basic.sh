#!/bin/bash
# tabs-basic.sh — API happy-path tab scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap --tab <id> (regression #207)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)
show_tab "created" "$TAB_ID"

pt_get "/tabs/${TAB_ID}/snapshot"
assert_ok "tab snapshot"
assert_json_contains "$RESULT" '.title' 'E2E Test'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text/screenshot --tab <id>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/tabs/${TAB_ID}/text"
assert_ok "tab text"

pt_get "/tabs/${TAB_ID}/screenshot"
assert_ok "tab screenshot"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab close"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)
AFTER_CREATE=$(get_tab_count)

pt_post "/tabs/${TAB_ID}/close" -d '{}'
assert_ok "tab close"

sleep 1
assert_tab_closed "$AFTER_CREATE"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Focus tab by ID"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate to index.html"
TAB_ID_A=$(echo "$RESULT" | jq -r '.tabId // empty')
assert_tab_id "tab A"
show_tab "TAB_ID_A" "$TAB_ID_A"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate to form.html"
TAB_ID_B=$(echo "$RESULT" | jq -r '.tabId // empty')
assert_tab_id "tab B"
show_tab "TAB_ID_B" "$TAB_ID_B"

pt_post /tab "{\"action\":\"focus\",\"tabId\":\"${TAB_ID_A}\"}"
assert_ok "focus on tab A"
assert_json_contains "$RESULT" ".focused" "true" "response contains focused=true"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Focus tab switches active tab"

pt_get "/snapshot?tabId=${TAB_ID_A}"
assert_ok "snapshot of tab A"
assert_result_exists ".title" "snapshot has title"

pt_get "/text?tabId=${TAB_ID_A}&format=text"
assert_ok "text of tab A"
assert_contains "$RESULT" "E2E" "tab A contains E2E Test Suite content"

pt_post /tab "{\"action\":\"focus\",\"tabId\":\"${TAB_ID_B}\"}"
assert_ok "focus on tab B"

pt_get "/text?tabId=${TAB_ID_B}&format=text"
assert_ok "text of tab B"
assert_contains "$RESULT" "Form" "tab B contains Form content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Focus missing tabId returns 400"

pt_post /tab "{\"action\":\"focus\"}"
assert_http_status 400 "missing tabId returns 400"
assert_json_contains "$RESULT" ".error" "tabId" "error message mentions tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Focus nonexistent tab returns 404"

pt_post /tab "{\"action\":\"focus\",\"tabId\":\"nonexistent-tab-id-12345\"}"
assert_http_status 404 "nonexistent tabId returns 404"
assert_json_contains "$RESULT" ".error" "not found" "error message indicates tab not found"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Focus invalid action returns 400"

pt_post /tab "{\"action\":\"invalid\",\"tabId\":\"${TAB_ID_A}\"}"
assert_http_status 400 "invalid action returns 400"
assert_json_contains "$RESULT" ".error" "action" "error message mentions action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Navigate back to previous page"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate to index.html"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId // empty')
assert_tab_id "get tab ID"
show_tab "TAB_ID" "$TAB_ID"
URL_A=$(echo "$RESULT" | jq -r '.url // empty')
echo -e "  ${MUTED}URL_A: $URL_A${NC}"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "navigate to form.html"
URL_B=$(echo "$RESULT" | jq -r '.url // empty')
echo -e "  ${MUTED}URL_B: $URL_B${NC}"

pt_post "/back?tabId=${TAB_ID}" ''
assert_ok "POST /back"
RESULT_URL=$(echo "$RESULT" | jq -r '.url // empty')
assert_json_contains "$RESULT" ".url" "index.html" "back returned to index.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "back response contains tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Navigate forward after back"

pt_post "/forward?tabId=${TAB_ID}" ''
assert_ok "POST /forward"
RESULT_URL=$(echo "$RESULT" | jq -r '.url // empty')
assert_json_contains "$RESULT" ".url" "form.html" "forward returned to form.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "forward response contains tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Reload page stays on same URL"

pt_post "/reload?tabId=${TAB_ID}" ''
assert_ok "POST /reload"
RESULT_URL=$(echo "$RESULT" | jq -r '.url // empty')
assert_json_contains "$RESULT" ".url" "form.html" "reload stayed on form.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "reload response contains tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Back with tabId parameter"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "navigate to index.html with tabId"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "navigate to form.html with tabId"

pt_post "/back?tabId=${TAB_ID}" ''
assert_ok "POST /back?tabId=<id>"
assert_json_contains "$RESULT" ".url" "index.html" "back with tabId returned to index.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "back with tabId response contains correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Tab-scoped back using path parameter"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "navigate to form.html"

pt_post "/tabs/${TAB_ID}/back" ''
assert_ok "POST /tabs/{id}/back"
assert_json_contains "$RESULT" ".url" "index.html" "path-scoped back returned to index.html"
assert_json_contains "$RESULT" ".tabId" "$TAB_ID" "path-scoped back response contains correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Back with no history stays on same page"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons.html"
NEW_TAB_ID=$(echo "$RESULT" | jq -r '.tabId // empty')
URL_SINGLE=$(echo "$RESULT" | jq -r '.url // empty')
show_tab "NEW_TAB_ID" "$NEW_TAB_ID"

pt_post "/back?tabId=${NEW_TAB_ID}" ''
assert_ok "POST /back with no history (should stay on same page)"
RESULT_URL=$(echo "$RESULT" | jq -r '.url // empty')
assert_json_contains "$RESULT" ".url" "buttons.html" "back with no history stayed on same page"
assert_json_contains "$RESULT" ".tabId" "$NEW_TAB_ID" "back with no history response contains correct tabId"

end_test
