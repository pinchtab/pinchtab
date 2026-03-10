#!/bin/bash
# 15-api-contract.sh — API contract validation (OpenAPI schema)

source "$(dirname "$0")/common.sh"

start_test "api contract: GET /openapi.json returns valid schema"

pt_get /openapi.json
assert_ok "openapi.json"
assert_json_exists "$RESULT" ".openapi" "has openapi version"
assert_json_exists "$RESULT" ".info" "has info block"
assert_json_exists "$RESULT" ".paths" "has paths"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "api contract: openapi.json lists key endpoints"

# Check that documented endpoints include key operations
for endpoint in "/health" "/navigate" "/tabs" "/snapshot" "/action"; do
  if echo "$RESULT" | jq -e ".paths.\"$endpoint\"" >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} endpoint $endpoint documented"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} endpoint $endpoint missing from OpenAPI"
    ((ASSERTIONS_FAILED++)) || true
  fi
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "api contract: GET /help returns documentation"

pt_get /help
assert_ok "help endpoint"
assert_contains "$RESULT" "PinchTab\|navigate\|snapshot" "help text contains expected content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "api contract: endpoints match openapi schema"

# Sanity check: if /navigate is in OpenAPI, it should work
pt_post /navigate -d '{"url":"about:blank"}'
assert_ok "documented /navigate endpoint works"

# If /tabs is documented, it should return a list
pt_get /tabs
assert_ok "documented /tabs endpoint works"
assert_json_exists "$RESULT" ".tabs" "tabs response has expected structure"

end_test
