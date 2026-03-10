#!/bin/bash
# 13-tab-variants.sh — Tab-specific upload and download operations

source "$(dirname "$0")/common.sh"

start_test "tab-specific upload: POST /tabs/{id}/upload"

# Navigate to upload page
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/upload.html\"}"
TAB_ID=$(get_tab_id)
show_tab "created" "$TAB_ID"

# Upload file to specific tab
pt_post "/tabs/${TAB_ID}/upload" -d '{"selector":"#single-file","files":["data:text/plain;base64,dGVzdCBmaWxl"]}'
assert_ok "upload to tab"
assert_json_exists "$RESULT" ".uploaded" "upload response has uploaded count"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab-specific upload: multiple files"

pt_post "/tabs/${TAB_ID}/upload" -d '{"selector":"#multi-file","files":["data:text/plain;base64,ZmlsZTE=","data:text/plain;base64,ZmlsZTI="]}'
assert_ok "upload multiple files"
assert_json_contains "$RESULT" ".uploaded" "2" "uploaded 2 files"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab-specific download: GET /tabs/{id}/download"

# Navigate to a page with downloadable content
pt_post /navigate -d '{"url":"https://httpbin.org/robots.txt"}'
TAB_ID2=$(get_tab_id)
show_tab "created" "$TAB_ID2"

# Download from specific tab
pt_get "/tabs/${TAB_ID2}/download?url=https://httpbin.org/json"
assert_ok "download from tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab-specific download: verify content"

# Response should be JSON from httpbin
if echo "$RESULT" | jq -e '.slideshow or .uuid' >/dev/null 2>&1; then
  echo -e "  ${GREEN}✓${NC} download returned httpbin content"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} download content validation failed"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test
