#!/bin/bash
# 01-attach-bridge.sh — Remote bridge attachment through the orchestrator

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../scenarios/common.sh"

BRIDGE_URL="${E2E_BRIDGE_URL:-}"
BRIDGE_TOKEN="${E2E_BRIDGE_TOKEN:-}"
if [ -z "$BRIDGE_URL" ]; then
  echo "  E2E_BRIDGE_URL not set, skipping attach-bridge scenario"
  return 0 2>/dev/null || exit 0
fi

start_test "orchestrator: attach remote bridge and proxy tab traffic"

pt_post /instances/attach-bridge "{\"name\":\"e2e-remote-bridge\",\"baseUrl\":\"${BRIDGE_URL}\",\"token\":\"${BRIDGE_TOKEN}\"}"
assert_http_status "201" "attach bridge"
assert_json_eq "$RESULT" '.attachType' 'bridge' "instance attachType is bridge"
assert_json_eq "$RESULT" '.attached' 'true' "instance is marked attached"
assert_json_eq "$RESULT" '.url' "${BRIDGE_URL}" "instance stores remote bridge URL"

ATTACHED_INST_ID=$(echo "$RESULT" | jq -r '.id // empty')
if [ -n "$ATTACHED_INST_ID" ]; then
  echo -e "  ${GREEN}✓${NC} attached bridge instance id: ${ATTACHED_INST_ID}"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} attach response missing instance id"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post "/instances/${ATTACHED_INST_ID}/tabs/open" "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "open tab on attached bridge"
assert_tab_id "attached bridge returned tabId"
ATTACHED_TAB_ID="${TAB_ID}"

pt_get "/tabs/${ATTACHED_TAB_ID}/text?format=text"
assert_ok "proxy text via attached bridge tab route"
assert_contains "$RESULT" "Welcome to the E2E test fixtures." "tab text came back through orchestrator proxy"

pt_get "/instances/${ATTACHED_INST_ID}/tabs"
assert_ok "list tabs for attached bridge instance"
assert_json_length_gte "$RESULT" '.' '1' "attached bridge has at least one tab"

pt_get /instances/tabs
assert_ok "aggregate tabs includes attached bridge"
if echo "$RESULT" | jq -e --arg inst "$ATTACHED_INST_ID" '.[] | select(.instanceId == $inst)' >/dev/null 2>&1; then
  echo -e "  ${GREEN}✓${NC} aggregate tab list includes attached bridge instance"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} aggregate tab list missing attached bridge instance"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post "/instances/${ATTACHED_INST_ID}/stop" '{}'
assert_ok "stop attached bridge instance"

end_test
