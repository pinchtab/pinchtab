#!/bin/bash
# 25-press-keys.sh — Verify press action sends actual key events (not text)
#
# Regression test for GitHub issue #236: press action was typing key names
# as literal text instead of dispatching keyboard events.

source "$(dirname "$0")/common.sh"

# Use permissive instance (needs evaluate enabled)
E2E_SERVER="http://pinchtab:9999"

# ─────────────────────────────────────────────────────────────────
start_test "press Enter: does not type 'Enter' as text"

# Navigate to form fixture
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

# Type into username field
pt_post /action -d '{"kind":"type","selector":"#username","text":"testuser"}'
assert_ok "type into username"

# Press Enter
pt_post /action -d '{"kind":"press","key":"Enter"}'
assert_ok "press Enter"

# The critical check: username should NOT contain "Enter" as literal text
assert_input_not_contains "#username" "Enter" "Enter key should dispatch event, not type text (bug #236)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "press Tab: does not type 'Tab' as text"

# Navigate fresh
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

# Focus on username and type
pt_post /action -d '{"kind":"click","selector":"#username"}'
pt_post /action -d '{"kind":"type","selector":"#username","text":"hello"}'
assert_ok "type hello"

# Press Tab
pt_post /action -d '{"kind":"press","key":"Tab"}'
assert_ok "press Tab"

# The critical check: username should NOT contain "Tab" as literal text
assert_input_not_contains "#username" "Tab" "Tab key should dispatch event, not type text (bug #236)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "press Escape: does not type 'Escape' as text"

# Navigate fresh
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

# Type something
pt_post /action -d '{"kind":"type","selector":"#username","text":"world"}'
assert_ok "type world"

# Press Escape
pt_post /action -d '{"kind":"press","key":"Escape"}'
assert_ok "press Escape"

# The critical check: username should NOT contain "Escape" as literal text
assert_input_not_contains "#username" "Escape" "Escape key should dispatch event, not type text (bug #236)"

end_test
