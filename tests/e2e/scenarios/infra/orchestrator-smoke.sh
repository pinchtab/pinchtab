#!/bin/bash
# orchestrator-smoke.sh — slow multi-instance topology checks.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# Each /instances/start is synchronous while Chrome comes up. Issue
# independent launches in parallel so the test waits for max launch time
# instead of the sum.
parallel_post() {
  e2e_curl -s -w "\n%{http_code}" \
    -X POST "${E2E_SERVER}/instances/start" \
    -H "Content-Type: application/json" \
    -d "$2" > "$1"
}

read_resp() {
  local content
  content=$(cat "$1")
  printf -v "$3" '%s' "${content##*$'\n'}"
  printf -v "$2" '%s' "${content%$'\n'*}"
}

record_assertion() {
  if [[ "$1" =~ ^2 ]]; then
    pass_assert "$2 -> $1"
  else
    fail_assert "$2 -> $1"
  fi
}

ACTIVE_INST_IDS=()
INST1=""
INST2=""
INST3=""
INST4=""
PORT1=""
PORT2=""
PORT3=""

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: launch new instance"

OUT1=$(mktemp); OUT2=$(mktemp)
parallel_post "$OUT1" '{"mode":"headless"}' &
parallel_post "$OUT2" '{"mode":"headless"}' &
wait
read_resp "$OUT1" RESP1 CODE1
read_resp "$OUT2" RESP2 CODE2
rm -f "$OUT1" "$OUT2"
record_assertion "$CODE1" "launch instance"
assert_json_eq "$RESP1" '.mode' 'headless' "launch response includes mode"
assert_json_eq "$RESP1" '.headless' 'true' "launch response keeps headless boolean"
assert_json_exists "$RESP1" '.id' "has instance id"
assert_json_exists "$RESP1" '.port' "has port"

INST1=$(echo "$RESP1" | jq -r '.id')
PORT1=$(echo "$RESP1" | jq -r '.port')
INST2=$(echo "$RESP2" | jq -r '.id')
PORT2=$(echo "$RESP2" | jq -r '.port')
ACTIVE_INST_IDS+=("$INST1" "$INST2")

wait_for_orchestrator_instance_status "${E2E_SERVER}" "${INST1}" "running" 30

# Issue 545 contract: once an instance reports running, the first immediate
# tab open must succeed without a startup-race 500.
pt_post "/instances/${INST1}/tabs/open" '{"url":"about:blank"}'
assert_ok "running instance accepts immediate tabs/open"
assert_json_exists "$RESULT" '.tabId' "immediate tabs/open returns tab id"

pt_get /instances
assert_ok "list after launch"
assert_instance_list_contains "$INST1" "instance $INST1 in list" "instance $INST1 not in list"
record_assertion "$CODE2" "launch second instance for topology smoke"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: stop instance"

pt_post "/instances/${INST1}/stop" '{}'
assert_ok "stop instance"

wait_for_instances_gone "${E2E_SERVER}" 10 "${INST1}" || true
pt_get /instances
assert_instance_list_absent "$INST1" "instance removed after stop" "instance still in list after stop"
ACTIVE_INST_IDS=("${INST2}")

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: ports, isolation, and cleanup"

if [ "$PORT1" != "$PORT2" ]; then
  pass_assert "unique ports: $PORT1 vs $PORT2"
else
  fail_assert "duplicate ports: $PORT1"
fi

pt_post /instances/start '{"mode":"headless"}'
assert_ok "relaunch"
INST3=$(echo "$RESULT" | jq -r '.id')
PORT3=$(echo "$RESULT" | jq -r '.port')
ACTIVE_INST_IDS+=("$INST3")

if [ "$PORT1" = "$PORT3" ]; then
  pass_assert "port reused: $PORT1"
else
  skip_assert "port not reused ($PORT1 vs $PORT3); reuse depends on timing"
fi

if wait_for_instances_running "${E2E_SERVER}" 30 "${INST2}" "${INST3}"; then
  pass_assert "reused instances are running"
else
  fail_assert "reused instances did not both reach running"
fi

TAB_OUT1=$(mktemp); TAB_OUT2=$(mktemp)
e2e_curl -s -w "\n%{http_code}" \
  -X POST "${E2E_SERVER}/instances/${INST2}/tabs/open" \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"${FIXTURES_URL}/index.html\"}" > "$TAB_OUT1" &
e2e_curl -s -w "\n%{http_code}" \
  -X POST "${E2E_SERVER}/instances/${INST3}/tabs/open" \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"${FIXTURES_URL}/form.html\"}" > "$TAB_OUT2" &
wait
read_resp "$TAB_OUT1" TAB_RESP1 TAB_CODE1
read_resp "$TAB_OUT2" TAB_RESP2 TAB_CODE2
rm -f "$TAB_OUT1" "$TAB_OUT2"
record_assertion "$TAB_CODE1" "open tab on second instance"
record_assertion "$TAB_CODE2" "open tab on reused instance"
TAB1=$(get_tab_id "$TAB_RESP1")
TAB2=$(get_tab_id "$TAB_RESP2")

pt_get /instances/tabs
assert_ok "aggregate tabs"
assert_json_length_gte "$RESULT" '.' '2' "at least 2 tabs across instances"

if [ -n "$TAB1" ] && [ -n "$TAB2" ] && [ "$TAB1" != "$TAB2" ]; then
  pass_assert "instances have separate tabs"
else
  fail_assert "instances did not produce distinct tab IDs: ${TAB1} vs ${TAB2}"
fi

pt_post /instances/start '{"mode":"headless"}'
assert_ok "launch cleanup-3"
INST4=$(echo "$RESULT" | jq -r '.id')
ACTIVE_INST_IDS+=("$INST4")

declare -a STOP_OUTS=()
for id in "${ACTIVE_INST_IDS[@]}"; do
  out=$(mktemp)
  STOP_OUTS+=("$out")
  e2e_curl -s -w "\n%{http_code}" \
    -X POST "${E2E_SERVER}/instances/${id}/stop" \
    -H "Content-Type: application/json" \
    -d '{}' > "$out" &
done
wait
for i in "${!ACTIVE_INST_IDS[@]}"; do
  read_resp "${STOP_OUTS[$i]}" _ STOP_CODE
  record_assertion "$STOP_CODE" "stop ${ACTIVE_INST_IDS[$i]}"
  rm -f "${STOP_OUTS[$i]}"
done

wait_for_instances_gone "${E2E_SERVER}" 10 "${ACTIVE_INST_IDS[@]}" || true

pt_get /instances
for id in "${ACTIVE_INST_IDS[@]}"; do
  FOUND=$(echo "$RESULT" | jq -r ".[] | select(.id == \"$id\") | .id")
  if [ -z "$FOUND" ] || [ "$FOUND" = "null" ]; then
    pass_assert "$id removed"
  else
    fail_assert "$id still present"
  fi
done

end_test
