#!/bin/bash
# actions-basic.sh — API happy-path action scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

ACTIONS_TAB_ID=""

actions_navigate() {
  local url="$1"
  local body
  if [ -n "$ACTIONS_TAB_ID" ]; then
    body=$(jq -nc --arg url "$url" --arg tabId "$ACTIONS_TAB_ID" '{url:$url, tabId:$tabId}')
  else
    body=$(jq -nc --arg url "$url" '{url:$url}')
  fi

  pt_post /navigate -d "$body"
  assert_ok "navigate"

  local tab_id
  tab_id=$(echo "$RESULT" | jq -r '.tabId // empty')
  if [ -n "$tab_id" ] && [ "$tab_id" != "null" ]; then
    ACTIONS_TAB_ID="$tab_id"
  fi
}

actions_snapshot() {
  local query="${1:-}"
  if [ -n "$query" ]; then
    pt_get "/snapshot?tabId=${ACTIONS_TAB_ID}&${query}"
  else
    pt_get "/snapshot?tabId=${ACTIONS_TAB_ID}"
  fi
}

actions_post_action() {
  local body="$1"
  local with_tab
  with_tab=$(echo "$body" | jq -c --arg tabId "$ACTIONS_TAB_ID" '. + {tabId:$tabId}')
  pt_post /action "$with_tab"
}

actions_evaluate() {
  local expression="$1"
  local body
  body=$(jq -nc --arg tabId "$ACTIONS_TAB_ID" --arg expression "$expression" '{tabId:$tabId, expression:$expression}')
  pt_post /evaluate "$body"
}

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click <button>"

actions_navigate "${FIXTURES_URL}/buttons.html"

actions_snapshot
REF=$(find_ref_by_name "Increment")
if assert_ref_found "$REF" "button Increment"; then
  actions_post_action "{\"kind\":\"click\",\"ref\":\"${REF}\"}"
  assert_ok "click button by ref"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab type <field> <text>"

actions_navigate "${FIXTURES_URL}/form.html"

actions_snapshot
REF=$(find_ref_by_name "Username")
if [ -z "$REF" ] || [ "$REF" = "null" ]; then
  REF=$(find_ref_by_role "textbox")
fi
if assert_ref_found "$REF" "Username textbox"; then
  actions_post_action "{\"kind\":\"type\",\"ref\":\"${REF}\",\"text\":\"testuser123\"}"
  assert_ok "type text by ref"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab press <key>"

actions_post_action '{"kind":"press","key":"Escape"}'
assert_ok "press Escape"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab click (CSS selector)"

actions_navigate "${FIXTURES_URL}/buttons.html"

actions_post_action '{"kind":"click","selector":"#increment"}'
assert_ok "click by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab type (CSS selector)"

actions_navigate "${FIXTURES_URL}/form.html"

actions_post_action '{"kind":"type","selector":"#username","text":"selectortest"}'
assert_ok "type by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snapshot (CSS selector filter)"

actions_navigate "${FIXTURES_URL}/form.html"

actions_snapshot "selector=%23username"
assert_ok "snapshot with selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab scroll (down)"

actions_navigate "${FIXTURES_URL}/table.html"

actions_post_action '{"kind":"scroll","direction":"down"}'
assert_ok "scroll down"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab focus (ref)"

actions_navigate "${FIXTURES_URL}/form.html"

actions_snapshot
REF=$(find_ref_by_role "textbox")
if assert_ref_found "$REF" "textbox ref"; then
  actions_post_action "{\"kind\":\"focus\",\"ref\":\"${REF}\"}"
  assert_ok "focus on input"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab select (combobox)"

actions_navigate "${FIXTURES_URL}/form.html"

actions_snapshot
REF=$(find_ref_by_role "combobox")
if assert_ref_found "$REF" "combobox ref"; then
  actions_post_action "{\"kind\":\"select\",\"ref\":\"${REF}\",\"value\":\"uk\"}"
  assert_ok "select option"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab fill (sets value + verifiable)"

actions_navigate "${FIXTURES_URL}/form.html"

actions_snapshot
REF=$(find_ref_by_role "textbox")
if assert_ref_found "$REF" "textbox ref"; then
  actions_post_action "{\"kind\":\"fill\",\"ref\":\"${REF}\",\"text\":\"e2e_fill_test\"}"
  assert_ok "fill input"

  actions_evaluate 'document.querySelector("#username").value'
  assert_ok "evaluate"
  assert_json_contains "$RESULT" '.result' 'e2e_fill_test' "fill value persisted"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab check/uncheck (CSS selector)"

actions_navigate "${FIXTURES_URL}/form.html"

actions_post_action '{"kind":"check","selector":"#terms"}'
assert_ok "check checkbox"
assert_json_eq "$RESULT" '.result.checked' 'true' "check response reports checked"

actions_evaluate 'document.querySelector("#terms").checked'
assert_ok "evaluate checked state"
assert_json_eq "$RESULT" '.result' 'true' "checkbox is checked in DOM"

actions_post_action '{"kind":"uncheck","selector":"#terms"}'
assert_ok "uncheck checkbox"
assert_json_eq "$RESULT" '.result.checked' 'false' "uncheck response reports unchecked"

actions_evaluate 'document.querySelector("#terms").checked'
assert_ok "evaluate unchecked state"
assert_json_eq "$RESULT" '.result' 'false' "checkbox is unchecked in DOM"

end_test
