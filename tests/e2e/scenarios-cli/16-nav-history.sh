#!/bin/bash
# 16-nav-history.sh — CLI navigation history commands (back, forward, reload)

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab back (no history)"

pt_ok back
assert_output_json "back returns JSON"
assert_output_contains "tabId" "response contains tabId"
assert_output_contains "url" "response contains url"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab back (navigate two pages then back)"

# Navigate to page A
pt_ok nav "${FIXTURES_URL}/index.html"
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId')
URL_A=$(echo "$PT_OUT" | jq -r '.url')

# Navigate to page B
pt_ok nav "${FIXTURES_URL}/form.html" --tab "$TAB_ID"
URL_B=$(echo "$PT_OUT" | jq -r '.url')

# Go back to page A
pt_ok back --tab "$TAB_ID"
assert_output_json "back returns JSON"
assert_output_contains "index.html" "back returned to index.html"
assert_output_contains "$TAB_ID" "back response contains correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab forward"

pt_ok forward --tab "$TAB_ID"
assert_output_json "forward returns JSON"
assert_output_contains "form.html" "forward returned to form.html"
assert_output_contains "$TAB_ID" "forward response contains correct tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab reload"

pt_ok reload --tab "$TAB_ID"
assert_output_json "reload returns JSON"
assert_output_contains "form.html" "reload kept same page"
assert_output_contains "$TAB_ID" "reload response contains correct tabId"

end_test
