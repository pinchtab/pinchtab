#!/bin/bash
# auto-switch-smoke.sh — Verify click on target=_blank auto-switches the
# current tab to the newly opened popup, and that autoSwitch=false disables it.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

wait_for_tab_count_at_most() {
  local max="$1"
  local desc="$2"
  local attempts="${3:-20}"
  local actual=""

  for _ in $(seq 1 "$attempts"); do
    actual=$(get_tab_count)
    if [ "$actual" -le "$max" ]; then
      pass_assert "${desc} (${actual} <= ${max})"
      return 0
    fi
    sleep 0.2
  done

  fail_assert "${desc} (tab count ${actual} > ${max})"
  return 1
}

assert_opener_captured_window_open() {
  local opener_tab_id="$1"
  local desc="$2"

  pt_post /evaluate "$(jq -nc --arg tabId "$opener_tab_id" '{tabId:$tabId, expression:"document.body.dataset.captured || \"\""}')"
  assert_ok "read capture flag from opener tab"
  local captured
  captured=$(echo "$RESULT" | jq -r '.result // empty')
  if [ "$captured" = "true" ]; then
    pass_assert "${desc}"
  else
    fail_assert "${desc} (captured=${captured:-unset})"
  fi
}

# ─────────────────────────────────────────────────────────────────
start_test "click on target=_blank auto-switches current tab"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/target-blank.html\"}"
OPENER_TAB_ID=$(get_tab_id)
BEFORE=$(get_tab_count)

pt_post /action -d '{"kind":"click","selector":"#blank-link"}'
assert_ok "click target=_blank link"

NEW_TAB_ID=$(echo "$RESULT" | jq -r '.result.switchedToTab // empty')
if [ -z "$NEW_TAB_ID" ]; then
  fail_assert "result.switchedToTab missing"
else
  pass_assert "switchedToTab returned: $NEW_TAB_ID"
fi

if [ "$NEW_TAB_ID" = "$OPENER_TAB_ID" ]; then
  fail_assert "switchedToTab equals opener tab"
fi

AFTER=$(get_tab_count)
if [ "$AFTER" -le "$BEFORE" ]; then
  fail_assert "expected tab count to increase, before=${BEFORE} after=${AFTER}"
else
  pass_assert "tab count increased (${BEFORE} → ${AFTER})"
fi

pt_get /text
assert_ok "unscoped /text after auto-switch"
assert_json_contains "$RESULT" ".url" "index.html" "current tab URL is the opened tab"
assert_json_contains "$RESULT" ".text" "Welcome to the E2E test fixtures" "current tab text is from the opened tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "window.open auto-switches current tab"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/target-blank.html\"}"
OPENER_TAB_ID=$(get_tab_id)
pt_post /action -d '{"kind":"click","selector":"#window-open"}'
assert_ok "click window.open button"
assert_json_exists "$RESULT" '.result.switchedToTab' "switchedToTab set for window.open"
assert_opener_captured_window_open "$OPENER_TAB_ID" "selector click installed the window.open capture before onclick"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "humanized x/y click on window.open auto-switches current tab"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/target-blank.html\"}"
OPENER_TAB_ID=$(get_tab_id)

CENTER_EXPR="var r=document.getElementById('window-open').getBoundingClientRect(); JSON.stringify([Math.round(r.left+r.width/2), Math.round(r.top+r.height/2)])"
pt_post /evaluate "$(jq -nc --arg tabId "$OPENER_TAB_ID" --arg expression "$CENTER_EXPR" '{tabId:$tabId, expression:$expression}')"
assert_ok "read #window-open bounds"
CENTER=$(echo "$RESULT" | jq -r '.result // empty')
BUTTON_X=$(echo "$CENTER" | jq -r '.[0]')
BUTTON_Y=$(echo "$CENTER" | jq -r '.[1]')

pt_post /action -d "{\"kind\":\"click\",\"x\":${BUTTON_X},\"y\":${BUTTON_Y},\"humanize\":true}"
assert_ok "humanized x/y click on window.open button"

NEW_TAB_ID=$(echo "$RESULT" | jq -r '.result.switchedToTab // empty')
if [ -z "$NEW_TAB_ID" ]; then
  fail_assert "result.switchedToTab missing for humanized x/y click"
elif [ "$NEW_TAB_ID" = "$OPENER_TAB_ID" ]; then
  fail_assert "switchedToTab equals opener tab"
else
  pass_assert "switchedToTab returned: $NEW_TAB_ID"
fi

assert_opener_captured_window_open "$OPENER_TAB_ID" "humanized x/y click installed the window.open capture before onclick"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "regular same-tab click is unaffected"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/target-blank.html\"}"
pt_post /action -d '{"kind":"click","selector":"#same-tab","waitNav":true}'
assert_ok "same-tab click"

SWITCHED=$(echo "$RESULT" | jq -r '.result.switchedToTab // empty')
if [ -n "$SWITCHED" ]; then
  fail_assert "unexpected switchedToTab on same-tab click: $SWITCHED"
else
  pass_assert "no switchedToTab on same-tab click"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "autoSwitch=false disables auto-switch"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/target-blank.html\"}"
BEFORE_DISABLED=$(get_tab_count)
pt_post /action -d '{"kind":"click","selector":"#blank-link","autoSwitch":false}'
assert_ok "click with autoSwitch=false"

SWITCHED=$(echo "$RESULT" | jq -r '.result.switchedToTab // empty')
if [ -n "$SWITCHED" ]; then
  fail_assert "switchedToTab set despite autoSwitch=false: $SWITCHED"
else
  pass_assert "no switchedToTab when autoSwitch=false (popup guard closes the popup)"
fi

wait_for_tab_count_at_most "$BEFORE_DISABLED" "popup was closed when autoSwitch=false"

pt_get /text
assert_ok "unscoped /text after autoSwitch=false"
assert_contains "$RESULT" "Opener page" "current tab remains on the opener"

end_test
