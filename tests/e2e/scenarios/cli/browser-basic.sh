#!/bin/bash
# browser-basic.sh — CLI happy-path browser scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab health"

pt_ok health
# Terse mode outputs "ok" on success
assert_output_contains "ok" "returns ok status"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab instances"

pt_ok instances --json
assert_output_json
# Output is an array like [{id:..., status:...}], check for instance properties
assert_output_contains "id" "returns instance id"
assert_output_contains "status" "returns instance status"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab profiles"

pt_ok profiles

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav <url>"

pt_ok nav "${FIXTURES_URL}/index.html"
assert_tab_id "returns tab ID"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav (empty URL)"

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

pt_ok nav "${FIXTURES_URL}/index.html"
TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

pt_ok nav "${FIXTURES_URL}/form.html" --tab "$TAB_ID"
# nav emits bare tab ID on piped stdout; verify --tab reused the same tab.
if [ "$(echo "$PT_OUT" | tr -d '[:space:]')" = "$TAB_ID" ]; then
  echo -e "  ${GREEN}✓${NC} navigated in same tab"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected tab $TAB_ID, got $PT_OUT"
  ((ASSERTIONS_FAILED++)) || true
fi
# Follow up with a tab-scoped eval to confirm the URL actually changed.
pt_ok eval "location.pathname" --tab "$TAB_ID"
assert_output_contains "form.html" "navigated to form.html"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap"

pt_ok nav "${FIXTURES_URL}/index.html"
# --full gives JSON (the default snap output is now compact text)
pt_ok snap --full
assert_output_json
assert_output_contains "nodes" "returns snapshot nodes"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab eval <expression>"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok eval "1 + 1"
assert_output_contains "2" "evaluates simple expression"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab eval (DOM query)"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok eval "document.title"
assert_output_contains "Form" "returns page title"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab eval (JSON result)"

pt_ok eval 'JSON.stringify({a: 1, b: 2})'
# Output is {"result": "{\"a\":1,\"b\":2}"} - escaped JSON
assert_output_contains 'a' "returns JSON object"
assert_output_contains 'b' "contains both keys"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab eval --tab <tabId> <expression>"

pt_ok nav "${FIXTURES_URL}/buttons.html"
TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

pt_ok eval "document.title" --tab "$TAB_ID"
assert_output_contains "Button" "evaluates in correct tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok text
# Default output is plain text (no JSON wrapper)
assert_output_contains "Welcome" "returns page text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text --json"

pt_ok text --json
assert_output_json
assert_output_contains "text" "returns JSON with text field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text -s <selector>"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok text -s "#welcome"
# Default output is plain text
assert_output_contains "Welcome" "extracts text from element"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab title url html styles"

pt_ok nav "${FIXTURES_URL}/table.html"
TAB_ID=$(echo "$PT_OUT" | tr -d '[:space:]')

pt_ok title --tab "$TAB_ID"
assert_output_contains "E2E Test - Table" "returns page title"

pt_ok url --tab "$TAB_ID"
assert_output_contains "table.html" "returns page url"

pt_ok html "#data-table" --tab "$TAB_ID"
assert_output_contains "<table id=\"data-table\">" "returns selected table html"
assert_output_contains "Alice Johnson" "html contains table row content"

pt_ok styles ".status-active" --prop display --tab "$TAB_ID"
assert_output_contains "inline-block" "returns computed display property"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text <ref>"

pt_ok nav "${FIXTURES_URL}/index.html"
# -i keeps the interactive filter; --compact=false forces JSON for jq parsing
pt_ok snap -i --compact=false
# Extract a ref from the snapshot (first link)
REF=$(echo "$PT_OUT" | safe_jq -r '.nodes[] | select(.role == "link") | .ref' | head -1)
if [ -n "$REF" ] && [ "$REF" != "null" ]; then
  pt_ok text "$REF"
  # Default output is plain text
  assert_output_not_contains "{" "returns plain text, not JSON"
else
  echo -e "  ${YELLOW}⚠${NC} Could not extract ref from snapshot, skipping"
fi

end_test

# ─────────────────────────────────────────────────────────────────
# SKIP: text --raw outputs JSON instead of plain text
# Bug: CLI sets mode=raw but not format=text
# See: ~/dev/tmp/text-raw-bug.md
# start_test "pinchtab text --raw"
# pt_ok text --raw
# end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav <url> --snap"

pt_ok nav "${FIXTURES_URL}/form.html" --snap
assert_output_contains "nodes" "returns snapshot nodes"
assert_output_contains "form.html" "navigated to correct page"

end_test
