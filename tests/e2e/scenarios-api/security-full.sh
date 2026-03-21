#!/bin/bash
# security-full.sh — API extended security scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

secure_get() {
  local path="$1"
  shift
  local old_url="$E2E_SERVER"
  E2E_SERVER="$E2E_SECURE_SERVER"
  pt_get "$path" "$@"
  E2E_SERVER="$old_url"
}

secure_post() {
  local path="$1"
  shift
  local old_url="$E2E_SERVER"
  E2E_SERVER="$E2E_SECURE_SERVER"
  pt_post "$path" "$@"
  E2E_SERVER="$old_url"
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
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

secure_post /wait -d "{\"tabId\":\"${TAB_ID}\",\"fn\":\"true\",\"timeout\":1000}"
assert_http_status 403 "wait fn blocked"
assert_contains "$RESULT" "evaluate_disabled" "wait fn uses evaluate_disabled guard"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: download BLOCKED when disabled"

secure_get "/download?url=https://httpbin.org/robots.txt"
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

# ─────────────────────────────────────────────────────────────────
start_test "security: blocked responses include helpful info"

secure_post /evaluate -d '{"expression":"1"}'
assert_http_status 403 "returns 403"
assert_json_exists "$RESULT" ".code" "has error code"
assert_json_exists "$RESULT" ".error" "has error message"

end_test
