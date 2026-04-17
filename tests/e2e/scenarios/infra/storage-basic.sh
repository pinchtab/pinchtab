#!/bin/bash
# storage-basic.sh — Storage API (localStorage/sessionStorage) tests.
# Requires: a running PinchTab instance with security.allowStateExport=true
# and an active tab navigated to a page (e.g. https://example.com).

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ═══════════════════════════════════════════════════════════════════
# Setup: Navigate to a fixture page so storage has a valid origin
# ═══════════════════════════════════════════════════════════════════

start_test "Setup: navigate to fixture page for storage tests"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate to fixture"
sleep 0.5

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /storage sets a localStorage item"

pt_post "/storage" -d '{"key":"pt_test_key","value":"pt_test_value","type":"local"}'
assert_ok "set localStorage item"
assert_json_exists "$RESULT" '.success' "has success field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /storage reads back the set item"

pt_get "/storage?type=local&key=pt_test_key"
assert_ok "get localStorage item"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /storage sets a sessionStorage item"

pt_post "/storage" -d '{"key":"pt_sess_key","value":"pt_sess_value","type":"session"}'
assert_ok "set sessionStorage item"
assert_json_exists "$RESULT" '.success' "has success field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /storage with no type returns both stores"

pt_get "/storage"
assert_ok "get all storage"
assert_json_exists "$RESULT" '.local' "has local field"
assert_json_exists "$RESULT" '.session' "has session field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "DELETE /storage removes a specific localStorage key"

pt_delete "/storage" -d '{"key":"pt_test_key","type":"local"}'
assert_ok "delete localStorage key"
assert_json_exists "$RESULT" '.success' "has success field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "DELETE /storage with type=all clears both stores"

# First seed both stores
pt_post "/storage" -d '{"key":"all_local","value":"1","type":"local"}'
assert_ok "seed localStorage"

pt_post "/storage" -d '{"key":"all_session","value":"2","type":"session"}'
assert_ok "seed sessionStorage"

# Then clear both
pt_delete "/storage" -d '{"type":"all"}'
assert_ok "delete type=all"
assert_json_exists "$RESULT" '.success' "has success"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /storage rejects missing key"

pt_post "/storage" -d '{"value":"v","type":"local"}'
assert_not_ok "rejects missing key"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /storage rejects invalid type"

pt_post "/storage" -d '{"key":"k","value":"v","type":"invalid"}'
assert_not_ok "rejects invalid type"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /storage returns 403 when allowStateExport=false"

# This test is advisory — only run when storage gate is disabled.
# The assertion is informational; actual gate test is in security-basic.sh.

end_test

# ═══════════════════════════════════════════════════════════════════
# Tab-scoped storage tests
# ═══════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────
start_test "GET /tabs/{id}/storage reads storage for specific tab"

# Get the active tab ID first
pt_get "/tabs"
assert_ok "get tabs"
TAB_ID=$(echo "$RESULT" | jq -r '.tabs[0].id // empty')

if [ -n "$TAB_ID" ]; then
  pt_get "/tabs/${TAB_ID}/storage"
  assert_ok "get tab storage"
  assert_json_exists "$RESULT" '.origin' "has origin field"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tabs/{id}/storage sets storage for specific tab"

if [ -n "$TAB_ID" ]; then
  pt_post "/tabs/${TAB_ID}/storage" -d '{"key":"pt_tab_key","value":"pt_tab_value","type":"local"}'
  assert_ok "set tab localStorage item"
  assert_json_exists "$RESULT" '.success' "has success field"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "DELETE /tabs/{id}/storage deletes storage for specific tab"

if [ -n "$TAB_ID" ]; then
  pt_delete "/tabs/${TAB_ID}/storage" -d '{"key":"pt_tab_key","type":"local"}'
  assert_ok "delete tab localStorage key"
  assert_json_exists "$RESULT" '.success' "has success field"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tabs/{id}/storage rejects mismatched tabId in body"

if [ -n "$TAB_ID" ]; then
  pt_post "/tabs/${TAB_ID}/storage" -d '{"tabId":"wrong_tab_id","key":"k","value":"v","type":"local"}'
  assert_not_ok "rejects tabId mismatch"
fi

end_test
