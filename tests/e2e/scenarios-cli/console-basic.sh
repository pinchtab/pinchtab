#!/bin/bash
# console-basic.sh — CLI console and errors commands.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab console"

pt_ok nav "${FIXTURES_URL}/index.html"
pt_ok console
# Should return "No console logs" or list of logs
assert_output_contains "console\|No console" "returns console output"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab console (with logs)"

pt_ok nav "${FIXTURES_URL}/console.html"
sleep 1
pt_ok console
# Should show logged messages
assert_output_contains "LOG\|log\|console\|Hello\|No console" "shows log level or no logs"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab console --clear"

pt_ok console --clear
assert_output_contains "clear\|Clear" "confirms clear"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab console --limit"

pt_ok nav "${FIXTURES_URL}/console.html"
sleep 1
pt_ok console --limit 2

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab errors"

pt_ok errors
# Should return "No errors" or list of errors
assert_output_contains "error\|Error\|No error" "returns errors output"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab errors --clear"

pt_ok errors --clear
assert_output_contains "clear\|Clear" "confirms clear"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab console --tab <tabId>"

pt_ok nav "${FIXTURES_URL}/index.html"
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')

pt_ok console --tab "$TAB_ID"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab errors --tab <tabId>"

pt_ok errors --tab "$TAB_ID"

end_test
