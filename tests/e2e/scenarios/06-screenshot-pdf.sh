#!/bin/bash
# 06-screenshot-pdf.sh — Screenshot and PDF export

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
sleep 1

pt_get /screenshot
assert_ok "screenshot"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf (default tab)"

pt_get /pdf
assert_ok "pdf default"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf (landscape)"

pt_post /pdf -d '{"landscape":true}'
assert_ok "pdf landscape"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot --tab <id>"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_get "/tabs/${TAB_ID}/screenshot"
assert_ok "tab screenshot"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf --tab <id>"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_get "/tabs/${TAB_ID}/pdf"
assert_ok "tab pdf"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf --tab <id> (with options)"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_post "/tabs/${TAB_ID}/pdf" -d '{"printBackground":true,"scale":0.8}'
assert_ok "tab pdf with options"

end_test
