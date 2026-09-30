#!/bin/bash
# security-smoke.sh — slow strict-instance policy boundary checks.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

secure_get() {
  with_server "$E2E_SECURE_SERVER" pt_get "$@"
}

secure_post() {
  with_server "$E2E_SECURE_SERVER" pt_post "$@"
}

PIVOT_URL="http://pivot-target:80/index.html"

# ─────────────────────────────────────────────────────────────────
start_test "security: instance-scoped allowedDomains widen one strict instance only"

secure_post /navigate -d "{\"url\":\"${PIVOT_URL}\"}"
assert_http_status 403 "default strict server blocks pivot-target"

secure_post /instances/start -d '{"mode":"headless","securityPolicy":{"allowedDomains":["pivot-target"]}}'
assert_http_status 201 "start widened instance"
SECURE_WIDE_INST_ID=$(echo "$RESULT" | jq -r '.id // empty')

if [ -n "$SECURE_WIDE_INST_ID" ] && wait_for_orchestrator_instance_status "${E2E_SECURE_SERVER}" "${SECURE_WIDE_INST_ID}" "running" 30; then
  if echo "$RESULT" | jq -e '.securityPolicy.allowedDomains | index("pivot-target")' >/dev/null 2>&1; then
    pass_assert "widened instance exposes pivot-target in securityPolicy.allowedDomains"
  else
    fail_assert "widened instance response missing pivot-target in securityPolicy.allowedDomains"
  fi

  secure_post "/instances/${SECURE_WIDE_INST_ID}/tabs/open" "{\"url\":\"${PIVOT_URL}\"}"
  assert_ok "widened instance can open pivot-target"
  WIDE_TAB_ID=$(get_tab_id)
  if [ -n "$WIDE_TAB_ID" ]; then
    secure_get "/tabs/${WIDE_TAB_ID}/text"
    assert_ok "widened instance text works on pivot-target"
    assert_contains "$RESULT" "Welcome to the E2E test fixtures." "pivot-target serves expected fixture content"
  fi
fi

secure_post /navigate -d "{\"url\":\"${PIVOT_URL}\"}"
assert_http_status 403 "default strict server still blocks pivot-target after widened instance launch"

if [ -n "${SECURE_WIDE_INST_ID:-}" ]; then
  secure_post "/instances/${SECURE_WIDE_INST_ID}/stop" '{}'
  assert_ok "stop widened instance"
  wait_for_instances_gone "${E2E_SECURE_SERVER}" 10 "${SECURE_WIDE_INST_ID}" || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: an instance-scoped bare wildcard grants no private-IP override"

secure_post /instances/start -d '{"mode":"headless","securityPolicy":{"allowedDomains":["*"]}}'
assert_http_status 201 "start wildcard instance"
SECURE_WILDCARD_INST_ID=$(echo "$RESULT" | jq -r '.id // empty')

if [ -n "$SECURE_WILDCARD_INST_ID" ] && wait_for_orchestrator_instance_status "${E2E_SECURE_SERVER}" "${SECURE_WILDCARD_INST_ID}" "running" 30; then
  if echo "$RESULT" | jq -e '.securityPolicy.allowedDomains | index("*")' >/dev/null 2>&1; then
    pass_assert "wildcard instance exposes * in securityPolicy.allowedDomains"
  else
    fail_assert "wildcard instance response missing * in securityPolicy.allowedDomains"
  fi

  secure_post "/instances/${SECURE_WILDCARD_INST_ID}/tabs/open" "{\"url\":\"${PIVOT_URL}\"}"
  assert_http_status 403 "wildcard instance is refused the private pivot-target"
  assert_contains "$RESULT" "private/internal IP" "the refusal names the private-IP guard, not the domain allowlist"
fi

secure_post /navigate -d "{\"url\":\"${PIVOT_URL}\"}"
assert_http_status 403 "default strict server still blocks pivot-target with wildcard instance running"

if [ -n "${SECURE_WILDCARD_INST_ID:-}" ]; then
  secure_post "/instances/${SECURE_WILDCARD_INST_ID}/stop" '{}'
  assert_ok "stop wildcard instance"
  wait_for_instances_gone "${E2E_SECURE_SERVER}" 10 "${SECURE_WILDCARD_INST_ID}" || true
fi

end_test
