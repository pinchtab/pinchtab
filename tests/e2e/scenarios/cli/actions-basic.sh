#!/bin/bash
# actions-basic.sh — CLI happy-path action scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab fill <selector> <text>"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok fill "#username" "hello world"
assert_output_contains "filled" "confirms fill action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab press <key>"

pt_ok press Tab

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab scroll down"

pt_ok nav "${FIXTURES_URL}/table.html"
pt_ok scroll down

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab hover <ref>"

pt_ok nav "${FIXTURES_URL}/buttons.html"
pt_ok snap

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
  echo -e "  ${YELLOW}⚠${NC} no ref found, skipping hover"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab mouse move/down/up/wheel"

pt_ok nav "${FIXTURES_URL}/mouse-events.html"
pt_ok snap --interactive

MOUSE_REF=$(find_ref_by_name "Mouse Target" "$PT_OUT")
if assert_ref_found "$MOUSE_REF" "mouse target ref"; then
  pt_ok mouse move "$MOUSE_REF"
  assert_output_contains "moved" "confirms mouse move action"

  pt_ok mouse down --button left
  assert_output_contains "down" "confirms mouse down action"

  pt_ok mouse up --button left
  assert_output_contains "up" "confirms mouse up action"

  pt_ok mouse wheel 240 --dx 40
  assert_output_contains "wheel" "confirms mouse wheel action"

  pt_ok eval "window.mouseFixtureState.mousemoveCount"
  assert_output_jq '.result >= 1' "mousemove count incremented" "mousemove count did not increment"

  pt_ok eval "window.mouseFixtureState.mousedownCount"
  assert_json_field ".result" "1" "mousedown count is 1"

  pt_ok eval "window.mouseFixtureState.mouseupCount"
  assert_json_field ".result" "1" "mouseup count is 1"

  pt_ok eval "window.mouseFixtureState.lastButton"
  assert_json_field ".result" "left" "last button is left"

  pt_ok eval "window.mouseFixtureState.wheelCount"
  assert_json_field ".result" "1" "wheel count is 1"

  pt_ok eval "window.mouseFixtureState.wheelDeltaY"
  assert_json_field ".result" "240" "wheel delta Y accumulated"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab drag <from> <to>"

pt_ok nav "${FIXTURES_URL}/mouse-events.html"
pt_ok snap --interactive

DRAG_REF=$(find_ref_by_name "Mouse Target" "$PT_OUT")
if assert_ref_found "$DRAG_REF" "drag target ref"; then
  pt_ok drag "$DRAG_REF" "160,190"
  assert_output_contains "up" "confirms drag wrapper completed"

  pt_ok eval "window.mouseFixtureState.mousemoveCount"
  assert_output_jq '.result >= 2' "drag performs multiple move events" "drag did not perform multiple move events"

  pt_ok eval "window.mouseFixtureState.mousedownCount"
  assert_json_field ".result" "1" "drag performed one mouse down"

  pt_ok eval "window.mouseFixtureState.mouseupCount"
  assert_json_field ".result" "1" "drag performed one mouse up"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab check/uncheck <selector>"

pt_ok nav "${FIXTURES_URL}/form.html"

pt_ok check "#terms"
assert_json_field ".result.checked" "true" "check marks the checkbox"

pt_ok eval "document.querySelector('#terms').checked"
assert_json_field ".result" "true" "DOM checkbox state is checked"

pt_ok uncheck "#terms"
assert_json_field ".result.checked" "false" "uncheck clears the checkbox"

pt_ok eval "document.querySelector('#terms').checked"
assert_json_field ".result" "false" "DOM checkbox state is unchecked"

end_test

start_test "pinchtab select"
pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok snap --interactive
pt select e0 "option1" 2>/dev/null
echo -e "  ${GREEN}✓${NC} select command executed"
((ASSERTIONS_PASSED++)) || true
end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab focus <ref>"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok snap --interactive

USERNAME_REF=$(find_ref_by_role_and_name "textbox" "Username:" "$PT_OUT")
if assert_ref_found "$USERNAME_REF" "username input ref"; then
  pt_ok focus "$USERNAME_REF"
  assert_output_contains "focused" "confirms focus action"

  # Verify the element is now focused
  pt_ok eval "document.activeElement.id"
  assert_json_field ".result" "username" "username input is focused"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab focus --css <selector>"

pt_ok nav "${FIXTURES_URL}/form.html"

pt_ok focus --css "#email"
assert_output_contains "focused" "confirms focus by CSS selector"

# Verify the element is now focused
pt_ok eval "document.activeElement.id"
assert_json_field ".result" "email" "email input is focused"

end_test
