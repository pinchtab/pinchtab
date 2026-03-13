#!/bin/bash
# 03-actions.sh — CLI action commands (click, fill, press)

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab fill <selector> <text>"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok fill "#username" "hello world"
assert_output_contains "filled" "confirms fill action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab press <key>"

pt_ok press Tab
# Just verify command succeeds

end_test

# SKIP: click --css, focus --css, hover --css not yet in cobra refactor
# These commands only accept refs, not --css flag
