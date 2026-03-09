#!/bin/bash
# 07-find.sh — Element finding with semantic search

source "$(dirname "$0")/common.sh"

# Navigate to find test page
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/find.html\"}"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (login button)"

pt_post /find -d '{"query":"login button"}'
assert_ok "find login"
assert_result_exists ".best_ref" "has best_ref"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (email input)"

pt_post /find -d '{"query":"email input field"}'
assert_ok "find email"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (delete button)"

pt_post /find -d '{"query":"delete account button","threshold":0.2}'
assert_ok "find delete"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (search)"

pt_post /find -d '{"query":"search input","topK":5}'
assert_ok "find search"
assert_json_length_gte "$RESULT" ".matches" 1 "has matches"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find --tab <id>"

# Open find page in new tab - capture tabId from response
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/find.html\",\"newTab\":true}"
assert_ok "navigate for find"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
sleep 1

pt_post "/tabs/${TAB_ID}/find" -d '{"query":"sign up link"}'
assert_ok "tab find"

end_test
