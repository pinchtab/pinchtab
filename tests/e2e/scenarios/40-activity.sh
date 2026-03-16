#!/bin/bash
# 40-activity.sh — Activity API E2E tests

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "GET /api/activity — records navigation and actions"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

pt_post /action -d '{"kind":"click","selector":"#increment"}'
assert_http_status "200" "click action succeeded"

pt_get "/api/activity?tabId=${TAB_ID}&limit=20&ageSec=300"
assert_ok "activity query"
assert_json_exists "$RESULT" '.events' "events array present"

if echo "$RESULT" | jq -e --arg tab "$TAB_ID" '.events[] | select(.tabId == $tab and .path == "/navigate")' > /dev/null; then
  echo -e "  ${GREEN}✓${NC} navigate event recorded for tab"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} missing navigate event for tab ${TAB_ID}"
  ((ASSERTIONS_FAILED++)) || true
fi

if echo "$RESULT" | jq -e --arg tab "$TAB_ID" '.events[] | select(.tabId == $tab and .action == "click")' > /dev/null; then
  echo -e "  ${GREEN}✓${NC} click action recorded for tab"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} missing click action event for tab ${TAB_ID}"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test
