#!/bin/bash
# actions-extended.sh — CLI advanced action scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab type <ref> <text>"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok snap --interactive

USERNAME_REF=$(find_ref_by_role_and_name "textbox" "Username:" "$PT_OUT")
if assert_ref_found "$USERNAME_REF" "username input ref"; then
  pt_ok type "$USERNAME_REF" "typed-via-ref"
  assert_output_contains "typed" "confirms text was typed"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click <ref>"

pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok snap

BUTTON_REF=$(find_ref_by_role "button" "$PT_OUT")
if assert_ref_found "$BUTTON_REF" "button ref"; then
  pt_ok click "$BUTTON_REF"
  assert_output_contains "clicked" "confirms click by ref"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click --wait-nav"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok snap --interactive
pt click e0 --wait-nav

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click --css"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok click --css "button[type=submit]"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab hover (basic)"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok snap --interactive

HOVER_REF=$(find_ref_by_role_and_name "textbox" "Username:" "$PT_OUT")
if assert_ref_found "$HOVER_REF" "username input ref"; then
  pt_ok hover "$HOVER_REF"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab keydown/keyup (hold and release)"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok click --css "#username"

# Hold Shift key down
pt_ok keydown Shift
assert_output_contains "keydown" "keydown response"
assert_output_contains "Shift" "keydown key name"

# Release Shift key
pt_ok keyup Shift
assert_output_contains "keyup" "keyup response"
assert_output_contains "Shift" "keyup key name"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab keyboard type <text>"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok click --css "#username"

# Use keyboard type to simulate keystrokes
pt_ok keyboard type "hello123"
assert_output_contains "typed" "keyboard type response"

# Verify the text was actually typed into the input
pt_ok eval "document.querySelector('#username').value"
assert_output_contains "hello123" "keyboard type value persisted"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab keyboard inserttext <text>"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok click --css "#email"

# Use keyboard inserttext (paste-like, no key events)
pt_ok keyboard inserttext "test@example.com"
assert_output_contains "inserted" "keyboard inserttext response"

# Verify the text was actually inserted
pt_ok eval "document.querySelector('#email').value"
assert_output_contains "test@example.com" "keyboard inserttext value persisted"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab keyboard type vs inserttext difference"

# This test verifies that keyboard type triggers key events
# while inserttext does not (paste-like behavior)

pt_ok nav "${FIXTURES_URL}/form.html"

# Type into username using keyboard type (triggers keydown/keypress/keyup)
pt_ok click --css "#username"
pt_ok keyboard type "ABC"

# Insert into email using keyboard inserttext (no key events)
pt_ok click --css "#email"
pt_ok keyboard inserttext "XYZ"

# Both should have values
pt_ok eval "document.querySelector('#username').value"
assert_output_contains "ABC" "keyboard type value present"

pt_ok eval "document.querySelector('#email').value"
assert_output_contains "XYZ" "keyboard inserttext value present"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab keyboard type preserves special characters (#412)"

# Issue #412: keyboard type was swallowing dot/period because ASCII 46
# mapped to Delete key virtualKeyCode instead of Period key.

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok click --css "#email"

# Type text containing periods (email address)
pt_ok keyboard type "test@example.com"
assert_output_contains "typed" "keyboard type response"

# Verify dots were preserved
pt_ok eval "document.querySelector('#email').value"
assert_json_field ".result" "test@example.com" "period characters preserved"

# Test IP address (multiple dots)
pt_ok click --css "#username"
pt_ok keyboard type "192.168.1.100"
pt_ok eval "document.querySelector('#username').value"
assert_json_field ".result" "192.168.1.100" "multiple dots preserved"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click submit button fires JS submit handler (#411)"

# Issue #411: clicking a submit button must dispatch the full event chain
# (mousedown, mouseup, click, submit) so JS-only form handlers run.
# Regression: bare CDP click skipped the 'submit' event, causing a timeout
# on frameworks like Odoo that rely on addEventListener('submit').

pt_ok nav "${FIXTURES_URL}/js-submit.html"
pt_ok fill "#username" "admin"
pt_ok fill "#password" "secret"
pt_ok click "--css" "#submit-btn"

# The JS submit handler must have fired and written LOGIN_SUCCESS to #result.
pt_ok eval "document.getElementById('result-success')?.textContent"
assert_json_field ".result" "LOGIN_SUCCESS" "JS submit handler fired on button click"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click submit button with wrong creds shows failure (#411)"

pt_ok nav "${FIXTURES_URL}/js-submit.html"
pt_ok fill "#username" "wrong"
pt_ok fill "#password" "wrong"
pt_ok click "--css" "#submit-btn"

pt_ok eval "document.getElementById('result-failure')?.textContent"
assert_json_field ".result" "LOGIN_FAILURE" "JS submit handler fired and returned failure"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab keyboard type handles long strings (#413)"

# Issue #413: keyboard type with 50+ chars caused timeout and daemon freeze
# due to 2 CDP calls per character.

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok click --css "#username"

# Type a long string (65 chars) - should not timeout
LONG_TEXT="The quick brown fox jumps over the lazy dog and keeps on running"
pt_ok keyboard type "$LONG_TEXT"
assert_output_contains "typed" "keyboard type response"

# Verify the text was typed correctly
pt_ok eval "document.querySelector('#username').value"
assert_json_field ".result" "$LONG_TEXT" "long string typed correctly"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab wait --not-text (immediate, absent)"

# Text that never existed — should succeed immediately.
pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok wait --not-text "nonexistent-text-xyz" --timeout 2000
assert_output_contains "waited" "wait response present"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab wait --not-text (after DOM change)"

# Toggle click hides #toggle-content (display:none removes innerText).
pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok click --css "#toggle-btn"
pt_ok wait --not-text "This content can be toggled." --timeout 5000
assert_output_contains "waited" "wait returned after text disappeared"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab wait --not-text (timeout when text persists)"

# Text stays on page — command should return (200) with waited=false.
pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok wait --not-text "Increment" --timeout 500
assert_output_contains "timeout" "timeout reported when text persists"

end_test
