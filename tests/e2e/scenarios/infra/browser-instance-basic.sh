#!/bin/bash
# browser-instance-basic.sh — Instance launch with browser selection.
# Covers: launch instance with browser=chrome, verify instance metadata,
#         navigate/text path succeeds on the launched instance.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

EXPECTED_PROVIDER="${PINCHTAB_E2E_PROVIDER:-chrome}"

# ─────────────────────────────────────────────────────────────────
start_test "instance launch: create instance with browser=chrome"

pt_post "/navigate?browser=chrome" -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate with browser=chrome"
assert_json_contains "$RESULT" '.url' 'index.html' "navigated to index"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "instance launch: navigate then text on same browser"

pt_post "/navigate?browser=chrome" -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons with browser=chrome"

pt_get "/text?browser=chrome"
assert_ok "text with browser=chrome"
assert_contains "$RESULT" "Click me\|Button" "text includes button labels"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "instance launch: snapshot on browser-selected instance"

pt_get "/snapshot?browser=chrome"
assert_ok "snapshot with browser=chrome"
assert_json_length_gte "$RESULT" '.nodes' 1 "snapshot has nodes"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "instance launch: health reports active browser"

pt_get /health
assert_ok "health"
assert_json_eq "$RESULT" '.status' 'ok' "health status ok"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "instance launch: provider-specific navigate + text path"

pt_post "/navigate?browser=${EXPECTED_PROVIDER}" -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
assert_ok "navigate with provider=${EXPECTED_PROVIDER}"

TEXT_RESULT=$(e2e_curl -s "${E2E_SERVER}/text?browser=${EXPECTED_PROVIDER}" | jq -r '.text')
assert_table_page "$TEXT_RESULT"

end_test
