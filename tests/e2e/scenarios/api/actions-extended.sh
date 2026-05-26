#!/bin/bash
# actions-extended.sh — API advanced action scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"
source "${GROUP_DIR}/../../helpers/api-snapshot.sh"
source "${GROUP_DIR}/../../helpers/api-actions.sh"

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick by ref"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"

pt_get /snapshot
REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Increment")][0].ref // empty')

pt_post /action -d "{\"kind\":\"dblclick\",\"ref\":\"$REF\"}"
assert_ok "dblclick by ref"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick by CSS selector"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"

pt_post /action -d "{\"kind\":\"dblclick\",\"selector\":\"#increment\"}"
assert_ok "dblclick by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick by coordinates"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"

pt_post /action -d "{\"kind\":\"dblclick\",\"x\":100,\"y\":100,\"hasXY\":true}"
assert_ok "dblclick by coordinates"

end_test

start_test "HTTP: dblclick validation - missing ref/selector/coordinates"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"

pt_post /action -d "{\"kind\":\"dblclick\"}"
assert_not_ok "dblclick without parameters should fail"

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
start_test "pinchtab occluded click supports dom and dispatch modes"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/occluded-click.html\"}"
assert_ok "navigate"

pt_get "/snapshot?filter=interactive"
assert_ok "snapshot"
require_ref "button" "Proceed" TARGET_REF && {
  pt_post /action "{\"kind\":\"click\",\"ref\":\"${TARGET_REF}\"}"
  assert_http_status "500" "occluded ref click returns action failure"
  assert_json_contains "$RESULT" '.error' 'element is occluded' "occluded error surfaced"

  pt_get "/box?ref=${TARGET_REF}"
  assert_ok "box lookup by ref"
  assert_result_jq '.box.width > 0 and .box.height > 0' "box returns positive dimensions" "box dimensions missing"

  pt_post /action "{\"kind\":\"mouse-move\",\"ref\":\"${TARGET_REF}\"}"
  assert_ok "mouse-move by ref yields coordinates"
  assert_result_jq '.result.x > 0 and .result.y > 0' "mouse-move exposes positive coordinates" "mouse-move coordinates missing"

  pt_post /action "{\"kind\":\"click\",\"ref\":\"${TARGET_REF}\",\"mode\":\"dom\"}"
  assert_ok "dom mode click succeeds despite occlusion"

  pt_post /evaluate '{"expression":"window.occludedClickState"}'
  assert_ok "evaluate click state after dom mode"
  assert_json_eq "$RESULT" '.result.clicked' 'true' "dom mode triggered click"
  assert_json_eq "$RESULT" '.result.clicks' '1' "dom mode fired one click"

  pt_post /evaluate '{"expression":"window.occludedClickState = { clicked: false, clicks: 0, lastClientX: null, lastClientY: null }; window.occludedClickState"}'
  assert_ok "reset click state"

  pt_post /action "{\"kind\":\"click\",\"ref\":\"${TARGET_REF}\",\"mode\":\"dispatch\"}"
  assert_ok "dispatch mode click succeeds despite occlusion"

  pt_post /evaluate '{"expression":"window.occludedClickState"}'
  assert_ok "evaluate click state after dispatch mode"
  assert_json_eq "$RESULT" '.result.clicked' 'true' "dispatch mode triggered click"
  assert_json_eq "$RESULT" '.result.clicks' '1' "dispatch mode fired one click"
}

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
start_test "click with humanize: click input by ref"

navigate_fixture "human-type.html"
fresh_snapshot

require_ref "textbox" "Email" EMAIL_REF && \
  action_click_humanized "$EMAIL_REF"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "type with humanize: type into input by ref"

fresh_snapshot
require_ref "textbox" "Email" EMAIL_REF && {
  action_type_humanized "$EMAIL_REF" "test@example.com"

  fresh_snapshot
  assert_value "textbox" "Email" "test@example.com"
}

end_test

# ─────────────────────────────────────────────────────────────────
start_test "type with humanize: type into second input by ref"

fresh_snapshot
require_ref "textbox" "Name" NAME_REF && {
  action_type_humanized "$NAME_REF" "John Doe"

  fresh_snapshot
  assert_value "textbox" "Name" "John Doe"
}

end_test

# ─────────────────────────────────────────────────────────────────
start_test "type with humanize: type with CSS selector"

action_type_humanized_selector "#name" " Jr."

end_test

# ─────────────────────────────────────────────────────────────────
start_test "fill: textarea by ref"

navigate_fixture "text-fields.html" 0
fresh_snapshot

require_ref "textbox" "Notes" NOTES_REF && {
  pt_post /action -d "{\"kind\":\"fill\",\"ref\":\"${NOTES_REF}\",\"text\":\"textarea fill test\"}"
  assert_ok "fill textarea by ref"

  pt_post /evaluate -d '{"expression":"document.querySelector(\"#notes\").value"}'
  assert_ok "evaluate textarea after fill"
  assert_json_eq "$RESULT" '.result' 'textarea fill test' "textarea value persisted after fill"
}

end_test

# ─────────────────────────────────────────────────────────────────
start_test "type: textarea by ref"

navigate_fixture "text-fields.html" 0
fresh_snapshot

require_ref "textbox" "Notes" NOTES_REF && {
  action_type "$NOTES_REF" "textarea type test"

  pt_post /evaluate -d '{"expression":"document.querySelector(\"#notes\").value"}'
  assert_ok "evaluate textarea after type"
  assert_json_eq "$RESULT" '.result' 'textarea type test' "textarea value persisted after type"
}

end_test

# ─────────────────────────────────────────────────────────────────
start_test "iframe: snapshot refs support fill and click inside iframe"

navigate_fixture "iframe.html" 0
fresh_snapshot

assert_json_exists "$RESULT" '.nodes[] | select(.name == "payment-frame")' "snapshot includes iframe owner"

require_ref "textbox" "Card number" CARD_REF && \
require_ref "button" "Pay" PAY_REF && {
  pt_post /action -d "{\"kind\":\"fill\",\"ref\":\"${CARD_REF}\",\"text\":\"4111111111111111\"}"
  assert_ok "fill iframe textbox by ref"

  pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${PAY_REF}\"}"
  assert_ok "click iframe button by ref"

  pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"payment-frame\"); const doc = frame && frame.contentDocument; const result = doc && doc.getElementById(\"payment-result\"); return result ? result.textContent : \"\"; })()"}'
  assert_ok "evaluate iframe result"
  assert_json_eq "$RESULT" '.result' 'PAYMENT_SUBMITTED_4111111111111111' "iframe form submitted via refs"
}

end_test

# ─────────────────────────────────────────────────────────────────
start_test "iframe: frame-scoped selectors support focus hover select check scroll and click"

navigate_fixture "iframe.html" 0
fresh_snapshot

pt_get "/snapshot?filter=interactive&selector=%23card-number"
assert_not_ok "unscoped snapshot selector stays in main frame"

pt_post /frame -d '{"target":"#payment-frame"}'
assert_ok "set frame scope from iframe selector"
assert_json_eq "$RESULT" '.scoped' 'true' "frame scope enabled"

pt_get "/snapshot?filter=interactive"
assert_ok "frame-scoped snapshot"
assert_json_exists "$RESULT" '.nodes[] | select(.name == "Card number")' "frame snapshot includes card field"
assert_json_exists "$RESULT" '.nodes[] | select(.name == "Billing country")' "frame snapshot includes select"
assert_json_exists "$RESULT" '.nodes[] | select(.name == "Save card for later")' "frame snapshot includes checkbox"

pt_post /action -d '{"kind":"focus","selector":"#card-number"}'
assert_ok "focus selector inside frame"
pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"payment-frame\"); const doc = frame && frame.contentDocument; return doc && doc.activeElement ? doc.activeElement.id : \"\"; })()"}'
assert_ok "evaluate active element in iframe"
assert_json_eq "$RESULT" '.result' 'card-number' "focus targeted iframe input"

pt_post /action -d '{"kind":"type","selector":"#card-number","text":"5555444433331111"}'
assert_ok "type selector inside frame"
pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"payment-frame\"); const doc = frame && frame.contentDocument; const input = doc && doc.getElementById(\"card-number\"); return input ? input.value : \"\"; })()"}'
assert_ok "evaluate typed card number"
assert_json_eq "$RESULT" '.result' '5555444433331111' "type persisted in iframe input"

pt_post /action -d '{"kind":"hover","selector":"#hover-target"}'
assert_ok "hover selector inside frame"
pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"payment-frame\"); const doc = frame && frame.contentDocument; const result = doc && doc.getElementById(\"hover-result\"); return result ? result.textContent : \"\"; })()"}'
assert_ok "evaluate hover result"
assert_json_eq "$RESULT" '.result' 'HOVERED_1' "hover handler fired inside iframe"

pt_post /action -d '{"kind":"select","selector":"#billing-country","value":"uk"}'
assert_ok "select option inside frame"
pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"payment-frame\"); const doc = frame && frame.contentDocument; const select = doc && doc.getElementById(\"billing-country\"); return select ? select.value : \"\"; })()"}'
assert_ok "evaluate selected country"
assert_json_eq "$RESULT" '.result' 'uk' "select updated iframe dropdown"

pt_post /action -d '{"kind":"check","selector":"#save-card"}'
assert_ok "check checkbox inside frame"
pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"payment-frame\"); const doc = frame && frame.contentDocument; const checkbox = doc && doc.getElementById(\"save-card\"); return checkbox ? checkbox.checked : false; })()"}'
assert_ok "evaluate checked state in iframe"
assert_json_eq "$RESULT" '.result' 'true' "checkbox checked inside iframe"

pt_post /action -d '{"kind":"uncheck","selector":"#save-card"}'
assert_ok "uncheck checkbox inside frame"
pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"payment-frame\"); const doc = frame && frame.contentDocument; const checkbox = doc && doc.getElementById(\"save-card\"); return checkbox ? checkbox.checked : true; })()"}'
assert_ok "evaluate unchecked state in iframe"
assert_json_eq "$RESULT" '.result' 'false' "checkbox unchecked inside iframe"

pt_post /action -d '{"kind":"scrollintoview","selector":"#deep-target"}'
assert_ok "scrollintoview selector inside frame"
pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"payment-frame\"); const doc = frame && frame.contentDocument; const scrolling = doc && doc.scrollingElement; return scrolling ? scrolling.scrollTop > 0 : false; })()"}'
assert_ok "evaluate iframe scroll position"
assert_json_eq "$RESULT" '.result' 'true' "scrollintoview scrolled iframe document"

pt_post /action -d '{"kind":"click","selector":"#deep-target"}'
assert_ok "click selector inside frame after scroll"
pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"payment-frame\"); const doc = frame && frame.contentDocument; const result = doc && doc.getElementById(\"scroll-result\"); return result ? result.textContent : \"\"; })()"}'
assert_ok "evaluate deep target click result"
assert_json_eq "$RESULT" '.result' 'DEEP_TARGET_CLICKED' "click worked inside frame scope"

pt_post /frame -d '{"target":"main"}'
assert_ok "reset frame scope to main"
assert_json_eq "$RESULT" '.scoped' 'false' "frame scope cleared"

pt_get "/snapshot?filter=interactive&selector=%23card-number"
assert_not_ok "selector stops reaching iframe after frame reset"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "iframe: nested frames require explicit frame hops"

navigate_fixture "nested-iframe.html" 0
fresh_snapshot

pt_get "/snapshot?filter=interactive&selector=%23nested-code"
assert_not_ok "nested selector blocked from main frame"

pt_post /frame -d '{"target":"#outer-frame"}'
assert_ok "set outer frame scope"
assert_json_eq "$RESULT" '.scoped' 'true' "outer frame scope enabled"

pt_post /action -d '{"kind":"focus","selector":"#nested-code"}'
assert_not_ok "inner selector still blocked from outer frame"

pt_post /frame -d '{"target":"#inner-payment-frame"}'
assert_ok "set inner frame scope from outer frame"
assert_json_eq "$RESULT" '.frame.frameName' 'inner-payment-frame' "inner frame selected"

pt_post /action -d '{"kind":"type","selector":"#nested-code","text":"nested-123"}'
assert_ok "type inside nested inner frame"

pt_post /action -d '{"kind":"check","selector":"#nested-consent"}'
assert_ok "check checkbox inside nested inner frame"

pt_post /action -d '{"kind":"click","selector":"#nested-save"}'
assert_ok "click inside nested inner frame"

pt_post /evaluate -d '{"expression":"(() => { const outer = document.getElementById(\"outer-frame\"); const outerDoc = outer && outer.contentDocument; const inner = outerDoc && outerDoc.getElementById(\"inner-payment-frame\"); const innerDoc = inner && inner.contentDocument; const result = innerDoc && innerDoc.getElementById(\"nested-result\"); return result ? result.textContent : \"\"; })()"}'
assert_ok "evaluate nested iframe result"
assert_json_eq "$RESULT" '.result' 'NESTED_SAVED_nested-123_true' "nested frame actions succeeded"

pt_post /frame -d '{"target":"main"}'
assert_ok "reset nested frame scope to main"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "iframe: srcdoc frame-scoped selectors work"

navigate_fixture "srcdoc-iframe.html" 0
fresh_snapshot

pt_post /frame -d '{"target":"#srcdoc-payment-frame"}'
assert_ok "set srcdoc frame scope"
assert_json_eq "$RESULT" '.scoped' 'true' "srcdoc frame scope enabled"

pt_get "/snapshot?filter=interactive"
assert_ok "srcdoc frame-scoped snapshot"
assert_json_exists "$RESULT" '.nodes[] | select(.name == "Srcdoc code")' "srcdoc snapshot includes textbox"
assert_json_exists "$RESULT" '.nodes[] | select(.name | test("Srcdoc opt-in"))' "srcdoc snapshot includes checkbox"

pt_post /action -d '{"kind":"focus","selector":"#srcdoc-code"}'
assert_ok "focus selector inside srcdoc frame"

pt_post /action -d '{"kind":"type","selector":"#srcdoc-code","text":"srcdoc-321"}'
assert_ok "type selector inside srcdoc frame"

pt_post /action -d '{"kind":"check","selector":"#srcdoc-optin"}'
assert_ok "check selector inside srcdoc frame"

pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"srcdoc-payment-frame\"); const doc = frame && frame.contentDocument; const input = doc && doc.getElementById(\"srcdoc-code\"); const checkbox = doc && doc.getElementById(\"srcdoc-optin\"); const active = doc && doc.activeElement; return { value: input ? input.value : \"\", checked: checkbox ? checkbox.checked : false, active: active ? active.id : \"\" }; })()"}'
assert_ok "evaluate srcdoc iframe state"
assert_json_eq "$RESULT" '.result.value' 'srcdoc-321' "srcdoc input updated"
assert_json_eq "$RESULT" '.result.checked' 'true' "srcdoc checkbox updated"

pt_post /frame -d '{"target":"main"}'
assert_ok "reset srcdoc frame scope to main"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "iframe: stale refs after rerender require a fresh snapshot"

navigate_fixture "iframe-rerender.html" 0
fresh_snapshot

require_ref "textbox" "Legacy field" LEGACY_REF && {
  pt_post /evaluate -d '{"expression":"window.replaceTestFrame()"}'
  assert_ok "trigger iframe rerender"

  assert_eval_poll '(() => { const frame = document.getElementById("live-frame"); const doc = frame && frame.contentDocument; return !!(doc && doc.getElementById("fresh-field")); })()' "true" "fresh iframe loaded after rerender" 10 0.05

  pt_post /action -d "{\"kind\":\"fill\",\"ref\":\"${LEGACY_REF}\",\"text\":\"stale\"}"
  assert_not_ok "stale iframe ref rejected after rerender"

  fresh_snapshot
  assert_json_not_exists "$RESULT" '.nodes[] | select(.name == "Legacy field")' "legacy field removed after rerender"

  require_ref "textbox" "Fresh field" FRESH_REF && \
  require_ref "button" "Fresh save" FRESH_SAVE_REF && {
    pt_post /action -d "{\"kind\":\"fill\",\"ref\":\"${FRESH_REF}\",\"text\":\"fresh-456\"}"
    assert_ok "fresh iframe ref works after resnapshot"

    pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${FRESH_SAVE_REF}\"}"
    assert_ok "fresh iframe button click works after resnapshot"

    pt_post /evaluate -d '{"expression":"(() => { const frame = document.getElementById(\"live-frame\"); const doc = frame && frame.contentDocument; const result = doc && doc.getElementById(\"fresh-result\"); return result ? result.textContent : \"\"; })()"}'
    assert_ok "evaluate rerendered iframe result"
    assert_json_eq "$RESULT" '.result' 'FRESH_SAVED_fresh-456' "fresh refs work after rerender"
  }
}

end_test

# ─────────────────────────────────────────────────────────────────
start_test "iframe: cross-origin selector scope is not claimed as supported"

navigate_fixture "cross-origin-iframe.html" 0
fresh_snapshot

pt_post /frame -d '{"target":"#cross-origin-frame"}'
assert_not_ok "cross-origin frame selection is currently unsupported"

pt_get "/snapshot?filter=interactive"
assert_ok "cross-origin frame-scoped snapshot"
assert_json_exists "$RESULT" '.nodes[] | select(.name == "cross-origin-frame")' "cross-origin snapshot keeps iframe owner only"
assert_json_not_exists "$RESULT" '.nodes[] | select(.name == "Cross-origin code")' "cross-origin snapshot does not inline input descendants"
assert_json_not_exists "$RESULT" '.nodes[] | select(.name == "Cross-origin send")' "cross-origin snapshot does not inline button descendants"

pt_post /action -d '{"kind":"type","selector":"#cross-origin-code","text":"424242"}'
assert_not_ok "unscoped selector cannot type into cross-origin iframe"

end_test

# Regression test for GitHub issue #236: press action was typing key names
# as literal text instead of dispatching keyboard events.

# Use permissive instance (needs evaluate enabled)
E2E_SERVER="http://pinchtab:9999"

# ─────────────────────────────────────────────────────────────────
start_test "press Enter: does not type 'Enter' as text"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"

pt_post /action -d '{"kind":"type","selector":"#username","text":"testuser"}'
assert_ok "type into username"

pt_post /action -d '{"kind":"press","key":"Enter"}'
assert_ok "press Enter"

assert_input_not_contains "#username" "Enter" "Enter key should dispatch event, not type text (bug #236)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "press Tab: does not type 'Tab' as text"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"

pt_post /action -d '{"kind":"click","selector":"#username"}'
pt_post /action -d '{"kind":"type","selector":"#username","text":"hello"}'
assert_ok "type hello"

pt_post /action -d '{"kind":"press","key":"Tab"}'
assert_ok "press Tab"

assert_input_not_contains "#username" "Tab" "Tab key should dispatch event, not type text (bug #236)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "press Escape: does not type 'Escape' as text"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"

pt_post /action -d '{"kind":"type","selector":"#username","text":"world"}'
assert_ok "type world"

pt_post /action -d '{"kind":"press","key":"Escape"}'
assert_ok "press Escape"

assert_input_not_contains "#username" "Escape" "Escape key should dispatch event, not type text (bug #236)"

end_test

# Migrated from: tests/integration/actions_test.go (error cases)

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"

# ─────────────────────────────────────────────────────────────────
start_test "action: unknown kind → error"

pt_post /action '{"kind":"explode","ref":"e0"}'
assert_not_ok "rejects unknown kind"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "action: missing kind → error"

pt_post /action '{"ref":"e0"}'
assert_http_status "400" "rejects missing kind"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "action: ref not found → error"

pt_post /action '{"kind":"click","ref":"e999"}'
assert_not_ok "rejects missing ref"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "action: batch operations"

pt_post /actions '{"actions":[{"kind":"click","ref":"e4"},{"kind":"click","ref":"e5"}]}'
assert_ok "batch actions"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "action: empty batch → error"

pt_post /actions '{"actions":[]}'
assert_not_ok "rejects empty batch"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "action: nonexistent tabId → error"

pt_post /action '{"kind":"click","ref":"e0","tabId":"nonexistent_xyz_999"}'
assert_not_ok "rejects bad tab"

end_test

# ─────────────────────────────────────────────────────────────────
# POST /dialog — JavaScript dialog handling
# ─────────────────────────────────────────────────────────────────

start_test "POST /dialog: invalid JSON"

pt_post_raw /dialog "not json"
assert_http_status "400" "rejects invalid JSON"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /dialog: missing action"

pt_post /dialog '{}'
assert_http_status "400" "rejects missing action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /dialog: invalid action"

pt_post /dialog '{"action":"invalid"}'
assert_http_status "400" "rejects invalid action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /dialog: no pending dialog"

# Navigate to a page that has no dialog
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"

# Try to accept a dialog when none is pending
pt_post /dialog '{"action":"accept"}'
# Should fail because no dialog is pending
assert_http_status "400" "rejects when no dialog pending"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /dialog: accept action format"

# This test verifies the request format is accepted
# Note: Actually triggering and accepting a dialog requires JS execution
# which may not be enabled in all test configurations

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

# Try accept with tabId - should fail gracefully if no dialog
pt_post /dialog "{\"action\":\"accept\",\"tabId\":\"${TAB_ID}\"}"
# Expected: 400 (no pending dialog) - this confirms the endpoint works
assert_http_status "400" "accept request format valid (no pending dialog)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /dialog: dismiss action format"

# Try dismiss with text - should fail gracefully if no dialog
pt_post /dialog "{\"action\":\"dismiss\",\"tabId\":\"${TAB_ID}\"}"
assert_http_status "400" "dismiss request format valid (no pending dialog)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /dialog: accept with prompt text"

# Test the prompt text parameter format
pt_post /dialog "{\"action\":\"accept\",\"tabId\":\"${TAB_ID}\",\"text\":\"my response\"}"
# Should fail because no dialog is pending, but format is valid
assert_http_status "400" "accept with text format valid (no pending dialog)"

end_test

# ─────────────────────────────────────────────────────────────────
# JavaScript dialog handling via click --dialog-action
# ─────────────────────────────────────────────────────────────────

start_test "click alert without dialogAction: fast-fail with dialog_blocking error"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons"

pt_get /snapshot
ALERT_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Trigger Alert")][0].ref // empty')

# Click without dialogAction should fail fast with dialog_blocking error
_CLICK_T0=$(get_time_ms)
pt_post /action "{\"kind\":\"click\",\"ref\":\"${ALERT_REF}\"}"
_CLICK_T1=$(get_time_ms)

assert_not_ok "click without dialogAction fails"
assert_json_eq "$RESULT" '.code' 'dialog_blocking' "error code is dialog_blocking"

# Verify fast-fail: should complete in under 2 seconds (not 30s timeout)
_CLICK_ELAPSED=$((_CLICK_T1 - _CLICK_T0))
if [ "$_CLICK_ELAPSED" -gt 2000 ]; then
  echo "FAIL: click took ${_CLICK_ELAPSED}ms, expected fast-fail under 2000ms"
  exit 1
fi
echo "PASS: fast-fail in ${_CLICK_ELAPSED}ms"

# Clean up: dismiss the pending dialog
pt_post /dialog '{"action":"dismiss"}'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "click alert with dialogAction accept: dialog dismissed"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons"

pt_get /snapshot
ALERT_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Trigger Alert")][0].ref // empty')

pt_post /action "{\"kind\":\"click\",\"ref\":\"${ALERT_REF}\",\"dialogAction\":\"accept\"}"
assert_ok "click with dialogAction accept"

# Verify the click handler completed (alert was dismissed)
pt_post /evaluate -d '{"expression":"document.getElementById(\"dialog-result\").textContent"}'
assert_ok "evaluate dialog result"
assert_json_eq "$RESULT" '.result' 'ALERT_DISMISSED' "alert was dismissed and handler completed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "click confirm with dialogAction dismiss: confirm cancelled"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons"

pt_get /snapshot
CONFIRM_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Trigger Confirm")][0].ref // empty')

pt_post /action "{\"kind\":\"click\",\"ref\":\"${CONFIRM_REF}\",\"dialogAction\":\"dismiss\"}"
assert_ok "click with dialogAction dismiss"

# Verify confirm returned false (dismissed)
pt_post /evaluate -d '{"expression":"document.getElementById(\"dialog-result\").textContent"}'
assert_ok "evaluate dialog result"
assert_json_eq "$RESULT" '.result' 'CONFIRM_DISMISSED' "confirm was dismissed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "click confirm with dialogAction accept: confirm accepted"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons"

pt_get /snapshot
CONFIRM_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Trigger Confirm")][0].ref // empty')

pt_post /action "{\"kind\":\"click\",\"ref\":\"${CONFIRM_REF}\",\"dialogAction\":\"accept\"}"
assert_ok "click with dialogAction accept"

# Verify confirm returned true (accepted)
pt_post /evaluate -d '{"expression":"document.getElementById(\"dialog-result\").textContent"}'
assert_ok "evaluate dialog result"
assert_json_eq "$RESULT" '.result' 'CONFIRM_ACCEPTED' "confirm was accepted"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "click prompt with dialogAction accept and text: prompt value returned"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons"

pt_get /snapshot
PROMPT_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Trigger Prompt")][0].ref // empty')

pt_post /action "{\"kind\":\"click\",\"ref\":\"${PROMPT_REF}\",\"dialogAction\":\"accept\",\"dialogText\":\"e2e_input\"}"
assert_ok "click with dialogAction accept and dialogText"

# Verify prompt returned the provided text
pt_post /evaluate -d '{"expression":"document.getElementById(\"dialog-result\").textContent"}'
assert_ok "evaluate dialog result"
assert_json_eq "$RESULT" '.result' 'PROMPT_VALUE_e2e_input' "prompt returned provided text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "click prompt with dialogAction dismiss: prompt cancelled"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons"

pt_get /snapshot
PROMPT_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Trigger Prompt")][0].ref // empty')

pt_post /action "{\"kind\":\"click\",\"ref\":\"${PROMPT_REF}\",\"dialogAction\":\"dismiss\"}"
assert_ok "click with dialogAction dismiss"

# Verify prompt returned null (cancelled)
pt_post /evaluate -d '{"expression":"document.getElementById(\"dialog-result\").textContent"}'
assert_ok "evaluate dialog result"
assert_json_eq "$RESULT" '.result' 'PROMPT_CANCELLED' "prompt was cancelled"

end_test

# ─────────────────────────────────────────────────────────────────
# Regression: ref-based frame scoping (`/frame {target: "eN"}`).
# Three bugs were fixed together:
#   1) FrameOwnerMap called dom.GetFrameOwner without a chromedp.Run
#      wrapper, returning err="invalid context" → empty owner map.
#   2) FetchAXTree iterated frames root-first; pierce:true on the root
#      let the seen[] dedup drop child-frame nodes before they could be
#      tagged with their real FrameID/FrameOwnerNodeID.
#   3) resolveFrameScope silently reset to main when an iframe ref had
#      no ChildFrameID, returning HTTP 200 + scoped:false (looked OK).
# Together these meant `pinchtab frame eN` was a silent no-op even when
# eN was clearly an iframe in the snap.
start_test "iframe: /frame target=eN drills into child frame (ref-based scoping)"

navigate_fixture "iframe.html" 0
fresh_snapshot

IFRAME_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "Iframe")][0].ref // empty')
[ -n "$IFRAME_REF" ] || fail_assert "snapshot did not expose any Iframe ref"

# Fix 1+2: snapshot must populate childFrameId on the iframe owner ref.
assert_json_exists "$RESULT" \
  ".nodes[] | select(.ref == \"${IFRAME_REF}\") | select(.childFrameId != null and .childFrameId != \"\")" \
  "iframe ref ${IFRAME_REF} carries non-empty childFrameId"

# Fix 3 (positive path): drilling by ref must succeed and report the child URL.
pt_post /frame -d "{\"target\":\"${IFRAME_REF}\"}"
assert_ok "frame target=${IFRAME_REF} (ref) succeeds"
assert_json_eq "$RESULT" '.scoped' 'true' "ref-based frame scoping reports scoped=true"
assert_json_contains "$RESULT" '.current.frameUrl' 'iframe-content.html' \
  "scoped frame url points at iframe-content.html"

# Reset to main for the next case.
pt_post /frame -d '{"target":"main"}'
assert_json_eq "$RESULT" '.scoped' 'false' "reset back to main"

end_test

# ─────────────────────────────────────────────────────────────────
# Regression: /frame target=eN must error (not silently no-op) when the
# ref points to a non-iframe element in the main frame. Before fix #3
# this returned HTTP 200 with scoped:false — indistinguishable from a
# successful "reset to main" — making the bug invisible to clients.
start_test "iframe: /frame target=eN errors when ref is not an iframe owner"

navigate_fixture "iframe.html" 0
fresh_snapshot

# Pick any non-iframe ref in main (a heading or paragraph is fine).
NON_IFRAME_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role != "Iframe" and .role != "RootWebArea")][0].ref // empty')
[ -n "$NON_IFRAME_REF" ] || fail_assert "snapshot had no non-iframe ref to test against"

pt_post /frame -d "{\"target\":\"${NON_IFRAME_REF}\"}"
assert_http_error 400 "is not an iframe owner" \
  "frame target=${NON_IFRAME_REF} (non-iframe ref) returns 400 with iframe-owner message"

end_test

# ─────────────────────────────────────────────────────────────────
# Regression: the iframe owner ref discovered in the *outer* page snap
# must keep drilling correctly when the iframe sits inside another
# iframe (i.e. `Iframe` role discovered via the same-origin pierce).
# The flat snap of nested-iframe-outer.html exposes the inner-payment
# iframe directly because pierce:true descends into the outer frame.
start_test "iframe: /frame target=eN drills into iframe surfaced via same-origin pierce"

navigate_fixture "nested-iframe-outer.html" 0
fresh_snapshot

NESTED_IFRAME_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "Iframe")][0].ref // empty')
[ -n "$NESTED_IFRAME_REF" ] || fail_assert "no Iframe ref surfaced from pierced outer fixture"

pt_post /frame -d "{\"target\":\"${NESTED_IFRAME_REF}\"}"
assert_json_eq "$RESULT" '.scoped' 'true' "scoped into pierced inner iframe"
assert_json_contains "$RESULT" '.current.frameUrl' 'nested-iframe-inner.html' \
  "scoped frame url points at nested-iframe-inner.html"

pt_post /frame -d '{"target":"main"}'

end_test
