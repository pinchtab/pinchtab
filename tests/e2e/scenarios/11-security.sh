#!/bin/bash
# 11-security.sh — Security controls verification
#
# Tests that security features properly block/allow requests based on config.
# Uses two pinchtab instances:
#   - E2E_SERVER: permissive (evaluate/download/upload enabled)
#   - E2E_SECURE_SERVER: restrictive (all disabled)

source "$(dirname "$0")/common.sh"

# Helper to target the secure (restrictive) instance
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

# ═══════════════════════════════════════════════════════════════════
# EVALUATE SECURITY
# ═══════════════════════════════════════════════════════════════════

start_test "security: evaluate ALLOWED when enabled"

pt_post /navigate -d '{"url":"about:blank"}'
pt_post /evaluate -d '{"expression":"1+1"}'
assert_ok "evaluate allowed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: evaluate BLOCKED when disabled"

secure_post /navigate -d '{"url":"about:blank"}'
secure_post /evaluate -d '{"expression":"1+1"}'
assert_http_status 403 "evaluate blocked"
assert_contains "$RESULT" "evaluate_disabled" "correct error code"

end_test

# ═══════════════════════════════════════════════════════════════════
# DOWNLOAD SECURITY
# ═══════════════════════════════════════════════════════════════════

start_test "security: download ALLOWED when enabled"

pt_get "/download?url=https://httpbin.org/robots.txt"
assert_ok "download allowed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: download BLOCKED when disabled"

secure_get "/download?url=https://httpbin.org/robots.txt"
assert_http_status 403 "download blocked"
assert_contains "$RESULT" "download_disabled" "correct error code"

end_test

# ═══════════════════════════════════════════════════════════════════
# UPLOAD SECURITY
# ═══════════════════════════════════════════════════════════════════

start_test "security: upload ALLOWED when enabled"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/upload.html\"}"
sleep 1
pt_post /upload -d '{"selector":"#single-file","files":["data:text/plain;base64,dGVzdA=="]}'
assert_ok "upload allowed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: upload BLOCKED when disabled"

# Note: navigation might fail due to IDPI, but upload check happens first
secure_post /upload -d '{"selector":"#single-file","files":["data:text/plain;base64,dGVzdA=="]}'
assert_http_status 403 "upload blocked"
assert_contains "$RESULT" "upload_disabled" "correct error code"

end_test

# ═══════════════════════════════════════════════════════════════════
# IDPI (Navigation Security)
# ═══════════════════════════════════════════════════════════════════

start_test "security: IDPI allows whitelisted domains"

# Main instance has fixtures in allowlist
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate to allowed domain"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: IDPI blocks non-whitelisted domains"

# Secure instance has empty allowlist - should block all navigation
secure_post /navigate -d '{"url":"https://example.com"}'
assert_http_status 403 "navigate blocked by IDPI"
assert_contains "$RESULT" "IDPI\|blocked\|allowed" "IDPI error message"

end_test

# ═══════════════════════════════════════════════════════════════════
# VERIFY ERROR RESPONSE FORMAT
# ═══════════════════════════════════════════════════════════════════

start_test "security: blocked responses include helpful info"

secure_post /evaluate -d '{"expression":"1"}'
assert_http_status 403 "returns 403"
assert_json_exists "$RESULT" ".code" "has error code"
assert_json_exists "$RESULT" ".error" "has error message"

end_test
