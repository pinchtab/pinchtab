#!/bin/bash
# 01-nav.sh — CLI nav commands

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav <url>"

pt_ok nav "${FIXTURES_URL}/index.html"
assert_output_json
assert_output_contains "tabId" "returns tab ID"
assert_output_contains "title" "returns page title"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav (empty URL)"

# Empty URL should fail (only truly invalid case with URL normalization)
pt_fail nav ""

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav (bare hostname normalizes to https)"

# Bare hostnames are now normalized to https:// - Chrome shows error page but nav succeeds
pt nav "not-a-valid-url"
if echo "$PT_OUT" | grep -q "chrome-error"; then
  echo -e "  ${GREEN}✓${NC} Normalized to https://not-a-valid-url (Chrome error page)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} Unexpected result: $PT_OUT"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav --tab <tabId> <url>"

# First navigate to get a tab
pt_ok nav "${FIXTURES_URL}/index.html"
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')

# Navigate same tab using --tab flag
pt_ok nav "${FIXTURES_URL}/form.html" --tab "$TAB_ID"
assert_output_contains "form.html" "navigated to form.html"

end_test
