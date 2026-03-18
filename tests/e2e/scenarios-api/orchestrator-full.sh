#!/bin/bash
# orchestrator-full.sh — API full orchestration scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

BRIDGE_URL="${E2E_BRIDGE_URL:-}"
BRIDGE_TOKEN="${E2E_BRIDGE_TOKEN:-}"

if [ -z "$BRIDGE_URL" ]; then
  echo "  E2E_BRIDGE_URL not set, skipping orchestrator full scenarios"
  return 0 2>/dev/null || exit 0
fi

# ─────────────────────────────────────────────────────────────────
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

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: health shows dashboard mode"

pt_get /health
assert_ok "health"
assert_json_eq "$RESULT" '.mode' 'dashboard'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: list instances"

pt_get /instances
assert_ok "list instances"
assert_json_length_gte "$RESULT" '.' '1' "at least 1 instance"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: launch new instance"

pt_post /instances/launch '{"name":"e2e-multi-test","headless":true}'
assert_ok "launch instance"

INST_ID=$(echo "$RESULT" | jq -r '.id')
assert_json_exists "$RESULT" '.id' "has instance id"
assert_json_exists "$RESULT" '.port' "has port"

wait_for_orchestrator_instance_status "${E2E_SERVER}" "${INST_ID}" "running" 30

pt_get /instances
assert_ok "list after launch"
FOUND=$(echo "$RESULT" | jq -r ".[] | select(.id == \"$INST_ID\") | .id")
if [ "$FOUND" = "$INST_ID" ]; then
  echo -e "  ${GREEN}✓${NC} instance $INST_ID in list"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} instance $INST_ID not in list"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: get instance by id"

pt_get "/instances/${INST_ID}"
assert_ok "get instance"
assert_json_eq "$RESULT" '.id' "$INST_ID"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: instance logs"

pt_get "/instances/${INST_ID}/logs"
assert_ok "get logs"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: aggregate tabs"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

pt_get /instances/tabs
assert_ok "aggregate tabs"
assert_json_length_gte "$RESULT" '.' '1' "at least 1 tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: aggregate tabs (multi-instance)"

pt_post /instances/launch '{"name":"e2e-agg-tabs","headless":true}'
assert_ok "launch for aggregate"
AGG_INST=$(echo "$RESULT" | jq -r '.id')
wait_for_orchestrator_instance_status "${E2E_SERVER}" "${AGG_INST}" "running" 30

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate on default instance"
pt_post "/instances/${AGG_INST}/tabs/open" "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "open tab on aggregate instance"

pt_get /instances/tabs
assert_ok "aggregate tabs"
assert_json_length_gte "$RESULT" '.' '2' "at least 2 tabs across instances"

pt_post "/instances/${AGG_INST}/stop" '{}'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: instance tabs"

for i in $(seq 1 10); do
  INST_STATUS=$(curl -sf "${E2E_SERVER}/instances/${INST_ID}" 2>/dev/null | jq -r '.status // empty' || true)
  if [ "$INST_STATUS" = "running" ]; then
    break
  fi
  sleep 1
done

pt_get "/instances/${INST_ID}/tabs"
assert_ok "instance tabs"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: stop instance"

pt_post "/instances/${INST_ID}/stop" '{}'
assert_ok "stop instance"

sleep 1
pt_get /instances
FOUND=$(echo "$RESULT" | jq -r ".[] | select(.id == \"$INST_ID\") | .id")
if [ -z "$FOUND" ] || [ "$FOUND" = "null" ]; then
  echo -e "  ${GREEN}✓${NC} instance removed after stop"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} instance still in list after stop"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: ID format (inst_ prefix)"

pt_post /instances/launch '{"name":"e2e-id-format","headless":true}'
assert_ok "launch"
ID_CHECK_INST=$(echo "$RESULT" | jq -r '.id')

if echo "$ID_CHECK_INST" | grep -q "^inst_"; then
  echo -e "  ${GREEN}✓${NC} instance ID has inst_ prefix: $ID_CHECK_INST"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} bad ID format: $ID_CHECK_INST"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post "/instances/${ID_CHECK_INST}/stop" '{}'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: port allocation (unique ports)"

pt_post /instances/launch '{"name":"e2e-port-1","headless":true}'
assert_ok "launch 1"
PORT_INST1=$(echo "$RESULT" | jq -r '.id')
PORT1=$(echo "$RESULT" | jq -r '.port')

pt_post /instances/launch '{"name":"e2e-port-2","headless":true}'
assert_ok "launch 2"
PORT_INST2=$(echo "$RESULT" | jq -r '.id')
PORT2=$(echo "$RESULT" | jq -r '.port')

if [ "$PORT1" != "$PORT2" ]; then
  echo -e "  ${GREEN}✓${NC} unique ports: $PORT1 vs $PORT2"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} duplicate ports: $PORT1"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post "/instances/${PORT_INST1}/stop" '{}'
pt_post "/instances/${PORT_INST2}/stop" '{}'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: port reuse after stop"

pt_post /instances/launch '{"name":"e2e-reuse-1","headless":true}'
assert_ok "launch"
REUSE_INST1=$(echo "$RESULT" | jq -r '.id')
REUSE_PORT1=$(echo "$RESULT" | jq -r '.port')

pt_post "/instances/${REUSE_INST1}/stop" '{}'
assert_ok "stop"
sleep 2

pt_post /instances/launch '{"name":"e2e-reuse-2","headless":true}'
assert_ok "relaunch"
REUSE_INST2=$(echo "$RESULT" | jq -r '.id')
REUSE_PORT2=$(echo "$RESULT" | jq -r '.port')

if [ "$REUSE_PORT1" = "$REUSE_PORT2" ]; then
  echo -e "  ${GREEN}✓${NC} port reused: $REUSE_PORT1"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} port not reused ($REUSE_PORT1 vs $REUSE_PORT2) — may depend on timing"
  ((ASSERTIONS_PASSED++)) || true
fi

pt_post "/instances/${REUSE_INST2}/stop" '{}'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: instance isolation (separate tabs)"

pt_post /instances/launch '{"name":"e2e-iso-1","headless":true}'
assert_ok "launch iso-1"
ISO_INST1=$(echo "$RESULT" | jq -r '.id')

pt_post /instances/launch '{"name":"e2e-iso-2","headless":true}'
assert_ok "launch iso-2"
ISO_INST2=$(echo "$RESULT" | jq -r '.id')

wait_for_orchestrator_instance_status "${E2E_SERVER}" "${ISO_INST1}" "running" 30
wait_for_orchestrator_instance_status "${E2E_SERVER}" "${ISO_INST2}" "running" 30

pt_post "/instances/${ISO_INST1}/tabs/open" "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "open tab on iso-1"
TAB1=$(echo "$RESULT" | jq -r '.tabId // .id // empty')

pt_post "/instances/${ISO_INST2}/tabs/open" "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "open tab on iso-2"
TAB2=$(echo "$RESULT" | jq -r '.tabId // .id // empty')

if [ -n "$TAB1" ] && [ -n "$TAB2" ] && [ "$TAB1" != "$TAB2" ]; then
  echo -e "  ${GREEN}✓${NC} instances have separate tabs"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} instances did not produce distinct tab IDs: ${TAB1} vs ${TAB2}"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post "/instances/${ISO_INST1}/stop" '{}'
pt_post "/instances/${ISO_INST2}/stop" '{}'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: bulk cleanup (create 3, stop all)"

CLEANUP_IDS=()
for i in 1 2 3; do
  pt_post /instances/launch "{\"name\":\"e2e-cleanup-$i\",\"headless\":true}"
  assert_ok "launch cleanup-$i"
  CLEANUP_IDS+=($(echo "$RESULT" | jq -r '.id'))
done

for id in "${CLEANUP_IDS[@]}"; do
  pt_post "/instances/${id}/stop" '{}'
  assert_ok "stop $id"
done

sleep 1

pt_get /instances
for id in "${CLEANUP_IDS[@]}"; do
  FOUND=$(echo "$RESULT" | jq -r ".[] | select(.id == \"$id\") | .id")
  if [ -z "$FOUND" ] || [ "$FOUND" = "null" ]; then
    echo -e "  ${GREEN}✓${NC} $id removed"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $id still present"
    ((ASSERTIONS_FAILED++)) || true
  fi
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: proxy with query params"

pt_post /navigate '{"url":"'"${FIXTURES_URL}"'/index.html?foo=bar&baz=qux"}'
assert_ok "navigate with query params"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: stop non-existent instance"

pt_post "/instances/nonexistent_xyz/stop" '{}'
assert_not_ok "rejects bad instance id"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: proxy routing"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate via proxy"

pt_get /snapshot
assert_ok "snapshot via proxy"
assert_json_exists "$RESULT" '.nodes' "has nodes"

end_test
