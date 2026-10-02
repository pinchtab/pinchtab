#!/bin/bash
# orchestrator-extended.sh — API full orchestration scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

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
  pass_assert "attached bridge instance id: ${ATTACHED_INST_ID}"
else
  fail_assert "attach response missing instance id"
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
  pass_assert "aggregate tab list includes attached bridge instance"
else
  fail_assert "aggregate tab list missing attached bridge instance"
fi

DEFAULT_IDS=""
for _ in 1 2 3 4 5 6 7 8; do
  pt_get /health
  DEFAULT_IDS="${DEFAULT_IDS}$(echo "$RESULT" | jq -r '.defaultInstance.id // empty')"$'\n'
done
DEFAULT_ID=$(printf '%s' "$DEFAULT_IDS" | head -1)
if [ -n "$DEFAULT_ID" ] && [ "$(printf '%s' "$DEFAULT_IDS" | sort -u | wc -l | tr -d ' ')" = "1" ]; then
  pass_assert "health defaultInstance stays ${DEFAULT_ID} across calls with two instances"
else
  fail_assert "health defaultInstance changed between calls: $(printf '%s' "$DEFAULT_IDS" | tr '\n' ' ')"
fi
if [ "$DEFAULT_ID" != "$ATTACHED_INST_ID" ]; then
  pass_assert "health defaultInstance is the earlier instance, not the attached bridge"
else
  fail_assert "health defaultInstance named the later-attached bridge ${ATTACHED_INST_ID}"
fi

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "shorthand navigate"
assert_tab_id "shorthand navigate returned tabId"
pt_get /instances/tabs
SHORTHAND_INST_ID=$(echo "$RESULT" | jq -r --arg tab "$TAB_ID" '[.[] | select(.id == $tab or .tabId == $tab)][0].instanceId // empty')
if [ -n "$SHORTHAND_INST_ID" ] && [ "$SHORTHAND_INST_ID" = "$DEFAULT_ID" ]; then
  pass_assert "health defaultInstance is the instance shorthand routes use"
else
  fail_assert "shorthand route used '${SHORTHAND_INST_ID}' but health defaultInstance is '${DEFAULT_ID}'"
fi

pt_post "/instances/${ATTACHED_INST_ID}/stop" '{}'
assert_ok "stop attached bridge instance"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: front door routes about:blank tabs on a second instance"

pt_post /instances/start '{"mode":"headless"}'
assert_ok "start a second instance"
TRANSIENT_INST_ID=$(echo "$RESULT" | jq -r '.id // empty')
wait_for_orchestrator_instance_status "${E2E_SERVER}" "${TRANSIENT_INST_ID}" "running" 60
pt_get /health
TRANSIENT_DEFAULT_ID=$(echo "$RESULT" | jq -r '.defaultInstance.id // empty')
if [ -n "$TRANSIENT_INST_ID" ] && [ -n "$TRANSIENT_DEFAULT_ID" ] && [ "$TRANSIENT_INST_ID" != "$TRANSIENT_DEFAULT_ID" ]; then
  pass_assert "two instances: default ${TRANSIENT_DEFAULT_ID}, second ${TRANSIENT_INST_ID}"
else
  fail_assert "two distinct instances (default '${TRANSIENT_DEFAULT_ID}', second '${TRANSIENT_INST_ID}')"
fi

pt_post "/instances/${TRANSIENT_INST_ID}/tabs/open" '{"url":"about:blank"}'
assert_ok "open about:blank tab to navigate"
assert_tab_id "about:blank tab to navigate returned tabId"
BLANK_NAV_TAB="${TAB_ID}"

pt_post "/instances/${TRANSIENT_INST_ID}/tabs/open" '{"url":"about:blank"}'
assert_ok "open about:blank tab to close"
assert_tab_id "about:blank tab to close returned tabId"
BLANK_CLOSE_TAB="${TAB_ID}"

pt_get "/instances/${TRANSIENT_INST_ID}/tabs"
assert_ok "list second instance tabs"
if echo "$RESULT" | jq -e --arg a "$BLANK_NAV_TAB" --arg b "$BLANK_CLOSE_TAB" 'map(select(.id == $a or .id == $b)) | length == 0' >/dev/null 2>&1; then
  pass_assert "GET /instances/{id}/tabs omits the about:blank tabs"
else
  fail_assert "GET /instances/{id}/tabs omits the about:blank tabs (got $RESULT)"
fi

pt_post "/tabs/${BLANK_NAV_TAB}/navigate" "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "front-door navigate of an about:blank tab on the second instance"
assert_contains "$RESULT" "buttons.html" "navigate landed on the fixture"

pt_post "/tabs/${BLANK_CLOSE_TAB}/close" '{}'
assert_ok "front-door close of an about:blank tab on the second instance"

pt_get "/screencast/tabs?tabId=${BLANK_NAV_TAB}"
if echo "$RESULT" | jq -e --arg id "$BLANK_CLOSE_TAB" 'type == "array" and (map(select(.id == $id)) | length == 0)' >/dev/null 2>&1; then
  pass_assert "the closed tab is gone from the second instance's Chrome"
else
  fail_assert "the closed tab is gone from the second instance's Chrome (got $RESULT)"
fi
pt_post "/tabs/${BLANK_NAV_TAB}/close" '{}'
assert_ok "close the navigated tab"

pt_post "/instances/${TRANSIENT_DEFAULT_ID}/tabs/open" '{"url":"about:blank"}'
assert_ok "open about:blank tab on the default instance"
assert_tab_id "default-instance about:blank tab returned tabId"
DEFAULT_BLANK_TAB="${TAB_ID}"
for TABS_QUERY in "" "?includeTransient=1"; do
  pt_get "/tabs${TABS_QUERY}"
  assert_ok "public GET /tabs${TABS_QUERY}"
  if echo "$RESULT" | jq -e --arg id "$DEFAULT_BLANK_TAB" '(.tabs | type == "array") and (.tabs | map(select(.id == $id)) | length == 0)' >/dev/null 2>&1; then
    pass_assert "public GET /tabs${TABS_QUERY} omits the about:blank tab"
  else
    fail_assert "public GET /tabs${TABS_QUERY} omits the about:blank tab (got $RESULT)"
  fi
done
pt_post "/tabs/${DEFAULT_BLANK_TAB}/close" '{}'
assert_ok "front-door close of an about:blank tab on the default instance"

pt_post "/instances/${TRANSIENT_INST_ID}/stop" '{}'
assert_ok "stop the second instance"

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
INST_ID=$(echo "$RESULT" | jq -r '.[0].id // empty')
if [ -n "$INST_ID" ] && [ "$INST_ID" != "null" ]; then
  pass_assert "selected instance id: $INST_ID"
else
  fail_assert "list response did not include an instance id"
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
start_test "orchestrator: instance tabs"

wait_for_orchestrator_instance_status "${E2E_SERVER}" "${INST_ID}" "running" 10

pt_get "/instances/${INST_ID}/tabs"
assert_ok "instance tabs"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "orchestrator: ID format (inst_ prefix)"

assert_instance_id_prefix "$INST_ID"

end_test

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
