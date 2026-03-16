#!/bin/bash
# 40-activity.sh — Activity API E2E tests

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "GET /api/activity — records tab-scoped requests"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/tabs/${TAB_ID}/snapshot"
assert_ok "tab snapshot succeeded"

pt_post "/tabs/${TAB_ID}/action" -d '{"kind":"click","selector":"#increment"}'
assert_http_status "200" "click action succeeded"

pt_get "/api/activity?tabId=${TAB_ID}&limit=20&ageSec=300"
assert_ok "activity query"
assert_json_exists "$RESULT" '.events' "events array present"

if echo "$RESULT" | jq -e --arg tab "$TAB_ID" --arg path "/tabs/${TAB_ID}/snapshot" '.events[] | select(.tabId == $tab and .path == $path)' > /dev/null; then
  echo -e "  ${GREEN}✓${NC} tab snapshot event recorded"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} missing tab snapshot event for tab ${TAB_ID}"
  ((ASSERTIONS_FAILED++)) || true
fi

if echo "$RESULT" | jq -e --arg tab "$TAB_ID" --arg path "/tabs/${TAB_ID}/action" '.events[] | select(.tabId == $tab and .path == $path)' > /dev/null; then
  echo -e "  ${GREEN}✓${NC} tab action event recorded"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} missing tab action event for tab ${TAB_ID}"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test
