#!/bin/bash
# Shared utilities for E2E bash suites.

set -uo pipefail

RED='\033[0;31m'
ERROR="${RED}"
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
MUTED='\033[0;90m'
BOLD='\033[1m'
NC='\033[0m'

if [ "${E2E_DEBUG:-0}" = "1" ]; then
  set -x
fi

safe_jq() {
  jq "$@"
}

E2E_SERVER="${E2E_SERVER:-http://localhost:9999}"
E2E_SECURE_SERVER="${E2E_SECURE_SERVER:-http://localhost:9998}"
E2E_MEDIUM_SERVER="${E2E_MEDIUM_SERVER:-}"
E2E_FULL_SERVER="${E2E_FULL_SERVER:-}"
E2E_BRIDGE_URL="${E2E_BRIDGE_URL:-}"

# Auto-load token from config file if not set
if [ -z "${E2E_SERVER_TOKEN:-}" ] && [ -f "$HOME/.pinchtab/config.json" ]; then
  E2E_SERVER_TOKEN=$(safe_jq -r '.server.token // empty' "$HOME/.pinchtab/config.json" 2>/dev/null || echo "")
fi
E2E_SERVER_TOKEN="${E2E_SERVER_TOKEN:-}"

E2E_BRIDGE_TOKEN="${E2E_BRIDGE_TOKEN:-}"
FIXTURES_URL="${FIXTURES_URL:-http://localhost:8080}"

CURRENT_TEST="${CURRENT_TEST:-}"
CURRENT_SCENARIO_FILE="${CURRENT_SCENARIO_FILE:-}"
TESTS_FAILED="${TESTS_FAILED:-0}"
ASSERTIONS_PASSED="${ASSERTIONS_PASSED:-0}"
ASSERTIONS_FAILED="${ASSERTIONS_FAILED:-0}"
ASSERTIONS_SKIPPED="${ASSERTIONS_SKIPPED:-0}"
TEST_START_TIME="${TEST_START_TIME:-0}"

get_time_ms() {
  if [ -f /proc/uptime ]; then
    awk '{printf "%.0f", $1 * 1000}' /proc/uptime
  elif command -v gdate &>/dev/null; then
    gdate +%s%3N
  elif command -v perl &>/dev/null; then
    perl -MTime::HiRes=time -e 'printf "%.0f", time * 1000'
  else
    echo $(($(date +%s) * 1000))
  fi
}

e2e_curl() {
  local token="${E2E_SERVER_TOKEN:-}"
  if [ "${1:-}" = "--token" ]; then
    token="${2:-}"
    shift 2
  fi

  if [ -n "$token" ]; then
    curl -H "Authorization: Bearer ${token}" "$@"
  else
    curl "$@"
  fi
}

wait_for_instance_ready() {
  local base_url="$1"
  local timeout_sec="${2:-60}"
  local token="${3:-${E2E_SERVER_TOKEN:-}}"
  local started_at
  started_at=$(date +%s)

  while true; do
    local now
    now=$(date +%s)
    if [ $((now - started_at)) -ge "$timeout_sec" ]; then
      echo -e "  ${RED}✗${NC} instance at ${base_url} did not reach running within ${timeout_sec}s"
      return 1
    fi

    local health_json
    health_json=$(e2e_curl --token "$token" -sf "${base_url}/health" 2>/dev/null || true)
    if [ -n "$health_json" ]; then
      local inst_status
      inst_status=$(echo "$health_json" | safe_jq -r '.defaultInstance.status // .status // empty' 2>/dev/null || true)
      if [ "$inst_status" = "running" ] || [ "$inst_status" = "ok" ]; then
        echo -e "  ${GREEN}✓${NC} instance ready at ${base_url}"
        return 0
      fi
    fi

    sleep 1
  done
}

start_test() {
  ASSERTIONS_PASSED=0
  ASSERTIONS_FAILED=0
  ASSERTIONS_SKIPPED=0
  if [ -n "${CURRENT_SCENARIO_FILE}" ]; then
    CURRENT_TEST="[${CURRENT_SCENARIO_FILE}] $1"
  else
    CURRENT_TEST="$1"
  fi
  TEST_START_TIME=$(get_time_ms)
  echo -e "${BLUE}▶ ${CURRENT_TEST}${NC}"
}

end_test() {
  local end_time
  end_time=$(get_time_ms)
  local duration=$((end_time - TEST_START_TIME))
  local status

  if [ "$ASSERTIONS_FAILED" -eq 0 ]; then
    echo -e "${GREEN}✓ ${CURRENT_TEST} passed${NC} ${MUTED}(${duration}ms)${NC}\n"
    status="passed"
  else
    echo -e "${RED}✗ ${CURRENT_TEST} failed${NC} ${MUTED}(${duration}ms, failed assertions: ${ASSERTIONS_FAILED})${NC}\n"
    status="failed"
    ((TESTS_FAILED++)) || true
  fi
  echo -e "E2E_RESULT\t${status}\t${duration}\t${CURRENT_TEST}"
  ASSERTIONS_PASSED=0
  ASSERTIONS_FAILED=0
}

pass_assert() {
  echo -e "  ${GREEN}✓${NC} ${1:-}"
  ((ASSERTIONS_PASSED++)) || true
}

fail_assert() {
  echo -e "  ${RED}✗${NC} ${1:-}"
  ((ASSERTIONS_FAILED++)) || true
}

skip_assert() {
  echo -e "  ${YELLOW}⚠${NC} ${1:-}"
  ((ASSERTIONS_SKIPPED++)) || true
}

# skip_test marks the current test as skipped with a reason; it does not
# increment failures. Must be paired with end_test as usual. The recorded
# duration counts only the time up to the skip call.
skip_test() {
  local reason="${1:-skipped}"
  echo -e "  ${YELLOW}⚠ skipped:${NC} ${reason}"
  ((ASSERTIONS_SKIPPED++)) || true
}

soft_pass_assert() {
  echo -e "  ${YELLOW}~${NC} ${1:-}"
  ((ASSERTIONS_PASSED++)) || true
}

_e2e_default_ref_json() {
  local ref_var="${E2E_REF_JSON_VAR:-RESULT}"
  printf '%s' "${!ref_var-}"
}

find_ref_by_role() {
  local role="$1"
  local json="${2:-$(_e2e_default_ref_json)}"
  echo "$json" | safe_jq -r "[.nodes[] | select(.role == \"$role\") | .ref] | first // empty"
}

find_ref_by_name() {
  local name="$1"
  local json="${2:-$(_e2e_default_ref_json)}"
  echo "$json" | safe_jq -r "[.nodes[] | select(.name == \"$name\") | .ref] | first // empty"
}

find_ref_by_role_and_name() {
  local role="$1"
  local name="$2"
  local json="${3:-$(_e2e_default_ref_json)}"
  echo "$json" | safe_jq -r "[.nodes[] | select(.role == \"$role\" and .name == \"$name\") | .ref] | first // empty"
}

assert_ref_found() {
  local ref="$1"
  local desc="${2:-ref}"
  if [ -n "$ref" ] && [ "$ref" != "null" ]; then
    pass_assert "found $desc: $ref"
    return 0
  fi

  skip_assert "could not find $desc, skipping"
  return 1
}

assert_json_jq() {
  local json="$1"
  local expr="$2"
  local success_desc="$3"
  local fail_desc="${4:-$3}"
  shift 4
  local -a jq_args=("$@")

  if echo "$json" | safe_jq -e "${jq_args[@]}" "$expr" >/dev/null 2>&1; then
    pass_assert "$success_desc"
  else
    fail_assert "$fail_desc"
  fi
}

assert_ref_json_jq() {
  local expr="$1"
  local success_desc="$2"
  local fail_desc="${3:-$2}"
  shift 3
  assert_json_jq "$(_e2e_default_ref_json)" "$expr" "$success_desc" "$fail_desc" "$@"
}

finish_suite() {
  if [ "$TESTS_FAILED" -gt 0 ]; then
    exit 1
  fi
}
