#!/bin/bash
# tab-default-basic.sh — commands with no --tab act on the CLI's current tab
# (the one `nav` wrote to the state file), not on whichever tab another client
# last touched on the shared server. Covers text, drag, mouse down and mouse up,
# the verbs whose PreRunE once shadowed the state-file default (PIN-431).

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# eval_on_tab runs an expression on one tab over HTTP and prints its result.
# An explicit-tab request also makes that tab the server's current tab.
eval_on_tab() {
  local tab_id="$1" expr="$2"
  e2e_curl -s -X POST "${E2E_SERVER}/evaluate" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg t "$tab_id" --arg e "$expr" '{tabId: $t, expression: $e}')" |
    jq -r '.result'
}

drag_state() {
  eval_on_tab "$1" "JSON.stringify(window.dragDropState)"
}

# touch_b makes the other client's tab the server's last-touched tab, and
# proves it: a tab-less HTTP read now lands on B.
touch_b() {
  drag_state "$TAB_B" >/dev/null
  local body
  body=$(e2e_curl -s "${E2E_SERVER}/text?mode=raw")
  if [[ "$body" == *"marker-B"* ]]; then
    pass_assert "server's tab-less current tab is B"
  else
    fail_assert "server's tab-less current tab is not B (got: ${body:0:200})"
  fi
}

# ─────────────────────────────────────────────────────────────────
start_test "two clients: CLI owns tab A, another client last touched tab B"

pt_ok nav "${FIXTURES_URL}/drag-drop.html?label=A" --new-tab
TAB_A=$(echo "$PT_OUT" | tr -d '[:space:]')

NAV_B=$(e2e_curl -s -X POST "${E2E_SERVER}/navigate" \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"${FIXTURES_URL}/drag-drop.html?label=B\",\"newTab\":true}")
TAB_B=$(get_tab_id "$NAV_B")

if [ -n "$TAB_A" ] && [ -n "$TAB_B" ] && [ "$TAB_A" != "$TAB_B" ]; then
  pass_assert "two distinct tabs (A=$TAB_A B=$TAB_B)"
else
  fail_assert "expected two distinct tabs (A='$TAB_A' B='$TAB_B', nav B: ${NAV_B:0:200})"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text with no --tab reads the CLI's tab A"

touch_b
pt_ok text
assert_output_contains "marker-A" "text returns A's content"
assert_output_not_contains "marker-B" "text does not return B's content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab drag with no --tab drops on tab A, not B"

touch_b
pt_ok drag "#drag" "#drop"
assert_output_contains "OK" "drag reports OK"

assert_json_jq "$(drag_state "$TAB_A")" '.dropped == true' \
  "A's drop zone received the drop" "A's drop zone did not receive the drop"
assert_json_jq "$(drag_state "$TAB_B")" '.dropped == false and .down == 0' \
  "B is untouched by the drag" "drag mutated B"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab mouse down / mouse up with no --tab press on tab A, not B"

A_BEFORE=$(drag_state "$TAB_A")
DOWN_BEFORE=$(echo "$A_BEFORE" | jq -r '.down')
UP_BEFORE=$(echo "$A_BEFORE" | jq -r '.up')

touch_b
pt_ok mouse down "#drop" --button left
assert_output_contains "OK" "mouse down reports OK"
touch_b
pt_ok mouse up "#drop" --button left
assert_output_contains "OK" "mouse up reports OK"

assert_json_jq "$(drag_state "$TAB_A")" \
  ".down == ($DOWN_BEFORE + 1) and .up == ($UP_BEFORE + 1)" \
  "A received one mouse down and one mouse up" "A did not receive the mouse down/up"
assert_json_jq "$(drag_state "$TAB_B")" '.down == 0 and .up == 0' \
  "B received no mouse down/up" "mouse down/up landed on B"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text --markdown --full still refuses locally"

pt_fail text --markdown --full

end_test

pt tab close "$TAB_A" >/dev/null
pt tab close "$TAB_B" >/dev/null
