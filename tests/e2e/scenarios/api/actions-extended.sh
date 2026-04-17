#!/bin/bash
# actions-extended.sh — API advanced action scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"
source "${GROUP_DIR}/../../helpers/api-snapshot.sh"
source "${GROUP_DIR}/../../helpers/api-actions.sh"

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick by ref"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Increment")][0].ref // empty')

pt_post /action -d "{\"kind\":\"dblclick\",\"ref\":\"$REF\"}"
assert_ok "dblclick by ref"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick by CSS selector"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_post /action -d "{\"kind\":\"dblclick\",\"selector\":\"#increment\"}"
assert_ok "dblclick by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick by coordinates"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_post /action -d "{\"kind\":\"dblclick\",\"x\":100,\"y\":100,\"hasXY\":true}"
assert_ok "dblclick by coordinates"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "CLI: pinchtab dblclick <ref>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Increment")][0].ref // empty')

run_cli dblclick "$REF"
assert_ok "CLI dblclick by ref"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "CLI: pinchtab dblclick --css <selector>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

run_cli dblclick --css "#increment"
assert_ok "CLI dblclick by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "CLI: pinchtab dblclick --tab <id>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\",\"newTab\":true}"
assert_ok "navigate for new tab"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Increment")][0].ref // empty')

run_cli dblclick "$REF" --tab "$TAB_ID"
assert_ok "CLI dblclick with --tab flag"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick validation - missing ref/selector/coordinates"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"

pt_post /action -d "{\"kind\":\"dblclick\"}"
assert_error "dblclick without parameters should fail"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "humanClick: click input by ref"

navigate_fixture "human-type.html"
fresh_snapshot

require_ref "textbox" "Email" EMAIL_REF && \
  action_human_click "$EMAIL_REF"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "humanType: type into input by ref"

fresh_snapshot
require_ref "textbox" "Email" EMAIL_REF && {
  action_human_type "$EMAIL_REF" "test@example.com"

  fresh_snapshot
  assert_value "textbox" "Email" "test@example.com"
}

end_test

# ─────────────────────────────────────────────────────────────────
start_test "humanType: type into second input by ref"

fresh_snapshot
require_ref "textbox" "Name" NAME_REF && {
  action_human_type "$NAME_REF" "John Doe"

  fresh_snapshot
  assert_value "textbox" "Name" "John Doe"
}

end_test

# ─────────────────────────────────────────────────────────────────
start_test "humanType: type with CSS selector"

action_human_type_selector "#name" " Jr."

end_test

# ─────────────────────────────────────────────────────────────────
start_test "fill: textarea by ref"

navigate_fixture "text-fields.html"
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

navigate_fixture "text-fields.html"
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

navigate_fixture "iframe.html"
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

navigate_fixture "iframe.html"
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

navigate_fixture "nested-iframe.html"
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

navigate_fixture "srcdoc-iframe.html"
sleep 1
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

navigate_fixture "iframe-rerender.html"
fresh_snapshot

require_ref "textbox" "Legacy field" LEGACY_REF && {
  pt_post /evaluate -d '{"expression":"window.replaceTestFrame()"}'
  assert_ok "trigger iframe rerender"

  sleep 1

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

navigate_fixture "cross-origin-iframe.html"
sleep 1
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
sleep 1

pt_post /action -d '{"kind":"type","selector":"#username","text":"testuser"}'
assert_ok "type into username"

pt_post /action -d '{"kind":"press","key":"Enter"}'
assert_ok "press Enter"

assert_input_not_contains "#username" "Enter" "Enter key should dispatch event, not type text (bug #236)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "press Tab: does not type 'Tab' as text"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

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
sleep 1

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
sleep 0.5

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
