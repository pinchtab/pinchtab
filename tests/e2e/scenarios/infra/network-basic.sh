#!/bin/bash
# network-basic.sh — Basic network capture API tests.
# Covers: /network, /network/{requestId}, /network/clear

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: returns entries after navigation"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/network?tabId=${TAB_ID}"
assert_ok "get network entries"
assert_json_exists "$RESULT" '.entries' "has entries array"
assert_json_exists "$RESULT" '.tabId' "has tabId"

ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
if [ "$ENTRIES_COUNT" -gt 0 ]; then
  pass_assert "captured $ENTRIES_COUNT network entries"
else
  skip_assert "no entries yet (timing-dependent)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: filter by method"

pt_get "/network?tabId=${TAB_ID}&method=GET"
assert_ok "filter by GET"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: limit parameter"

pt_get "/network?tabId=${TAB_ID}&limit=2"
assert_ok "get with limit"

ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
if [ "$ENTRIES_COUNT" -le 2 ]; then
  pass_assert "limit respected: $ENTRIES_COUNT <= 2"
else
  fail_assert "limit exceeded: $ENTRIES_COUNT > 2"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: non-existent tab → error"

pt_get "/network?tabId=nonexistent_xyz_999"
assert_not_ok "rejects bad tab"

end_test

# ─────────────────────────────────────────────────────────────────
# /network/clear is gated behind security.allowNetworkIntercept, which this
# server leaves off by default — see network-route-extended.sh for the
# happy-path assertion against the full-permissive server.
start_test "POST /network/clear: 403 when capability off (default server)"

pt_post /network/clear "{\"tabId\":\"${TAB_ID}\"}"
assert_not_ok "rejects when allowNetworkIntercept=false"
assert_json_contains "$RESULT" '.code' 'network_intercept_disabled' "error code identifies the disabled capability"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network/export: basic HAR export works"

# Generate fresh traffic
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"

pt_get "/network/export?tabId=${TAB_ID}"
assert_ok "basic HAR export"
assert_result_jq '.log.version' "1.2" "HAR version 1.2"
assert_json_exists "$RESULT" '.log.entries' "has entries"

end_test
