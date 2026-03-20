#!/bin/bash
# auth-full.sh — API auth, session, and elevation scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

AUTH_COOKIE_FILE="/tmp/pinchtab-auth-cookie-$$.txt"
AUTH_HEADERS_FILE="/tmp/pinchtab-auth-headers-$$.txt"
AUTH_BODY_FILE="/tmp/pinchtab-auth-body-$$.txt"

trap 'rm -f "$AUTH_COOKIE_FILE" "$AUTH_HEADERS_FILE" "$AUTH_BODY_FILE"' EXIT

auth_reset_session() {
  rm -f "$AUTH_COOKIE_FILE" "$AUTH_HEADERS_FILE" "$AUTH_BODY_FILE"
  : > "$AUTH_COOKIE_FILE"
  : > "$AUTH_HEADERS_FILE"
  : > "$AUTH_BODY_FILE"
}

capture_session_cookie() {
  local cookie_line
  cookie_line=$(grep -i '^Set-Cookie: pinchtab_auth_token=' "$AUTH_HEADERS_FILE" | tail -n 1 | sed 's/\r$//' || true)
  if [[ -z "$cookie_line" ]]; then
    return
  fi

  echo "$cookie_line" | sed -E 's/^Set-Cookie: ([^;]+).*/\1/I' > "$AUTH_COOKIE_FILE"
}

auth_request() {
  local method="$1"
  local path="$2"
  shift 2

  echo -e "${BLUE}→ curl -X $method ${E2E_SERVER}$path $(printf "%q " "$@")${NC}" >&2

  local -a cookie_args=()
  if [[ -s "$AUTH_COOKIE_FILE" ]]; then
    cookie_args=(-H "Cookie: $(cat "$AUTH_COOKIE_FILE")")
  fi

  local response
  response=$(e2e_curl --token "" -s -D "$AUTH_HEADERS_FILE" -w "\n%{http_code}" \
    -X "$method" \
    "${E2E_SERVER}$path" \
    "${cookie_args[@]}" \
    "$@")

  capture_session_cookie

  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)

  if [[ ! "$HTTP_STATUS" =~ ^2 ]]; then
    echo -e "${ERROR}  HTTP $HTTP_STATUS: $RESULT${NC}" >&2
  fi
}

auth_get() {
  local path="$1"
  shift
  auth_request GET "$path" "$@"
  _echo_truncated
}

auth_post_json() {
  local path="$1"
  local body="$2"
  shift 2
  auth_request POST "$path" -H "Content-Type: application/json" -d "$body" "$@"
  _echo_truncated
}

auth_put_json() {
  local path="$1"
  local body="$2"
  shift 2
  auth_request PUT "$path" -H "Content-Type: application/json" -d "$body" "$@"
  _echo_truncated
}

assert_auth_header_contains() {
  local needle="$1"
  local desc="${2:-header contains '$needle'}"
  local headers
  headers=$(cat "$AUTH_HEADERS_FILE" 2>/dev/null || true)
  assert_contains "$headers" "$needle" "$desc"
}

assert_session_cookie_present() {
  local desc="${1:-session cookie persisted}"
  local cookie
  cookie=$(cat "$AUTH_COOKIE_FILE" 2>/dev/null || true)
  assert_contains "$cookie" "pinchtab_auth_token=" "$desc"
}

auth_ws_get() {
  local path="$1"
  shift

  rm -f "$AUTH_HEADERS_FILE" "$AUTH_BODY_FILE"
  : > "$AUTH_HEADERS_FILE"
  : > "$AUTH_BODY_FILE"

  echo -e "${BLUE}→ curl -X GET ${E2E_SERVER}$path [websocket] $(printf "%q " "$@")${NC}" >&2

  local -a cookie_args=()
  if [[ -s "$AUTH_COOKIE_FILE" ]]; then
    cookie_args=(-H "Cookie: $(cat "$AUTH_COOKIE_FILE")")
  fi

  e2e_curl --token "" -s --http1.1 \
    -X GET \
    "${E2E_SERVER}$path" \
    "${cookie_args[@]}" \
    -D "$AUTH_HEADERS_FILE" \
    -o "$AUTH_BODY_FILE" \
    -H "Connection: Upgrade" \
    -H "Upgrade: websocket" \
    -H "Sec-WebSocket-Version: 13" \
    -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
    --max-time 2 \
    "$@" >/dev/null 2>&1 || true

  capture_session_cookie

  RESULT=$(cat "$AUTH_BODY_FILE" 2>/dev/null || true)
  HTTP_STATUS=$(grep '^HTTP/' "$AUTH_HEADERS_FILE" | tail -n 1 | awk '{print $2}')

  if [[ "$HTTP_STATUS" != "101" && ! "$HTTP_STATUS" =~ ^2 && -n "$RESULT" ]]; then
    echo -e "${ERROR}  HTTP $HTTP_STATUS: $RESULT${NC}" >&2
  fi
}

start_test "auth: query token is rejected"

auth_reset_session
auth_get "/health?token=${E2E_SERVER_TOKEN}"
assert_http_status 401 "query token auth rejected"
assert_contains "$RESULT" "missing_token" "query token ignored"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: login sets session cookie"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
assert_json_value "$RESULT" ".status" "ok" "login status ok"
assert_auth_header_contains "Set-Cookie: pinchtab_auth_token=" "session cookie set"
assert_session_cookie_present "session cookie stored in cookie jar"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: session cookie can access allowed dashboard route"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_get /api/config -H "Origin: ${E2E_SERVER}"
assert_http_status 200 "cookie auth can access config"
assert_json_exists "$RESULT" ".config" "config payload returned"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: cookie auth rejects cross-origin requests"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_get /api/config -H "Origin: http://evil.example"
assert_http_status 403 "cross-origin cookie request blocked"
assert_contains "$RESULT" "origin_forbidden" "origin error code returned"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: cookie auth allows same-origin requests"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_get /api/config -H "Origin: ${E2E_SERVER}"
assert_http_status 200 "same-origin cookie request allowed"
assert_json_exists "$RESULT" ".config" "config payload returned"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: sensitive dashboard config update requires elevation"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_put_json /api/config '{"server":{"token":"forbidden-from-dashboard"}}' -H "Origin: ${E2E_SERVER}"
assert_http_status 403 "config update requires elevation"
assert_contains "$RESULT" "elevation_required" "elevation error code returned"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: elevation unlocks sensitive dashboard config update"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_post_json /api/auth/elevate "{\"token\":\"${E2E_SERVER_TOKEN}\"}" -H "Origin: ${E2E_SERVER}"
assert_http_status 200 "session elevated"
assert_json_exists "$RESULT" ".elevationWindowSec" "elevation window returned"
auth_put_json /api/config '{"server":{"token":"forbidden-from-dashboard"}}' -H "Origin: ${E2E_SERVER}"
assert_http_status 400 "elevated request reaches config handler"
assert_contains "$RESULT" "token_write_only" "write-only token guard returned"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: header auth bypasses dashboard elevation flow for config updates"

pt PUT /api/config -d '{"server":{"token":"forbidden-from-dashboard"}}'
assert_http_status 400 "header auth reaches config handler directly"
assert_contains "$RESULT" "token_write_only" "write-only token guard returned"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: cookie session cannot use download endpoint"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_get "/download?url=https://httpbin.org/robots.txt" -H "Origin: ${E2E_SERVER}"
assert_http_status 403 "cookie session blocked on download endpoint"
assert_contains "$RESULT" "header_auth_required" "download requires authorization header"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: cookie session cannot use tab download endpoint"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
DOWNLOAD_TAB_ID=$(get_tab_id)
assert_ok "navigate for tab download auth boundary"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_get "/tabs/${DOWNLOAD_TAB_ID}/download?url=https://httpbin.org/robots.txt" -H "Origin: ${E2E_SERVER}"
assert_http_status 403 "cookie session blocked on download endpoint"
assert_contains "$RESULT" "header_auth_required" "download requires authorization header"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: screencast websocket rejects bad origin for cookie session"

OLD_SERVER="$E2E_SERVER"
E2E_SERVER="${E2E_FULL_SERVER:-$E2E_SERVER}"

pt_get /instances
SCREENCAST_INST_ID=$(echo "$RESULT" | jq -r '.[0].id')
assert_ok "list instances for screencast"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
SCREENCAST_TAB_ID=$(get_tab_id)
assert_ok "navigate for screencast auth boundary"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_ws_get "/instances/${SCREENCAST_INST_ID}/proxy/screencast?tabId=${SCREENCAST_TAB_ID}" -H "Origin: http://evil.example"
assert_http_status 403 "bad origin websocket upgrade blocked"
assert_contains "$RESULT" "origin_forbidden" "websocket origin error returned"

E2E_SERVER="$OLD_SERVER"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: screencast websocket allows same-origin cookie session"

OLD_SERVER="$E2E_SERVER"
E2E_SERVER="${E2E_FULL_SERVER:-$E2E_SERVER}"

pt_get /instances
SCREENCAST_INST_ID=$(echo "$RESULT" | jq -r '.[0].id')
assert_ok "list instances for same-origin screencast"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
SCREENCAST_TAB_ID=$(get_tab_id)
assert_ok "navigate for same-origin screencast"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_ws_get "/instances/${SCREENCAST_INST_ID}/proxy/screencast?tabId=${SCREENCAST_TAB_ID}" -H "Origin: ${E2E_SERVER}"
assert_http_status 101 "same-origin websocket upgrade allowed"
assert_auth_header_contains "101 Switching Protocols" "websocket handshake completed"

E2E_SERVER="$OLD_SERVER"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: logout revokes the session cookie"

auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}"
assert_http_status 200 "login succeeds"
auth_request POST /api/auth/logout
assert_http_status 200 "logout succeeds"
assert_json_value "$RESULT" ".status" "ok" "logout status ok"
auth_get /api/config
assert_http_status 401 "logged out session cannot access config"
assert_contains "$RESULT" "missing_token" "session cookie no longer accepted"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: forwarded proxy headers ignored by default"

# Login with real origin to get a valid session cookie
auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}" -H "Origin: ${E2E_SERVER}"
assert_http_status 200 "login succeeds with real origin"
assert_session_cookie_present "session cookie set"

# Session works with real origin
auth_get /health -H "Origin: ${E2E_SERVER}"
assert_http_status 200 "health ok with real origin"

# Forwarded headers should NOT override — request still uses real host
auth_get /health -H "Origin: https://proxy.example.com" -H "X-Forwarded-Proto: https" -H "X-Forwarded-Host: proxy.example.com"
assert_http_status 403 "forwarded headers ignored when trustProxyHeaders is off"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auth: forwarded proxy headers trusted when enabled"

# Enable trustProxyHeaders via bearer token (bypasses origin checks)
RESULT=$(e2e_curl -s -w "\n%{http_code}" -X PUT "${E2E_SERVER}/api/config" \
  -H "Content-Type: application/json" \
  -d '{"server":{"trustProxyHeaders":true}}')
HTTP_STATUS=$(echo "$RESULT" | tail -n 1)
RESULT=$(echo "$RESULT" | head -n -1)
assert_http_status 200 "enable trustProxyHeaders"

# Now login with forwarded origin
auth_reset_session
auth_post_json /api/auth/login "{\"token\":\"${E2E_SERVER_TOKEN}\"}" \
  -H "Origin: https://proxy.example.com" \
  -H "X-Forwarded-Proto: https" \
  -H "X-Forwarded-Host: proxy.example.com"
assert_http_status 200 "login succeeds with forwarded origin"
assert_session_cookie_present "session cookie set via proxy"

# Session requests work with forwarded headers
auth_get /health \
  -H "Origin: https://proxy.example.com" \
  -H "X-Forwarded-Proto: https" \
  -H "X-Forwarded-Host: proxy.example.com"
assert_http_status 200 "health ok with forwarded proxy headers"

# Disable trustProxyHeaders again
RESULT=$(e2e_curl -s -w "\n%{http_code}" -X PUT "${E2E_SERVER}/api/config" \
  -H "Content-Type: application/json" \
  -d '{"server":{"trustProxyHeaders":false}}')
HTTP_STATUS=$(echo "$RESULT" | tail -n 1)
RESULT=$(echo "$RESULT" | head -n -1)
assert_http_status 200 "disable trustProxyHeaders"

end_test

# ─────────────────────────────────────────────────────────────────
# Keep this rate-limit scenario last in the file.
#
# It intentionally exhausts the shared login-attempt bucket for the runner's
# client IP. auth_reset_session only clears cookies and headers; it does not
# reset the server-side login limiter. Moving this block earlier will cause
# subsequent auth/login scenarios from the same runner container to fail with
# 429 before they exercise their intended assertions.
start_test "auth: login attempts are rate limited"

auth_reset_session

for attempt in $(seq 1 10); do
  auth_post_json /api/auth/login '{"token":"wrong-token"}'
  assert_http_status 401 "bad login attempt ${attempt}"
  assert_contains "$RESULT" "bad_token" "bad token rejected on attempt ${attempt}"
done

auth_post_json /api/auth/login '{"token":"wrong-token"}'
assert_http_status 429 "rate limit triggers after repeated failures"
assert_contains "$RESULT" "login_rate_limited" "rate limit error code returned"
assert_json_exists "$RESULT" ".details.retryAfterSec" "retry-after details returned"
assert_auth_header_contains "Retry-After: " "retry-after header returned"

end_test
