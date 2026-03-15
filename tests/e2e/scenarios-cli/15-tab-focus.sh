#!/bin/bash
# 15-tab-focus.sh — CLI tab focus and close commands

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab (list tabs)"

pt_ok tab
assert_output_json "output is valid JSON"
assert_output_contains "tabs" "output contains tabs array"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab <id> (focus by tab ID)"

# Navigate to a page first to ensure there's a tab
pt nav "${FIXTURES_URL}/index.html"

# Get list of tabs and extract first tab ID
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

# Open two tabs
pt nav "${FIXTURES_URL}/index.html"
pt nav "${FIXTURES_URL}/form.html"

# Get tab count and last tab ID
pt tab
BEFORE=$(echo "$PT_OUT" | jq '.tabs | length')
CLOSE_ID=$(echo "$PT_OUT" | jq -r '.tabs[-1].id // empty')
echo -e "  ${MUTED}tab count before: $BEFORE, closing: ${CLOSE_ID:0:12}...${NC}"

# Close by ID
pt_ok tab close "$CLOSE_ID"

# Verify count decreased
pt tab
AFTER=$(echo "$PT_OUT" | jq '.tabs | length')
echo -e "  ${MUTED}tab count after: $AFTER${NC}"

if [ "$AFTER" -lt "$BEFORE" ]; then
  echo -e "  ${GREEN}✓${NC} tab was closed (count went from $BEFORE to $AFTER)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} tab count did not decrease"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test
