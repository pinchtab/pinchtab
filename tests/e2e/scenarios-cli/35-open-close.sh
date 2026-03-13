#!/bin/bash
# 35-open.sh — CLI open command + aliases

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab open <url>"

pt_ok open "${FIXTURES_URL}/index.html"
assert_output_json
assert_output_contains "tabId" "returns tab ID"
assert_output_contains "title" "returns page title"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab open --block-images <url>"

pt_ok open --block-images "${FIXTURES_URL}/index.html"
assert_output_json
assert_output_contains "tabId" "navigated with images blocked"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab open (no args → error)"

pt_fail open
assert_output_contains "requires" "shows usage error"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab goto <url> (alias for open)"

pt_ok goto "${FIXTURES_URL}/index.html"
assert_output_json
assert_output_contains "tabId" "goto works as alias"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab navigate <url> (alias for open)"

pt_ok navigate "${FIXTURES_URL}/index.html"
assert_output_json
assert_output_contains "tabId" "navigate works as alias"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav <url> (deprecated alias)"

pt_ok nav "${FIXTURES_URL}/index.html"
assert_output_json
assert_output_contains "tabId" "nav still works as deprecated alias"

end_test
