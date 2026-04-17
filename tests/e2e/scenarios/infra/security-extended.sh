#!/bin/bash
# security-extended.sh — API extended security scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

secure_get() {
  local path="$1"
  shift
  local old_url="$E2E_SERVER"
  E2E_SERVER="$E2E_SECURE_SERVER"
  pt_get "$path" "$@"
  E2E_SERVER="$old_url"
}

secure_post() {
  local path="$1"
  shift
  local old_url="$E2E_SERVER"
  E2E_SERVER="$E2E_SECURE_SERVER"
  pt_post "$path" "$@"
  E2E_SERVER="$old_url"
}

start_test "security: evaluate BLOCKED when disabled"

secure_post /navigate -d '{"url":"about:blank"}'
secure_post /evaluate -d '{"expression":"1+1"}'
assert_http_status 403 "evaluate blocked"
assert_contains "$RESULT" "evaluate_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: wait fn BLOCKED when evaluate disabled"

secure_post /navigate -d '{"url":"http://fixtures:80/index.html"}'
assert_ok "navigate to allowed fixture page"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

secure_post /wait -d "{\"tabId\":\"${TAB_ID}\",\"fn\":\"true\",\"timeout\":1000}"
assert_http_status 403 "wait fn blocked"
assert_contains "$RESULT" "evaluate_disabled" "wait fn uses evaluate_disabled guard"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: download BLOCKED when disabled"

secure_get "/download?url=${FIXTURES_URL}/sample.txt"
assert_http_status 403 "download blocked"
assert_contains "$RESULT" "download_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: upload BLOCKED when disabled"

secure_post /upload -d '{"selector":"#single-file","files":["data:text/plain;base64,dGVzdA=="]}'
assert_http_status 403 "upload blocked"
assert_contains "$RESULT" "upload_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: IDPI blocks non-whitelisted domains"

secure_post /navigate -d '{"url":"https://example.com"}'
assert_http_status 403 "navigate blocked by IDPI"
assert_contains "$RESULT" "IDPI\|blocked\|allowed" "IDPI error message"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: instance-scoped allowedDomains widen one strict instance only"

PIVOT_URL="http://pivot-target:80/index.html"

secure_post /navigate -d "{\"url\":\"${PIVOT_URL}\"}"
assert_http_status 403 "default strict server blocks pivot-target"

secure_post /instances/start -d '{"mode":"headless","securityPolicy":{"allowedDomains":["pivot-target"]}}'
assert_http_status 201 "start widened instance"
SECURE_WIDE_INST_ID=$(echo "$RESULT" | jq -r '.id // empty')

if [ -n "$SECURE_WIDE_INST_ID" ] && wait_for_orchestrator_instance_status "${E2E_SECURE_SERVER}" "${SECURE_WIDE_INST_ID}" "running" 30; then
  if echo "$RESULT" | jq -e '.securityPolicy.allowedDomains | index("pivot-target")' >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} widened instance exposes pivot-target in securityPolicy.allowedDomains"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} widened instance response missing pivot-target in securityPolicy.allowedDomains"
    ((ASSERTIONS_FAILED++)) || true
  fi

  secure_post "/instances/${SECURE_WIDE_INST_ID}/tabs/open" "{\"url\":\"${PIVOT_URL}\"}"
  assert_ok "widened instance can open pivot-target"
  WIDE_TAB_ID=$(echo "$RESULT" | jq -r '.tabId // empty')
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
start_test "security: instance-scoped wildcard widens one strict instance only"

PIVOT_URL="http://pivot-target:80/index.html"

secure_post /instances/start -d '{"mode":"headless","securityPolicy":{"allowedDomains":["*"]}}'
assert_http_status 201 "start wildcard instance"
SECURE_WILDCARD_INST_ID=$(echo "$RESULT" | jq -r '.id // empty')

if [ -n "$SECURE_WILDCARD_INST_ID" ] && wait_for_orchestrator_instance_status "${E2E_SECURE_SERVER}" "${SECURE_WILDCARD_INST_ID}" "running" 30; then
  if echo "$RESULT" | jq -e '.securityPolicy.allowedDomains | index("*")' >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} wildcard instance exposes * in securityPolicy.allowedDomains"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} wildcard instance response missing * in securityPolicy.allowedDomains"
    ((ASSERTIONS_FAILED++)) || true
  fi

  secure_post "/instances/${SECURE_WILDCARD_INST_ID}/tabs/open" "{\"url\":\"${PIVOT_URL}\"}"
  assert_ok "wildcard instance can open pivot-target"
  WILDCARD_TAB_ID=$(echo "$RESULT" | jq -r '.tabId // empty')
  if [ -n "$WILDCARD_TAB_ID" ]; then
    secure_get "/tabs/${WILDCARD_TAB_ID}/text"
    assert_ok "wildcard instance text works on pivot-target"
    assert_contains "$RESULT" "Welcome to the E2E test fixtures." "wildcard instance reaches non-baseline host"
  fi
fi

secure_post /navigate -d "{\"url\":\"${PIVOT_URL}\"}"
assert_http_status 403 "default strict server still blocks pivot-target with wildcard instance running"

if [ -n "${SECURE_WILDCARD_INST_ID:-}" ]; then
  secure_post "/instances/${SECURE_WILDCARD_INST_ID}/stop" '{}'
  assert_ok "stop wildcard instance"
  wait_for_instances_gone "${E2E_SECURE_SERVER}" 10 "${SECURE_WILDCARD_INST_ID}" || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: blocked responses include helpful info"

secure_post /evaluate -d '{"expression":"1"}'
assert_http_status 403 "returns 403"
assert_json_exists "$RESULT" ".code" "has error code"
assert_json_exists "$RESULT" ".error" "has error message"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: stateExport BLOCKED when disabled"

secure_get /state/list
assert_http_status 403 "state list blocked"
assert_contains "$RESULT" "state_export_disabled" "correct error code"

secure_get /storage
assert_http_status 403 "storage get blocked"
assert_contains "$RESULT" "state_export_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: state save/load BLOCKED when disabled"

secure_post /state/save -d '{"name":"test-blocked"}'
assert_http_status 403 "state save blocked"
assert_contains "$RESULT" "state_export_disabled" "correct error code"

secure_post /state/load -d '{"name":"test-blocked"}'
assert_http_status 403 "state load blocked"
assert_contains "$RESULT" "state_export_disabled" "correct error code"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "security: tab-scoped storage BLOCKED when stateExport disabled"

secure_get "/tabs"
TAB_ID=$(echo "$RESULT" | jq -r '.tabs[0].id // empty')

if [ -n "$TAB_ID" ]; then
  secure_get "/tabs/${TAB_ID}/storage"
  assert_http_status 403 "tab storage blocked"
  assert_contains "$RESULT" "state_export_disabled" "correct error code"
fi

end_test
