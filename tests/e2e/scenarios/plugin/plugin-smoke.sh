#!/bin/bash
# plugin-smoke.sh — multi-instance routing contracts the plugin's profile mapping rests on.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

PLUGIN_INST_IDS=()
PLUGIN_INST=""

# ─────────────────────────────────────────────────────────────────
start_test "plugin: /instances carries the id and status the readiness latch reads"

pt_get /instances
assert_ok "instances"
assert_json_length_gte "$RESULT" '.' 1 "at least one instance listed"

INCOMPLETE=$(echo "$RESULT" | jq '[.[] | select((.id // "") == "" or (.status // "") == "")] | length')
if [ "$INCOMPLETE" = "0" ]; then
  pass_assert "every instance entry reports both id and status"
else
  fail_assert "$INCOMPLETE instance entries are missing id or status"
fi

RUNNING_COUNT=$(echo "$RESULT" | jq '[.[] | select(.status == "running" and (.id // "") != "")] | length')
if [ "$RUNNING_COUNT" -ge 1 ]; then
  pass_assert "an instance reports status=running ($RUNNING_COUNT)"
else
  fail_assert "no instance reports status=running, so the readiness latch can never open"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: a profile-named instance owns the tabs opened on it"

pt_post /instances/start '{"mode":"headless"}'
assert_ok "launch instance"
PLUGIN_INST=$(echo "$RESULT" | jq -r '.id')
assert_instance_id_prefix "$PLUGIN_INST"
PLUGIN_INST_IDS+=("$PLUGIN_INST")

wait_for_orchestrator_instance_status "${E2E_SERVER}" "${PLUGIN_INST}" "running" 60

pt_get /instances
assert_instance_list_contains "$PLUGIN_INST" \
  "launched instance is listed for the readiness latch" \
  "launched instance never appeared in /instances"

pt_post "/instances/${PLUGIN_INST}/tabs/open" "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "open tab on the named instance"
assert_result_exists ".tabId" "returns tabId"
PLUGIN_TAB=$(get_tab_id)

pt_get "/instances/tabs?fresh=1"
assert_ok "list tabs across instances"
TAB_OWNER=$(echo "$RESULT" | jq -r --arg t "$PLUGIN_TAB" '.[] | select(.id == $t) | .instanceId' | head -n 1)
if [ "$TAB_OWNER" = "$PLUGIN_INST" ]; then
  pass_assert "tab is owned by ${PLUGIN_INST}"
else
  fail_assert "tab landed on '${TAB_OWNER}', expected ${PLUGIN_INST}"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: an unknown instance is refused, never silently rerouted"

REFUSED_URL="${FIXTURES_URL}/form.html?plugin-smoke-refused=1"

pt_post "/instances/inst_pinchtab_absent/tabs/open" "{\"url\":\"${REFUSED_URL}\"}"
assert_http_status 404 "open tab on an unknown instance"

pt_get "/instances/tabs?fresh=1"
assert_ok "list tabs after the refused open"
STRAY=$(echo "$RESULT" | jq --arg u "$REFUSED_URL" '[.[] | select(.url == $u)] | length')
if [ "$STRAY" = "0" ]; then
  pass_assert "the refused open created no tab on any instance"
else
  fail_assert "the refused open still opened a tab ($STRAY matching tabs)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: instance targeting is path-scoped, not a request-body field"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\",\"instanceId\":\"${PLUGIN_INST}\"}"
assert_ok "navigate carrying instanceId in the body"
BODY_TAB=$(get_tab_id)

pt_get "/instances/tabs?fresh=1"
assert_ok "list tabs after the body-routed navigate"
BODY_OWNER=$(echo "$RESULT" | jq -r --arg t "$BODY_TAB" '.[] | select(.id == $t) | .instanceId' | head -n 1)
if [ -z "$BODY_OWNER" ]; then
  fail_assert "navigate returned tab ${BODY_TAB} that no instance owns"
elif [ "$BODY_OWNER" != "$PLUGIN_INST" ]; then
  pass_assert "body instanceId does not route: tab landed on ${BODY_OWNER}, and only /instances/{id}/... targets an instance"
else
  fail_assert "body instanceId now routes to ${PLUGIN_INST}; update plugins/openclaw/tools/browser.ts and this expectation together"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: launched instances stop cleanly"

for id in "${PLUGIN_INST_IDS[@]}"; do
  pt_post "/instances/${id}/stop" '{}'
  assert_ok "stop ${id}"
done

wait_for_instances_gone "${E2E_SERVER}" 15 "${PLUGIN_INST_IDS[@]}" || true

pt_get /instances
assert_ok "instances after stop"
for id in "${PLUGIN_INST_IDS[@]}"; do
  assert_instance_list_absent "$id" "$id removed" "$id still present"
done

end_test
