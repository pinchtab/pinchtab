#!/bin/bash
# tabs-basic.sh — CLI happy-path tab scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab (list)"

pt_ok nav "${FIXTURES_URL}/index.html"

pt_ok tab --json
assert_output_json
assert_output_contains "tabs" "returns tabs array"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab close <id>"

pt_ok nav "${FIXTURES_URL}/form.html"
TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

pt_ok tab close "$TAB_ID"

pt_ok tab
assert_output_not_contains "$TAB_ID" "tab was closed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav --new-tab <url>"

pt_ok nav "${FIXTURES_URL}/index.html" --new-tab
BASE_TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

pt_ok nav "${FIXTURES_URL}/buttons.html" --new-tab --json
assert_output_json
assert_output_contains "tabId" "returns new tab ID"
NEW_TAB_ID=$(get_tab_id "$PT_OUT")

if [ "$NEW_TAB_ID" != "$BASE_TAB_ID" ]; then
  pass_assert "--new-tab returned a different tab"
else
  fail_assert "--new-tab reused ${BASE_TAB_ID}"
fi

pt_ok tab
assert_output_contains "$NEW_TAB_ID" "new tab appears in list"

if [ -n "$NEW_TAB_ID" ]; then
  pt tabs close "$NEW_TAB_ID" > /dev/null 2>&1 || true
fi
if [ -n "$BASE_TAB_ID" ]; then
  pt tabs close "$BASE_TAB_ID" > /dev/null 2>&1 || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav reuses current tracked tab"

pt_ok nav "${FIXTURES_URL}/index.html" --new-tab
TRACKED_TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

pt_ok nav "${FIXTURES_URL}/form.html"
REUSED_TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

if [ "$REUSED_TAB_ID" = "$TRACKED_TAB_ID" ]; then
  pass_assert "plain nav reused current tab"
else
  fail_assert "expected $TRACKED_TAB_ID, got $REUSED_TAB_ID"
fi

pt_ok url --tab "$TRACKED_TAB_ID"
assert_output_contains "form.html" "tracked tab navigated to second URL"

pt tabs close "$TRACKED_TAB_ID" > /dev/null 2>&1 || true

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav creates after current tab closes"

pt_ok nav "${FIXTURES_URL}/index.html" --new-tab
CLOSED_TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

pt_ok tab close "$CLOSED_TAB_ID"

pt_ok nav "${FIXTURES_URL}/form.html"
NEW_AFTER_CLOSE_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

if [ "$NEW_AFTER_CLOSE_ID" != "$CLOSED_TAB_ID" ]; then
  pass_assert "nav created a fresh tab after close"
else
  fail_assert "nav reused closed tab ${CLOSED_TAB_ID}"
fi

pt tabs close "$NEW_AFTER_CLOSE_ID" > /dev/null 2>&1 || true

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab (list tabs)"

pt_ok tab --json
assert_output_json "output is valid JSON"
assert_output_contains "tabs" "output contains tabs array"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab <id> (focus by tab ID)"

pt_ok nav "${FIXTURES_URL}/index.html" --new-tab
FOCUS_TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

pt_ok nav "${FIXTURES_URL}/buttons.html" --new-tab
OTHER_TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

if [ -n "$FOCUS_TAB_ID" ]; then
  echo -e "  ${BLUE}→ focusing on tab ID: ${FOCUS_TAB_ID:0:12}...${NC}"
  pt_ok tab "$FOCUS_TAB_ID"
  assert_output_contains "$FOCUS_TAB_ID" "output contains focused tab ID"

  pt_ok nav "${FIXTURES_URL}/form.html"
  REUSED_FOCUS_ID=$(echo "$PT_OUT" | tr -d '[:space:]')
  if [ "$REUSED_FOCUS_ID" = "$FOCUS_TAB_ID" ]; then
    pass_assert "nav reused focused tab"
  else
    fail_assert "expected focused tab $FOCUS_TAB_ID, got $REUSED_FOCUS_ID"
  fi
else
  skip_assert "could not extract tab ID, skipping"
fi

pt tabs close "$FOCUS_TAB_ID" > /dev/null 2>&1 || true
pt tabs close "$OTHER_TAB_ID" > /dev/null 2>&1 || true

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tab close <id> (close by tab ID)"

pt nav "${FIXTURES_URL}/form.html"
CLOSE_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

if [ -n "$CLOSE_ID" ]; then
  echo -e "  ${MUTED}closing tab: ${CLOSE_ID:0:12}...${NC}"
  pt_ok tab close "$CLOSE_ID"
  assert_output_contains "OK" "output confirms tab was closed"
else
  skip_assert "could not get tab ID from navigate, skipping"
fi

end_test
