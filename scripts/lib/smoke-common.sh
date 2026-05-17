# shellcheck shell=bash
# Shared primitives for smoke harnesses. Callers must export:
#   HOST_PORT, TOKEN, TMP_DIR, FAILED, API_RESULT, API_STATUS.

skip() {
  echo "SKIP: $*"
  exit 77
}

fail() {
  FAILED=1
  echo "FAIL: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 not found in PATH"
}

choose_free_port() {
  python3 - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
}

api_request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local url="http://127.0.0.1:${HOST_PORT}${path}"
  local -a args=(
    -sS
    -w $'\n%{http_code}'
    -X "$method"
    -H "Authorization: Bearer ${TOKEN}"
  )
  if [ "$method" = "POST" ]; then
    args+=(-H "Content-Type: application/json" -d "$body")
  fi

  local response
  if ! response="$(curl "${args[@]}" "$url" 2>&1)"; then
    fail "$method $path failed: $response"
  fi
  API_STATUS="${response##*$'\n'}"
  API_RESULT="${response%$'\n'*}"
  if [ "$API_STATUS" -lt 200 ] || [ "$API_STATUS" -ge 300 ]; then
    fail "$method $path failed with HTTP $API_STATUS: $API_RESULT"
  fi
}

api_get() {
  api_request GET "$1"
}

api_post() {
  api_request POST "$1" "$2"
}

assert_api_jq() {
  local expr="$1"
  local label="$2"
  echo "$API_RESULT" | jq -e "$expr" >/dev/null || fail "$label failed: $API_RESULT"
}

assert_file_min_bytes() {
  local path="$1"
  local min_bytes="$2"
  local label="$3"
  local bytes
  bytes="$(wc -c < "$path" | tr -d '[:space:]')"
  if [ "${bytes:-0}" -lt "$min_bytes" ]; then
    fail "$label too small: ${bytes:-0} bytes"
  fi
}

assert_screenshot_png() {
  local path="$1"
  local out="$TMP_DIR/screenshot.png"
  local headers="$TMP_DIR/screenshot.headers"
  curl -fsS \
    -D "$headers" \
    -o "$out" \
    -H "Authorization: Bearer ${TOKEN}" \
    "http://127.0.0.1:${HOST_PORT}${path}" || fail "GET $path failed"
  grep -qi '^content-type: image/png' "$headers" || fail "GET $path did not return image/png"
  assert_file_min_bytes "$out" 500 "screenshot"
}
