#!/bin/bash
# 05-actions.sh — Browser actions (click, type, press)

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click <button>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
click_button "Increment"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab type <field> <text>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

pt_get /snapshot
type_into "Username" "testuser123"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab press <key>"

press_key "Escape"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click (CSS selector)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

# Click using CSS selector instead of ref
pt_post /action -d '{"kind":"click","selector":"#increment"}'
assert_ok "click by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab type (CSS selector)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

# Type using CSS selector
pt_post /action -d '{"kind":"type","selector":"#username","text":"selectortest"}'
assert_ok "type by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snapshot (CSS selector filter)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

# Get snapshot scoped to a specific element
pt_get "/snapshot?selector=#username"
assert_ok "snapshot with selector"

end_test
