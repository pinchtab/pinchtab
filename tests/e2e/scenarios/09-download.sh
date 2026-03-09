#!/bin/bash
# 09-download.sh — File download
#
# NOTE: Download endpoint has SSRF protection that blocks private IPs.
# In Docker, fixtures resolves to internal IP, so we test with public URLs.

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download (public URL)"

# Use a small public file for testing
pt_get "/download?url=https://httpbin.org/robots.txt"
assert_ok "download public"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download (SSRF blocked)"

# Verify internal URLs are blocked (security feature)
pt_get "/download?url=${FIXTURES_URL}/sample.txt"
assert_http_status 400 "download blocked"

# Verify error message mentions blocking
if ! echo "$RESULT" | grep -q "blocked\|private"; then
  fail "expected SSRF block message"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab download --tab <id>"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_get "/tabs/${TAB_ID}/download?url=https://httpbin.org/robots.txt"
assert_ok "tab download"

end_test
