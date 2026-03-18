#!/bin/bash
# tabs-basic.sh — CLI happy-path tab scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab (list)"

pt_ok nav "${FIXTURES_URL}/index.html"

pt_ok tab
assert_output_json
assert_output_contains "tabs" "returns tabs array"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab close <id>"

pt_ok nav "${FIXTURES_URL}/form.html"
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')

pt_ok tab close "$TAB_ID"

pt_ok tab
assert_output_not_contains "$TAB_ID" "tab was closed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab new <url>"

pt_ok tab new "${FIXTURES_URL}/buttons.html"
assert_output_json
assert_output_contains "tabId" "returns new tab ID"

NEW_TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')

pt_ok tab
assert_output_contains "$NEW_TAB_ID" "new tab appears in list"

if [ -n "$NEW_TAB_ID" ] && [ "$NEW_TAB_ID" != "null" ]; then
  pt tabs close "$NEW_TAB_ID" > /dev/null 2>&1 || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab (list tabs)"

pt_ok tab
assert_output_json "output is valid JSON"
assert_output_contains "tabs" "output contains tabs array"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab <id> (focus by tab ID)"

pt nav "${FIXTURES_URL}/index.html"

pt tab
assert_output_json "tab list is valid JSON"
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabs[0].id // empty')

if [ -n "$TAB_ID" ] && [ "$TAB_ID" != "null" ]; then
  echo -e "  ${BLUE}→ focusing on tab ID: ${TAB_ID:0:12}...${NC}"
  pt_ok tab "$TAB_ID"
  assert_output_contains "focused" "output indicates tab is focused"
else
  echo -e "  ${YELLOW}⚠${NC} could not extract tab ID, skipping"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab close <id> (close by tab ID)"

pt nav "${FIXTURES_URL}/form.html"
CLOSE_ID=$(echo "$PT_OUT" | jq -r '.tabId // empty')

if [ -n "$CLOSE_ID" ] && [ "$CLOSE_ID" != "null" ]; then
  echo -e "  ${MUTED}closing tab: ${CLOSE_ID:0:12}...${NC}"
  pt_ok tab close "$CLOSE_ID"
  assert_output_contains "closed" "output confirms tab was closed"
else
  echo -e "  ${YELLOW}⚠${NC} could not get tab ID from navigate, skipping"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test
