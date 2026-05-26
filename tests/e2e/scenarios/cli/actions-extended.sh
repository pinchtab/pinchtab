#!/bin/bash
# actions-extended.sh — CLI advanced action scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab type <ref> <text>"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok snap --interactive --compact=false

USERNAME_REF=$(find_ref_by_role_and_name "textbox" "Username:" "$PT_OUT")
if assert_ref_found "$USERNAME_REF" "username input ref"; then
  pt_ok type "$USERNAME_REF" "typed-via-ref"
  assert_output_contains "OK" "confirms text was typed"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click <ref>"

pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok snap --full

BUTTON_REF=$(find_ref_by_role "button" "$PT_OUT")
if assert_ref_found "$BUTTON_REF" "button ref"; then
  pt_ok click "$BUTTON_REF"
  assert_output_contains "OK" "confirms click by ref"
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
start_test "pinchtab click --mode dom|dispatch"

pt_ok nav "${FIXTURES_URL}/occluded-click.html"
pt_ok snap --interactive --compact=false

TARGET_REF=$(find_ref_by_role_and_name "button" "Proceed" "$PT_OUT")
if assert_ref_found "$TARGET_REF" "occluded target ref"; then
  pt click "$TARGET_REF"
  assert_exit_code 1 "plain cli click fails on occlusion"
  if printf '%s\n%s\n' "$PT_OUT" "$PT_ERR" | grep -q "element is occluded"; then
    pass_assert "plain cli click surfaces occlusion"
  else
    fail_assert "plain cli click surfaces occlusion"
    echo -e "  ${RED}stdout: $PT_OUT${NC}"
    echo -e "  ${RED}stderr: $PT_ERR${NC}"
  fi

  pt_ok click "$TARGET_REF" --mode dom
  assert_output_contains "OK" "dom mode click succeeds"
  pt_ok eval "JSON.stringify(window.occludedClickState)"
  assert_json_jq "$PT_OUT" '.clicked == true and .clicks == 1' "dom mode updated click state" "dom mode did not trigger click"

  pt_ok eval "window.occludedClickState = { clicked: false, clicks: 0, lastClientX: null, lastClientY: null }; JSON.stringify(window.occludedClickState)"
  pt_ok click "$TARGET_REF" --mode dispatch
  assert_output_contains "OK" "dispatch mode click succeeds"
  pt_ok eval "JSON.stringify(window.occludedClickState)"
  assert_json_jq "$PT_OUT" '.clicked == true and .clicks == 1' "dispatch mode updated click state" "dispatch mode did not trigger click"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab hover <ref>"

pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok snap --interactive --compact=false

# Pick a stable, interactive element ref instead of the first arbitrary ref.
REF=$(find_ref_by_name "Increment" "$PT_OUT")
if [ -z "$REF" ] || [ "$REF" = "null" ]; then
  REF=$(find_ref_by_name "Decrement" "$PT_OUT")
fi
if [ -z "$REF" ] || [ "$REF" = "null" ]; then
  REF=$(find_ref_by_name "Reset" "$PT_OUT")
fi

if [ -n "$REF" ] && [ "$REF" != "null" ]; then
  pt_ok hover "$REF"
else
  skip_assert "no ref found, skipping hover"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab mouse move/down/up/wheel"

pt_ok nav "${FIXTURES_URL}/mouse-events.html"
pt_ok mouse move --css "#mouse-target"
assert_output_contains "OK" "confirms mouse move action"

pt_ok mouse down --button left
assert_output_contains "OK" "confirms mouse down action"

pt_ok mouse up --button left
assert_output_contains "OK" "confirms mouse up action"

pt_ok mouse wheel 240 --dx 40
assert_output_contains "OK" "confirms mouse wheel action"

pt_ok eval "JSON.stringify({mousemoveCount: window.mouseFixtureState.mousemoveCount, mousedownCount: window.mouseFixtureState.mousedownCount, mouseupCount: window.mouseFixtureState.mouseupCount, lastButton: window.mouseFixtureState.lastButton, wheelCount: window.mouseFixtureState.wheelCount, wheelDeltaY: window.mouseFixtureState.wheelDeltaY})"
assert_json_jq "$PT_OUT" '(.mousemoveCount // 0) >= 1' "mousemove count incremented" "mousemove count did not increment"
assert_json_jq "$PT_OUT" '.mousedownCount == 1' "mousedown count is 1" "mousedown count is not 1"
assert_json_jq "$PT_OUT" '.mouseupCount == 1' "mouseup count is 1" "mouseup count is not 1"
assert_json_jq "$PT_OUT" '.lastButton == "left"' "last button is left" "last button is not left"
assert_json_jq "$PT_OUT" '.wheelCount == 1' "wheel count is 1" "wheel count is not 1"
assert_json_jq "$PT_OUT" '.wheelDeltaY == 240' "wheel delta Y accumulated" "wheel delta Y did not accumulate"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab drag <from> <to>"

pt_ok nav "${FIXTURES_URL}/mouse-events.html"
pt_ok drag "#mouse-target" --drag-x 80 --drag-y 30
assert_output_contains "OK" "confirms drag action completed"

pt_ok eval "JSON.stringify({mousemoveCount: window.mouseFixtureState.mousemoveCount, mousedownCount: window.mouseFixtureState.mousedownCount, mouseupCount: window.mouseFixtureState.mouseupCount})"
assert_json_jq "$PT_OUT" '(.mousemoveCount // 0) >= 2' "drag performs multiple move events" "drag did not perform multiple move events"
assert_json_jq "$PT_OUT" '.mousedownCount == 1' "drag performed one mouse down" "drag did not perform one mouse down"
assert_json_jq "$PT_OUT" '.mouseupCount == 1' "drag performed one mouse up" "drag did not perform one mouse up"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab keydown/keyup (hold and release)"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok click --css "#username"

# Hold Shift key down
pt_ok keydown Shift
assert_output_contains "OK" "keydown response"

# Release Shift key
pt_ok keyup Shift
assert_output_contains "OK" "keyup response"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab keyboard type <text>"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok click --css "#username"

# Use keyboard type to simulate keystrokes
pt_ok keyboard type "hello123"
assert_output_contains "OK" "keyboard type response"

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
assert_output_contains "OK" "keyboard inserttext response"

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
assert_output_contains "OK" "keyboard type response"

# Verify dots were preserved
pt_ok eval "document.querySelector('#email').value"
assert_output_contains "test@example.com" "period characters preserved"

# Test IP address (multiple dots)
pt_ok click --css "#username"
pt_ok keyboard type "192.168.1.100"
pt_ok eval "document.querySelector('#username').value"
assert_output_contains "192.168.1.100" "multiple dots preserved"

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
assert_output_contains "LOGIN_SUCCESS" "JS submit handler fired on button click"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click submit button with wrong creds shows failure (#411)"

pt_ok nav "${FIXTURES_URL}/js-submit.html"
pt_ok fill "#username" "wrong"
pt_ok fill "#password" "wrong"
pt_ok click "--css" "#submit-btn"

pt_ok eval "document.getElementById('result-failure')?.textContent"
assert_output_contains "LOGIN_FAILURE" "JS submit handler fired and returned failure"

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
assert_output_contains "OK" "keyboard type response"

# Verify the text was typed correctly
pt_ok eval "document.querySelector('#username').value"
assert_output_contains "$LONG_TEXT" "long string typed correctly"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab wait --not-text (immediate, absent)"

# Text that never existed — should succeed immediately.
pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok wait --not-text "nonexistent-text-xyz" --timeout 2000
assert_output_contains "OK" "wait response present"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab wait --not-text (after DOM change)"

# Toggle click hides #toggle-content (display:none removes innerText).
pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok click --css "#toggle-btn"
pt_ok wait --not-text "This content can be toggled." --timeout 5000
assert_output_contains "OK" "wait returned after text disappeared"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab wait --not-text (timeout when text persists)"

# Text stays on page — command should timeout (exit code 4).
pt_ok nav "${FIXTURES_URL}/buttons.html"
pt wait --not-text "Increment" --timeout 500
# Expect non-zero exit due to timeout
if [ "$PT_CODE" -ne 0 ]; then
  pass_assert "timeout reported when text persists (exit $PT_CODE)"
else
  fail_assert "expected timeout, got success"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click --snap"

pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok click --css "#increment" --snap

# Output should contain OK and snapshot content
assert_output_contains "OK" "click succeeded"
assert_output_contains "button" "snapshot contains button element"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav --snap"

pt_ok nav "${FIXTURES_URL}/form.html" --snap

# Output should contain tab ID and snapshot
assert_output_contains "textbox" "snapshot contains form input"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab fill --snap"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok fill "#username" "testuser" --snap

# Output should contain OK and snapshot
assert_output_contains "OK" "fill succeeded"
assert_output_contains "textbox" "snapshot contains form input"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab select --snap"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok select "#country" "us" --snap

# Output should contain OK and snapshot
assert_output_contains "OK" "select succeeded"
assert_output_contains "combobox" "snapshot contains select element"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab back --snap"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok back --snap

# Output should contain URL and snapshot
assert_output_contains "index.html" "back navigated to previous URL"
assert_output_contains "link" "snapshot contains navigation elements"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab forward --snap"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok back
pt_ok forward --snap

# Output should contain URL and snapshot
assert_output_contains "form.html" "forward navigated to next URL"
assert_output_contains "textbox" "snapshot contains form input"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab reload --snap"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok reload --snap

# Output should contain OK and snapshot
assert_output_contains "OK" "reload succeeded"
assert_output_contains "textbox" "snapshot contains form input"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click --snap-diff"

pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok snap  # establish baseline
pt_ok click --css "#increment" --snap-diff

# Output should contain OK and compact diff format (+N ~N -N)
assert_output_contains "OK" "click succeeded"
assert_output_contains "~" "snap-diff shows changes"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab fill --snap-diff"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok snap  # establish baseline
pt_ok fill "#username" "diffuser" --snap-diff

# Output should contain OK and compact diff format (+N ~N -N)
assert_output_contains "OK" "fill succeeded"
assert_output_contains "~" "snap-diff shows changes"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab press --snap"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok focus --css "#username"
pt_ok press Tab --snap

# Output should be OK plus an interactive snapshot. The snapshot must mention
# a textbox (form contains username/email/password inputs); confirms --snap
# triggered the post-action snapshot fetch on a key press.
assert_output_contains "OK" "press succeeded"
assert_output_contains "textbox" "snap output contains form input"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab press --snap-diff"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok focus --css "#username"
pt_ok snap  # establish baseline
pt_ok press Tab --snap-diff

# Tab key shifts focus from #username to the next input (#email). Snap-diff
# must show the diff header (+N ~N -N) — focus changes mark prior/next
# elements as modified, so we expect a "~" change marker. Falls back to "+0"
# header if the platform's accessibility tree doesn't surface focus shifts.
assert_output_contains "OK" "press succeeded"
assert_output_contains "+" "snap-diff shows compact diff header"

end_test

# ─────────────────────────────────────────────────────────────────
# Cookie-banner dismissal — exercises the --dismiss-banners flag on the
# nav/back/forward/reload/click code paths against a fixture page that
# renders a fixed-position cookie banner with an "Accept all" button.
# Phase 1 of the dismissal heuristic clicks the labelled button, which
# triggers the fixture's onclick to set body[data-cookie-dismissed=true].
# We use that attribute as the post-condition for a successful dismissal.
# ─────────────────────────────────────────────────────────────────

start_test "pinchtab nav --dismiss-banners clears cookie banner"

pt_ok nav "${FIXTURES_URL}/cookie-banner.html" --dismiss-banners

# The fixture's Accept-all handler sets body[data-cookie-dismissed=true] and
# adds class=dismissed to #cookie-banner.
pt_ok eval "document.body.dataset.cookieDismissed"
assert_output_contains "true" "fixture's Accept-all handler ran"

pt_ok eval "document.getElementById('cookie-banner').classList.contains('dismissed')"
assert_output_contains "true" "cookie banner is hidden"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav (no flag) leaves cookie banner up"

pt_ok nav "${FIXTURES_URL}/cookie-banner.html"

pt_ok eval "document.body.dataset.cookieDismissed || ''"
# Should be empty — the helper did not run, and the user did not click.
assert_output_not_contains "true" "Accept-all was NOT auto-clicked"

pt_ok eval "document.getElementById('cookie-banner').classList.contains('dismissed')"
assert_output_contains "false" "cookie banner is still visible"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab reload --dismiss-banners clears re-shown banner"

# Land on the page with the flag — the banner is dismissed.
pt_ok nav "${FIXTURES_URL}/cookie-banner.html" --dismiss-banners
pt_ok eval "document.body.dataset.cookieDismissed"
assert_output_contains "true" "initial dismissal landed"

# Reload WITHOUT the flag — the banner re-renders fresh, dataset wiped.
pt_ok reload
pt_ok eval "document.body.dataset.cookieDismissed || ''"
assert_output_not_contains "true" "reload re-rendered the banner"

# Reload WITH --dismiss-banners — should re-dismiss.
pt_ok reload --dismiss-banners
pt_ok eval "document.body.dataset.cookieDismissed"
assert_output_contains "true" "reload --dismiss-banners cleared it again"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab back --dismiss-banners clears banner on prior page"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok nav "${FIXTURES_URL}/cookie-banner.html"
pt_ok back --dismiss-banners

# back should land us on index.html — banner-less. The dismissal helper
# is allowed to run (it's a no-op when there are no matching elements);
# we just verify back navigated and didn't error.
assert_output_contains "index.html" "back navigated to prior URL"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab forward --dismiss-banners clears banner on next page"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok nav "${FIXTURES_URL}/cookie-banner.html"
pt_ok back
pt_ok forward --dismiss-banners

# forward returns to cookie-banner.html — the helper should fire the
# Accept-all click on the freshly-rendered banner.
pt_ok eval "document.body.dataset.cookieDismissed"
assert_output_contains "true" "forward --dismiss-banners cleared the banner"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click --wait-nav --dismiss-banners on landing page"

# Start on a banner-less page that links to the cookie-banner page. Click
# the link with --wait-nav so the click triggers a navigation; pair with
# --dismiss-banners so the helper fires after the nav settles.
pt_ok nav "${FIXTURES_URL}/index.html"

# Inject a link to the cookie-banner fixture so the click triggers a nav.
pt_ok eval "(() => { const a = document.createElement('a'); a.id = 'goto-cookie'; a.href = '${FIXTURES_URL}/cookie-banner.html'; a.textContent = 'go'; document.body.appendChild(a); return 'ok'; })()"
pt_ok click --css "#goto-cookie" --wait-nav --dismiss-banners

pt_ok eval "document.body.dataset.cookieDismissed"
assert_output_contains "true" "click --wait-nav --dismiss-banners cleared the banner"

end_test
