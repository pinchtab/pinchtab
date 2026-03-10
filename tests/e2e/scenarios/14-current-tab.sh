#!/bin/bash
# 14-current-tab.sh — Current tab shortcuts (POST /tab, POST /action without tab ID)

source "$(dirname "$0")/common.sh"

start_test "current tab: POST /tab creates and returns current tab"

# POST /tab creates a new tab (shorthand for /tabs with GET)
pt_post /tab -d '{"url":"about:blank"}'
assert_ok "POST /tab"
assert_json_exists "$RESULT" ".tabId" "returns tab ID"

TAB_ID=$(get_tab_id)
show_tab "created" "$TAB_ID"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "current tab: POST /action without tab ID uses current tab"

# Navigate to buttons page
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

# POST /action without tab ID — should act on current tab
pt_post /action -d '{"action":"click","selector":"button"}'
assert_ok "action on current tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "current tab: POST /snapshot without tab ID"

# Navigate to a known page
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"

# POST /snapshot (not /tabs/{id}/snapshot) uses current tab
pt_post /snapshot -d '{}'
assert_ok "snapshot current tab"
assert_json_contains "$RESULT" '.title' 'E2E Test' "snapshot has title"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "current tab: GET /text without tab ID"

# GET /text should read from current tab (the index page navigated above)
pt_get /text
assert_ok "text from current tab"
assert_contains "$RESULT" "Buttons\|Search\|E2E" "text content present"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "current tab: POST /actions batch on current tab"

# Navigate to form page
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"

# Batch actions on current tab without explicit tab ID
pt_post /actions -d '[
  {"action":"fill","selector":"input[name=\"name\"]","text":"Batch Test"},
  {"action":"fill","selector":"input[name=\"email\"]","text":"batch@test.com"}
]'
assert_ok "batch actions on current tab"

end_test
