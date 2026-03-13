#!/bin/bash
# 10-upload.sh — File upload

source "$(dirname "$0")/common.sh"

# Navigate to upload test page
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

# Open upload page in new tab - capture tabId from response
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/upload.html\",\"newTab\":true}"
assert_ok "navigate for upload"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
sleep 1

# 1x1 transparent PNG
PNG_DATA="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

pt_post "/tabs/${TAB_ID}/upload" -d "{\"selector\":\"#single-file\",\"files\":[\"${PNG_DATA}\"]}"
assert_ok "tab upload"

end_test
