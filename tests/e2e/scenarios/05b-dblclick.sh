#!/bin/bash
# 05b-dblclick.sh — Double-click action tests

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick by ref"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
REF=$(echo "$RESULT" | jq -r '.nodes[0].ref // .tree.ref // "e0"')

pt_post /action -d "{\"kind\":\"dblclick\",\"ref\":\"$REF\"}"
assert_ok "dblclick by ref"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick by CSS selector"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_post /action -d "{\"kind\":\"dblclick\",\"selector\":\"#increment\"}"
assert_ok "dblclick by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick by coordinates"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_post /action -d "{\"kind\":\"dblclick\",\"x\":100,\"y\":100}"
assert_ok "dblclick by coordinates"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "CLI: pinchtab dblclick <ref>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
REF=$(echo "$RESULT" | jq -r '.nodes[0].ref // .tree.ref // "e0"')

run_cli dblclick "$REF"
assert_ok "CLI dblclick by ref"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "CLI: pinchtab dblclick --css <selector>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

run_cli dblclick --css "#increment"
assert_ok "CLI dblclick by selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "CLI: pinchtab dblclick --tab <id>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\",\"newTab\":true}"
assert_ok "navigate for new tab"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
REF=$(echo "$RESULT" | jq -r '.nodes[0].ref // .tree.ref // "e0"')

run_cli dblclick "$REF" --tab "$TAB_ID"
assert_ok "CLI dblclick with --tab flag"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "HTTP: dblclick validation - missing ref/selector/coordinates"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"

pt_post /action -d "{\"kind\":\"dblclick\"}"
assert_error "dblclick without parameters should fail"

end_test
