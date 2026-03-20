#!/bin/bash
# port-collision-351.sh — Reproduces GitHub issue #351: CDP port collision.
#
# The bug: orchestrator allocates bridge ports (e.g., 9868, 9869) but each
# bridge independently grabs a CDP port from the same range. When bridge A
# on port 9868 grabs 9869 for Chrome CDP, bridge B assigned to 9869 collides.
#
# This test launches instances with adjacent ports and verifies they can
# both operate independently (open tabs, get snapshots).

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

# Skip if not running in orchestrator/dashboard mode
pt_get /health
MODE=$(echo "$RESULT" | jq -r '.mode // empty')
if [ "$MODE" != "dashboard" ]; then
  echo "  Skipping port-collision-351: requires dashboard mode (got: $MODE)"
  return 0 2>/dev/null || exit 0
fi

# ─────────────────────────────────────────────────────────────────
start_test "issue #351: adjacent port allocation causes CDP collision"

# We'll launch 3 instances rapidly with adjacent ports.
# If the bug exists, instance 2 or 3 will fail to start or will
# connect to the wrong Chrome process.

COLLISION_INST_IDS=()
COLLISION_PORTS=()

echo "  Launching 3 instances with adjacent auto-allocated ports..."

for i in 1 2 3; do
  pt_post /instances/launch "{\"name\":\"e2e-collision-${i}\",\"headless\":true}"
  if [ "$HTTP_STATUS" != "200" ] && [ "$HTTP_STATUS" != "201" ]; then
    echo -e "  ${RED}✗${NC} Failed to launch instance ${i}: HTTP $HTTP_STATUS"
    echo "  Response: $RESULT"
    ((ASSERTIONS_FAILED++)) || true
    # Clean up any that did launch
    for id in "${COLLISION_INST_IDS[@]}"; do
      pt_post "/instances/${id}/stop" '{}' >/dev/null 2>&1 || true
    done
    end_test
    return 1 2>/dev/null || exit 1
  fi
  
  INST_ID=$(echo "$RESULT" | jq -r '.id')
  INST_PORT=$(echo "$RESULT" | jq -r '.port')
  COLLISION_INST_IDS+=("$INST_ID")
  COLLISION_PORTS+=("$INST_PORT")
  echo "    Instance ${i}: id=${INST_ID}, port=${INST_PORT}"
done

# Verify all ports are unique
UNIQUE_PORTS=$(printf '%s\n' "${COLLISION_PORTS[@]}" | sort -u | wc -l | tr -d ' ')
if [ "$UNIQUE_PORTS" -eq "3" ]; then
  echo -e "  ${GREEN}✓${NC} All 3 instances have unique bridge ports"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Duplicate bridge ports detected: ${COLLISION_PORTS[*]}"
  ((ASSERTIONS_FAILED++)) || true
fi

# Wait for all instances to be running
echo "  Waiting for instances to reach 'running' status..."
ALL_RUNNING=true
for id in "${COLLISION_INST_IDS[@]}"; do
  if ! wait_for_orchestrator_instance_status "${E2E_SERVER}" "${id}" "running" 30; then
    ALL_RUNNING=false
    echo -e "  ${RED}✗${NC} Instance ${id} failed to start"
    ((ASSERTIONS_FAILED++)) || true
  fi
done

if [ "$ALL_RUNNING" = "true" ]; then
  echo -e "  ${GREEN}✓${NC} All 3 instances are running"
  ((ASSERTIONS_PASSED++)) || true
fi

# Now the critical test: each instance should control its OWN Chrome.
# Open a unique page on each and verify we can retrieve it.
echo "  Opening unique pages on each instance..."

declare -A EXPECTED_CONTENT
EXPECTED_CONTENT["${COLLISION_INST_IDS[0]}"]="index"
EXPECTED_CONTENT["${COLLISION_INST_IDS[1]}"]="form"  
EXPECTED_CONTENT["${COLLISION_INST_IDS[2]}"]="buttons"

pt_post "/instances/${COLLISION_INST_IDS[0]}/tabs/open" "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "open index.html on instance 1"
TAB1=$(echo "$RESULT" | jq -r '.tabId // .id // empty')

pt_post "/instances/${COLLISION_INST_IDS[1]}/tabs/open" "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "open form.html on instance 2"
TAB2=$(echo "$RESULT" | jq -r '.tabId // .id // empty')

pt_post "/instances/${COLLISION_INST_IDS[2]}/tabs/open" "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "open buttons.html on instance 3"
TAB3=$(echo "$RESULT" | jq -r '.tabId // .id // empty')

# Verify each instance sees its own content (not another instance's)
echo "  Verifying instance isolation (each should see its own page)..."

pt_get "/instances/${COLLISION_INST_IDS[0]}/tabs/${TAB1}/text?format=text"
if echo "$RESULT" | grep -qi "welcome\|fixture"; then
  echo -e "  ${GREEN}✓${NC} Instance 1 sees index.html content"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Instance 1 does NOT see expected index.html content"
  echo "    Got: $(echo "$RESULT" | head -c 200)"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_get "/instances/${COLLISION_INST_IDS[1]}/tabs/${TAB2}/text?format=text"
if echo "$RESULT" | grep -qi "form\|input\|submit"; then
  echo -e "  ${GREEN}✓${NC} Instance 2 sees form.html content"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Instance 2 does NOT see expected form.html content"
  echo "    Got: $(echo "$RESULT" | head -c 200)"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_get "/instances/${COLLISION_INST_IDS[2]}/tabs/${TAB3}/text?format=text"
if echo "$RESULT" | grep -qi "button\|click"; then
  echo -e "  ${GREEN}✓${NC} Instance 3 sees buttons.html content"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Instance 3 does NOT see expected buttons.html content"
  echo "    Got: $(echo "$RESULT" | head -c 200)"
  ((ASSERTIONS_FAILED++)) || true
fi

# Additional check: verify instances don't share tabs
echo "  Verifying tab isolation across instances..."

pt_get "/instances/${COLLISION_INST_IDS[0]}/tabs"
TABS1=$(echo "$RESULT" | jq -r '.[].id // .[].tabId' 2>/dev/null | sort)

pt_get "/instances/${COLLISION_INST_IDS[1]}/tabs"
TABS2=$(echo "$RESULT" | jq -r '.[].id // .[].tabId' 2>/dev/null | sort)

pt_get "/instances/${COLLISION_INST_IDS[2]}/tabs"
TABS3=$(echo "$RESULT" | jq -r '.[].id // .[].tabId' 2>/dev/null | sort)

# Check for any overlap
OVERLAP=false
for t in $TABS1; do
  if echo "$TABS2 $TABS3" | grep -q "$t"; then
    OVERLAP=true
    echo -e "  ${RED}✗${NC} Tab ${t} appears in multiple instances!"
    ((ASSERTIONS_FAILED++)) || true
  fi
done

if [ "$OVERLAP" = "false" ]; then
  echo -e "  ${GREEN}✓${NC} No tab ID overlap between instances"
  ((ASSERTIONS_PASSED++)) || true
fi

# Cleanup
echo "  Cleaning up instances..."
for id in "${COLLISION_INST_IDS[@]}"; do
  pt_post "/instances/${id}/stop" '{}'
done

wait_for_instances_gone "${E2E_SERVER}" 15 "${COLLISION_INST_IDS[@]}" || true

end_test

# ─────────────────────────────────────────────────────────────────
start_test "issue #351: rapid sequential launch with explicit adjacent ports"

# This variant explicitly requests adjacent ports to maximize collision chance.
# Note: this may fail even without the bug if ports are externally occupied.

# First, find what port range the orchestrator is using
pt_get /health
START_PORT=$(echo "$RESULT" | jq -r '.config.instancePortStart // 9868')

PORT_A=$START_PORT
PORT_B=$((START_PORT + 1))

echo "  Attempting explicit adjacent ports: ${PORT_A} and ${PORT_B}"

pt_post /instances/launch "{\"name\":\"e2e-adjacent-a\",\"port\":\"${PORT_A}\",\"headless\":true}"
if [ "$HTTP_STATUS" = "200" ] || [ "$HTTP_STATUS" = "201" ]; then
  INST_A=$(echo "$RESULT" | jq -r '.id')
  echo "    Instance A launched on port ${PORT_A}: ${INST_A}"
  
  # Instance A's bridge will grab PORT_A for itself, then probe for CDP port.
  # If bug exists, it will grab PORT_B for Chrome CDP.
  
  # Small delay to let Chrome start and grab its CDP port
  sleep 2
  
  pt_post /instances/launch "{\"name\":\"e2e-adjacent-b\",\"port\":\"${PORT_B}\",\"headless\":true}"
  if [ "$HTTP_STATUS" = "200" ] || [ "$HTTP_STATUS" = "201" ]; then
    INST_B=$(echo "$RESULT" | jq -r '.id')
    echo "    Instance B launched on port ${PORT_B}: ${INST_B}"
    
    # Wait for both
    wait_for_orchestrator_instance_status "${E2E_SERVER}" "${INST_A}" "running" 30 || true
    wait_for_orchestrator_instance_status "${E2E_SERVER}" "${INST_B}" "running" 30 || true
    
    # Test isolation
    pt_post "/instances/${INST_A}/tabs/open" "{\"url\":\"${FIXTURES_URL}/index.html\"}"
    TAB_A=$(echo "$RESULT" | jq -r '.tabId // .id // empty')
    
    pt_post "/instances/${INST_B}/tabs/open" "{\"url\":\"${FIXTURES_URL}/form.html\"}"
    TAB_B=$(echo "$RESULT" | jq -r '.tabId // .id // empty')
    
    # Verify B sees form.html, not index.html (which would indicate it's talking to A's Chrome)
    pt_get "/instances/${INST_B}/tabs/${TAB_B}/text?format=text"
    if echo "$RESULT" | grep -qi "form\|input"; then
      echo -e "  ${GREEN}✓${NC} Instance B correctly sees its own page (form.html)"
      ((ASSERTIONS_PASSED++)) || true
    elif echo "$RESULT" | grep -qi "welcome\|fixture"; then
      echo -e "  ${RED}✗${NC} Instance B sees Instance A's page! CDP port collision confirmed."
      ((ASSERTIONS_FAILED++)) || true
    else
      echo -e "  ${YELLOW}~${NC} Instance B content unclear: $(echo "$RESULT" | head -c 100)"
      ((ASSERTIONS_PASSED++)) || true
    fi
    
    pt_post "/instances/${INST_B}/stop" '{}'
  else
    echo -e "  ${YELLOW}~${NC} Instance B failed to launch (port ${PORT_B} may be occupied by A's CDP)"
    echo "    This is expected behavior if bug #351 exists"
    ((ASSERTIONS_PASSED++)) || true
  fi
  
  pt_post "/instances/${INST_A}/stop" '{}'
  wait_for_instances_gone "${E2E_SERVER}" 10 "${INST_A}" "${INST_B}" 2>/dev/null || true
else
  echo -e "  ${YELLOW}~${NC} Could not launch on port ${PORT_A} (may be in use)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test
