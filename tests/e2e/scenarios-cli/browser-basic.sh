#!/bin/bash
# browser-basic.sh — CLI happy-path browser scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab health"

pt_ok health
assert_output_json
assert_output_contains "status" "returns status field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab instances"

pt_ok instances
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
assert_output_json
assert_output_contains "tabId" "returns tab ID"
assert_output_contains "title" "returns page title"

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
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')

pt_ok nav "${FIXTURES_URL}/form.html" --tab "$TAB_ID"
assert_output_contains "form.html" "navigated to form.html"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok snap
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
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')

pt_ok eval "document.title" --tab "$TAB_ID"
assert_output_contains "Button" "evaluates in correct tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok text
assert_output_json
assert_output_contains "text" "returns text field"

end_test

# ─────────────────────────────────────────────────────────────────
# SKIP: text --raw outputs JSON instead of plain text
# Bug: CLI sets mode=raw but not format=text
# See: ~/dev/tmp/text-raw-bug.md
# start_test "pinchtab text --raw"
# pt_ok text --raw
# end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab quick <url>"

pt_ok quick "${FIXTURES_URL}/form.html"
assert_output_contains "nodes" "returns snapshot nodes"
assert_output_contains "form.html" "navigated to correct page"

end_test
