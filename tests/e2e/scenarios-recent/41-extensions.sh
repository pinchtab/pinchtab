#!/bin/bash
# Test: Browser extension loading
# Verifies that extension paths reach Chrome startup reliably in CI.
source "$(dirname "$0")/common.sh"

ORCH_URL=$E2E_SERVER
ORIG_URL=$E2E_SERVER

assert_instance_logs_poll() {
  local inst_id="$1"
  local needle="$2"
  local desc="$3"
  local attempts="${4:-10}"
  local delay="${5:-1}"

  local i
  for i in $(seq 1 "$attempts"); do
    E2E_SERVER=$ORCH_URL pt_get "/instances/${inst_id}/logs" >/dev/null
    if [[ "$HTTP_STATUS" =~ ^2 ]] && echo "$RESULT" | grep -Fq "$needle"; then
      echo -e "  ${GREEN}✓${NC} $desc"
      ((ASSERTIONS_PASSED++)) || true
      return 0
    fi
    sleep "$delay"
  done

  echo -e "  ${RED}✗${NC} $desc (missing: $needle)"
  ((ASSERTIONS_FAILED++)) || true
  return 1
}

print_extension_hints() {
  local inst_id="${1:-}"
  echo ""
  echo "  ${YELLOW}${BOLD}🔍 Troubleshooting Extension Failure:${NC}"
  echo "  - Check if /extensions/test-extension exists and is readable in the pinchtab container."
  echo "  - Check Manifest V3 host_permissions matches: [\"*://*/*\"]"
  if [ -n "$inst_id" ]; then
    E2E_SERVER=$ORCH_URL pt_get "/instances/${inst_id}/logs" >/dev/null
    echo "  - Recent instance log tail:"
    printf '%s\n' "$RESULT" | tail -n 12 | sed 's/^/    /'
  fi
  echo ""
}

# --- T1: Default instance loads configured extension path ---
start_test "Extension config: default instance loads configured extension path"

pt_get /instances
assert_ok "list instances"
DEFAULT_INST_ID=$(echo "$RESULT" | jq -r '.[] | select(.profileName == "default") | .id' | head -n 1)
if [ -n "$DEFAULT_INST_ID" ] && [ "$DEFAULT_INST_ID" != "null" ]; then
  echo -e "  ${GREEN}✓${NC} default instance present"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} default instance present"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

DEFAULT_LOG_PASS=1
if [ -n "$DEFAULT_INST_ID" ] && [ "$DEFAULT_INST_ID" != "null" ]; then
  assert_instance_logs_poll \
    "$DEFAULT_INST_ID" \
    "loading extensions paths=/extensions/test-extension" \
    "default instance logs configured extension path"
  DEFAULT_LOG_PASS=$?

  assert_instance_logs_poll \
    "$DEFAULT_INST_ID" \
    "chrome initialized successfully" \
    "default instance chrome initialized"
fi

if [ $DEFAULT_LOG_PASS -ne 0 ]; then
  print_extension_hints "$DEFAULT_INST_ID"
  # Debug: dump full instance logs for analysis
  echo "  ${YELLOW}📋 Full instance logs:${NC}"
  E2E_SERVER=$ORCH_URL pt_get "/instances/${DEFAULT_INST_ID}/logs" >/dev/null
  printf '%s\n' "$RESULT" | head -30 | sed 's/^/    /'
fi

end_test

# --- T2: Instance start with extension via API ---
start_test "Extension config: instance start accepts extensionPaths"

pt_post /instances/start '{"extensionPaths":["/extensions/test-extension"]}'
assert_ok "instance start with extension"
INST_ID=$(echo "$RESULT" | jq -r '.id')
INST_PORT=$(echo "$RESULT" | jq -r '.port')
E2E_SERVER="http://pinchtab:${INST_PORT}"
wait_for_instance_ready "${E2E_SERVER}"
E2E_SERVER=$ORIG_URL

assert_instance_logs_poll \
  "$INST_ID" \
  "loading extensions paths=/extensions/test-extension" \
  "API-started instance logs extension path"

end_test

# --- T3: Global + API injected extensions (Additive) ---
start_test "Additive extensions: global + API paths merged"

# Start an instance that adds a SECOND extension (it should still have the global one)
pt_post /instances/start '{"extensionPaths":["/extensions/test-extension-api"]}'
assert_ok "instance start"
INST_ID=$(echo "$RESULT" | jq -r '.id')
INST_PORT=$(echo "$RESULT" | jq -r '.port')

# Switch E2E_SERVER to the new instance's port for direct testing
E2E_SERVER="http://pinchtab:${INST_PORT}"

# Wait for this child instance to be ready
wait_for_instance_ready "${E2E_SERVER}"

# Navigate
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
assert_instance_logs_poll \
  "$INST_ID" \
  "loading extensions paths=/extensions/test-extension,/extensions/test-extension-api" \
  "child instance logs merged extension paths"
MERGE_PASS=$?

assert_instance_logs_poll \
  "$INST_ID" \
  "chrome initialized successfully" \
  "child instance chrome initialized"

if [ $MERGE_PASS -ne 0 ]; then
  print_extension_hints "$INST_ID"
fi

# Restore original URL
E2E_SERVER=$ORIG_URL

end_test
