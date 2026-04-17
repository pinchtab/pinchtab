#!/bin/bash
# actions-basic.sh — API happy-path action scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

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

pt_post /action -d '{"kind":"click","selector":"#increment"}'
assert_ok "click by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab type (CSS selector)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

pt_post /action -d '{"kind":"type","selector":"#username","text":"selectortest"}'
assert_ok "type by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snapshot (CSS selector filter)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

pt_get "/snapshot?selector=#username"
assert_ok "snapshot with selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab scroll (down)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/table.html\"}"
assert_ok "navigate"

pt_post /action '{"kind":"scroll","direction":"down"}'
assert_ok "scroll down"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab hover (ref)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"

pt_get /snapshot
assert_ok "snapshot"
REF=$(find_ref_by_role "button")
assert_ref_found "$REF" "button ref"

pt_post /action "{\"kind\":\"hover\",\"ref\":\"${REF}\"}"
assert_ok "hover on button"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab low-level mouse actions"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/mouse-events.html\"}"
assert_ok "navigate"

pt_get "/snapshot?filter=interactive"
assert_ok "snapshot"
REF=$(find_ref_by_name "Mouse Target")
assert_ref_found "$REF" "mouse target ref"

pt_post /action "{\"kind\":\"mouse-move\",\"ref\":\"${REF}\"}"
assert_ok "mouse-move on target"

pt_post /action '{"kind":"mouse-move","x":160,"y":190}'
assert_ok "mouse-move by coordinates without hasXY"

pt_post /action '{"kind":"mouse-down","button":"left"}'
assert_ok "mouse-down at current pointer"

pt_post /action '{"kind":"mouse-up","button":"left"}'
assert_ok "mouse-up at current pointer"

pt_post /action '{"kind":"mouse-wheel","deltaY":240}'
assert_ok "mouse-wheel at current pointer"

pt_post /evaluate '{"expression":"window.mouseFixtureState.mousemoveCount"}'
assert_ok "evaluate mousemove count"
assert_result_jq '.result >= 2' "mousemove count incremented twice" "mousemove count did not increment twice"

pt_post /evaluate '{"expression":"window.mouseFixtureState.mousedownCount"}'
assert_ok "evaluate mousedown count"
assert_json_eq "$RESULT" '.result' '1' "mousedown count is 1"

pt_post /evaluate '{"expression":"window.mouseFixtureState.mouseupCount"}'
assert_ok "evaluate mouseup count"
assert_json_eq "$RESULT" '.result' '1' "mouseup count is 1"

pt_post /evaluate '{"expression":"window.mouseFixtureState.lastButton"}'
assert_ok "evaluate last button"
assert_json_eq "$RESULT" '.result' 'left' "last button is left"

pt_post /evaluate '{"expression":"window.mouseFixtureState.wheelCount"}'
assert_ok "evaluate wheel count"
assert_json_eq "$RESULT" '.result' '1' "wheel count is 1"

pt_post /evaluate '{"expression":"window.mouseFixtureState.wheelDeltaY"}'
assert_ok "evaluate wheel delta"
assert_json_eq "$RESULT" '.result' '240' "wheel delta Y accumulated"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab mouse current-pointer sequence"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/mouse-events.html\"}"
assert_ok "navigate"

pt_post /action '{"kind":"mouse-move","x":160,"y":190}'
assert_ok "prime pointer position"

pt_post /action '{"kind":"mouse-down","button":"left"}'
assert_ok "mouse-down without fresh target"

pt_post /action '{"kind":"mouse-move","x":165,"y":195}'
assert_ok "mouse-move while held"

pt_post /action '{"kind":"mouse-up","button":"left"}'
assert_ok "mouse-up without fresh target"

pt_post /evaluate '{"expression":"window.mouseFixtureState.sequence.join(\",\")"}'
assert_ok "evaluate pointer sequence"
assert_json_contains "$RESULT" '.result' 'mousedown' "sequence includes mousedown"
assert_json_contains "$RESULT" '.result' 'mouseup' "sequence includes mouseup"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab focus (ref)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate"

pt_get /snapshot
REF=$(find_ref_by_role "textbox")
assert_ref_found "$REF" "textbox ref"

pt_post /action "{\"kind\":\"focus\",\"ref\":\"${REF}\"}"
assert_ok "focus on input"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab select (combobox)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate"

pt_get /snapshot
REF=$(find_ref_by_role "combobox")
assert_ref_found "$REF" "combobox ref"

pt_post /action "{\"kind\":\"select\",\"ref\":\"${REF}\",\"value\":\"uk\"}"
assert_ok "select option"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab fill (sets value + verifiable)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate"

pt_get /snapshot
REF=$(find_ref_by_role "textbox")
assert_ref_found "$REF" "textbox ref"

pt_post /action "{\"kind\":\"fill\",\"ref\":\"${REF}\",\"text\":\"e2e_fill_test\"}"
assert_ok "fill input"

pt_post /evaluate '{"expression":"document.querySelector(\"#username\").value"}'
assert_ok "evaluate"
assert_json_contains "$RESULT" '.result' 'e2e_fill_test' "fill value persisted"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click triggers navigation"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

pt_get "/snapshot?filter=interactive"
REF=$(find_ref_by_role "link")

if assert_ref_found "$REF" "link ref"; then
  pt_post /action "{\"kind\":\"click\",\"ref\":\"${REF}\",\"waitNav\":true}"
  assert_ok "click link with waitNav"
else
  pt_post /action '{"kind":"click","selector":"a","waitNav":true}'
  assert_ok "click anchor with waitNav"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab check/uncheck (CSS selector)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate"

pt_post /action '{"kind":"check","selector":"#terms"}'
assert_ok "check checkbox"
assert_json_eq "$RESULT" '.result.checked' 'true' "check response reports checked"

pt_post /evaluate '{"expression":"document.querySelector(\"#terms\").checked"}'
assert_ok "evaluate checked state"
assert_json_eq "$RESULT" '.result' 'true' "checkbox is checked in DOM"

pt_post /action '{"kind":"uncheck","selector":"#terms"}'
assert_ok "uncheck checkbox"
assert_json_eq "$RESULT" '.result.checked' 'false' "uncheck response reports unchecked"

pt_post /evaluate '{"expression":"document.querySelector(\"#terms\").checked"}'
assert_ok "evaluate unchecked state"
assert_json_eq "$RESULT" '.result' 'false' "checkbox is unchecked in DOM"

end_test
