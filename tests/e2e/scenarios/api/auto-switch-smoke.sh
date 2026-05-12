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
      echo -e "  ${GREEN}✓${NC} ${desc} (${actual} <= ${max})"
      ((ASSERTIONS_PASSED++)) || true
      return 0
    fi
    sleep 0.2
  done

  echo -e "  ${RED}✗${NC} ${desc} (tab count ${actual} > ${max})"
  ((ASSERTIONS_FAILED++)) || true
  return 1
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
  echo -e "  ${RED}✗${NC} result.switchedToTab missing"
  ((ASSERTIONS_FAILED++)) || true
else
  echo -e "  ${GREEN}✓${NC} switchedToTab returned: $NEW_TAB_ID"
  ((ASSERTIONS_PASSED++)) || true
fi

if [ "$NEW_TAB_ID" = "$OPENER_TAB_ID" ]; then
  echo -e "  ${RED}✗${NC} switchedToTab equals opener tab"
  ((ASSERTIONS_FAILED++)) || true
fi

AFTER=$(get_tab_count)
if [ "$AFTER" -le "$BEFORE" ]; then
  echo -e "  ${RED}✗${NC} expected tab count to increase, before=${BEFORE} after=${AFTER}"
  ((ASSERTIONS_FAILED++)) || true
else
  echo -e "  ${GREEN}✓${NC} tab count increased (${BEFORE} → ${AFTER})"
  ((ASSERTIONS_PASSED++)) || true
fi

pt_get /text
assert_ok "unscoped /text after auto-switch"
assert_json_contains "$RESULT" ".url" "index.html" "current tab URL is the opened tab"
assert_json_contains "$RESULT" ".text" "Welcome to the E2E test fixtures" "current tab text is from the opened tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "window.open auto-switches current tab"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/target-blank.html\"}"
pt_post /action -d '{"kind":"click","selector":"#window-open"}'
assert_ok "click window.open button"
assert_json_exists "$RESULT" '.result.switchedToTab' "switchedToTab set for window.open"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "regular same-tab click is unaffected"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/target-blank.html\"}"
pt_post /action -d '{"kind":"click","selector":"#same-tab","waitNav":true}'
assert_ok "same-tab click"

SWITCHED=$(echo "$RESULT" | jq -r '.result.switchedToTab // empty')
if [ -n "$SWITCHED" ]; then
  echo -e "  ${RED}✗${NC} unexpected switchedToTab on same-tab click: $SWITCHED"
  ((ASSERTIONS_FAILED++)) || true
else
  echo -e "  ${GREEN}✓${NC} no switchedToTab on same-tab click"
  ((ASSERTIONS_PASSED++)) || true
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
  echo -e "  ${RED}✗${NC} switchedToTab set despite autoSwitch=false: $SWITCHED"
  ((ASSERTIONS_FAILED++)) || true
else
  echo -e "  ${GREEN}✓${NC} no switchedToTab when autoSwitch=false (popup guard closes the popup)"
  ((ASSERTIONS_PASSED++)) || true
fi

wait_for_tab_count_at_most "$BEFORE_DISABLED" "popup was closed when autoSwitch=false"

pt_get /text
assert_ok "unscoped /text after autoSwitch=false"
assert_contains "$RESULT" "Opener page" "current tab remains on the opener"

end_test
