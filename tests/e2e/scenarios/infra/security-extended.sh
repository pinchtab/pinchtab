#!/bin/bash
# security-extended.sh — API extended security scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

secure_get() {
  with_server "$E2E_SECURE_SERVER" pt_get "$@"
}

secure_post() {
  with_server "$E2E_SECURE_SERVER" pt_post "$@"
}

start_test "security: evaluate BLOCKED when disabled"

secure_post /navigate -d '{"url":"about:blank"}'
secure_post /evaluate -d '{"expression":"1+1"}'
assert_http_status 403 "evaluate blocked"
assert_contains "$RESULT" "evaluate_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: wait fn BLOCKED when evaluate disabled"

secure_post /navigate -d '{"url":"http://fixtures:80/index.html"}'
assert_ok "navigate to allowed fixture page"
TAB_ID=$(get_tab_id)

secure_post /wait -d "{\"tabId\":\"${TAB_ID}\",\"fn\":\"true\",\"timeout\":1000}"
assert_http_status 403 "wait fn blocked"
assert_contains "$RESULT" "evaluate_disabled" "wait fn uses evaluate_disabled guard"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: download BLOCKED when disabled"

secure_get "/download?url=${FIXTURES_URL}/sample.txt"
assert_http_status 403 "download blocked"
assert_contains "$RESULT" "download_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: upload BLOCKED when disabled"

secure_post /upload -d '{"selector":"#single-file","files":["data:text/plain;base64,dGVzdA=="]}'
assert_http_status 403 "upload blocked"
assert_contains "$RESULT" "upload_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: IDPI blocks non-whitelisted domains"

secure_post /navigate -d '{"url":"https://example.com"}'
assert_http_status 403 "navigate blocked by IDPI"
assert_contains "$RESULT" "IDPI\|blocked\|allowed" "IDPI error message"

end_test

start_test "security: blocked responses include helpful info"

secure_post /evaluate -d '{"expression":"1"}'
assert_http_status 403 "returns 403"
assert_json_exists "$RESULT" ".code" "has error code"
assert_json_exists "$RESULT" ".error" "has error message"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: stateExport BLOCKED when disabled"

secure_get /state/list
assert_http_status 403 "state list blocked"
assert_contains "$RESULT" "state_export_disabled" "correct error code"

secure_get /storage
assert_http_status 403 "storage get blocked"
assert_contains "$RESULT" "state_export_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: state save/load BLOCKED when disabled"

secure_post /state/save -d '{"name":"test-blocked"}'
assert_http_status 403 "state save blocked"
assert_contains "$RESULT" "state_export_disabled" "correct error code"

secure_post /state/load -d '{"name":"test-blocked"}'
assert_http_status 403 "state load blocked"
assert_contains "$RESULT" "state_export_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: tab-scoped storage BLOCKED when stateExport disabled"

secure_get "/tabs"
TAB_ID=$(echo "$RESULT" | jq -r '.tabs[0].id // empty')

if [ -n "$TAB_ID" ]; then
  secure_get "/tabs/${TAB_ID}/storage"
  assert_http_status 403 "tab storage blocked"
  assert_contains "$RESULT" "state_export_disabled" "correct error code"
fi

end_test
