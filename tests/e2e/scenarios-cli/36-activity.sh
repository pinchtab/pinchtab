#!/bin/bash
# 36-activity.sh — CLI activity commands

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab activity"

pt_ok nav "${FIXTURES_URL}/buttons.html"
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')

pt_ok snap --tab "$TAB_ID"
assert_output_json "snapshot output is valid JSON"

pt_ok click --tab "$TAB_ID" "#increment"
assert_output_contains "clicked" "click command completed"

pt_ok activity
assert_output_json "activity output is valid JSON"
assert_output_contains "\"events\"" "returns events payload"

if echo "$PT_OUT" | jq -e --arg tab "$TAB_ID" --arg path "/tabs/${TAB_ID}/action" '.events[] | select(.tabId == $tab and .path == $path)' > /dev/null; then
  echo -e "  ${GREEN}✓${NC} activity output includes tab-scoped action event"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} activity output missing tab-scoped action event for ${TAB_ID}"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab activity tab <id>"

pt_ok activity tab "$TAB_ID"
assert_output_json "tab activity output is valid JSON"

if echo "$PT_OUT" | jq -e --arg tab "$TAB_ID" 'all(.events[]?; .tabId == $tab)' > /dev/null; then
  echo -e "  ${GREEN}✓${NC} tab activity output is scoped to selected tab"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} tab activity output includes other tabs"
  ((ASSERTIONS_FAILED++)) || true
fi

if echo "$PT_OUT" | jq -e --arg path "/tabs/${TAB_ID}/snapshot" '.events[] | select(.path == $path)' > /dev/null; then
  echo -e "  ${GREEN}✓${NC} tab activity output includes tab-scoped snapshot event"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} tab activity output missing tab-scoped snapshot event"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test
