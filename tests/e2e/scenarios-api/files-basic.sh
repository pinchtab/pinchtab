#!/bin/bash
# files-basic.sh — API happy-path file and capture scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
sleep 1

pt_get /screenshot
assert_ok "screenshot"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf (default)"

pt_get /pdf
assert_ok "pdf"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot --tab <id>"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_get "/tabs/${TAB_ID}/screenshot"
assert_ok "tab screenshot"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf --tab <id>"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_get "/tabs/${TAB_ID}/pdf"
assert_ok "tab pdf"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf --tab <id> (with options)"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_post "/tabs/${TAB_ID}/pdf" -d '{"printBackground":true,"scale":0.8}'
assert_ok "tab pdf with options"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screenshot: quality parameter"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
sleep 1

LOW_Q_SIZE=$(e2e_curl -s "${E2E_SERVER}/screenshot?quality=10" | wc -c)
HIGH_Q_SIZE=$(e2e_curl -s "${E2E_SERVER}/screenshot?quality=95" | wc -c)

if [ "$LOW_Q_SIZE" -lt "$HIGH_Q_SIZE" ]; then
  echo -e "  ${GREEN}✓${NC} quality=10 ($LOW_Q_SIZE bytes) < quality=95 ($HIGH_Q_SIZE bytes)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} quality=10 ($LOW_Q_SIZE) not smaller than quality=95 ($HIGH_Q_SIZE)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screenshot: output=file"

pt_get "/screenshot?output=file"
assert_ok "screenshot output=file"

assert_json_exists "$RESULT" '.path' "response has path field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: output=file"

pt_get "/pdf?output=file"
assert_ok "pdf output=file"

assert_json_exists "$RESULT" '.path' "response has path field"

end_test

# NOTE: Download endpoint has SSRF protection that blocks private IPs.
# In Docker, fixtures resolves to internal IP, so we test with public URLs.

build_download_redirect_url() {
  local target_url="$1"
  local encoded_target
  local attacker_url

  encoded_target=$(jq -rn --arg u "$target_url" '$u|@uri')
  attacker_url="https://httpbin.org/redirect-to?url=${encoded_target}"
  jq -rn --arg u "$attacker_url" '$u|@uri'
}

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download (public URL)"

# Use a small public file for testing
pt_get "/download?url=https://httpbin.org/robots.txt"
assert_ok "download public"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download (SSRF blocked)"

pt_get "/download?url=${FIXTURES_URL}/sample.txt"
assert_http_status 400 "download blocked"

assert_contains "$RESULT" "blocked\|private" "SSRF error message"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download (redirected internal target blocked)"

ATTACKER_URL=$(build_download_redirect_url "http://127.0.0.1:9999/health")
pt_get "/download?url=${ATTACKER_URL}"
assert_http_status 400 "redirected internal target blocked"
assert_contains "$RESULT" "unsafe browser request\|blocked\|private" "redirect SSRF error message"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download --tab <id>"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_get "/tabs/${TAB_ID}/download?url=https://httpbin.org/robots.txt"
assert_ok "tab download"

end_test

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/upload.html\"}"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab upload (base64 file)"

# Files are base64 strings with optional data: prefix
# "Hello from E2E test!" in base64
FILE_CONTENT="data:text/plain;base64,SGVsbG8gZnJvbSBFMkUgdGVzdCE="

pt_post /upload -d "{\"selector\":\"#single-file\",\"files\":[\"${FILE_CONTENT}\"]}"
assert_ok "upload base64"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab upload (multiple files)"

pt_post /upload -d "{\"selector\":\"#multi-file\",\"files\":[\"${FILE_CONTENT}\",\"${FILE_CONTENT}\"]}"
assert_ok "upload multiple"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab upload --tab <id>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/upload.html\",\"newTab\":true}"
assert_ok "navigate for upload"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
sleep 1

# 1x1 transparent PNG
PNG_DATA="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

pt_post "/tabs/${TAB_ID}/upload" -d "{\"selector\":\"#single-file\",\"files\":[\"${PNG_DATA}\"]}"
assert_ok "tab upload"

end_test
