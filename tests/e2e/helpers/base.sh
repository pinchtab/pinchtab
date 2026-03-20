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

E2E_SERVER="${E2E_SERVER:-http://localhost:9999}"
E2E_SECURE_SERVER="${E2E_SECURE_SERVER:-http://localhost:9998}"
E2E_MEDIUM_SERVER="${E2E_MEDIUM_SERVER:-}"
E2E_FULL_SERVER="${E2E_FULL_SERVER:-}"
E2E_BRIDGE_URL="${E2E_BRIDGE_URL:-}"
E2E_SERVER_TOKEN="${E2E_SERVER_TOKEN:-}"
E2E_BRIDGE_TOKEN="${E2E_BRIDGE_TOKEN:-}"
FIXTURES_URL="${FIXTURES_URL:-http://localhost:8080}"
RESULTS_DIR="${RESULTS_DIR:-/results}"

CURRENT_TEST="${CURRENT_TEST:-}"
CURRENT_SCENARIO_FILE="${CURRENT_SCENARIO_FILE:-}"
TESTS_PASSED="${TESTS_PASSED:-0}"
TESTS_FAILED="${TESTS_FAILED:-0}"
ASSERTIONS_PASSED="${ASSERTIONS_PASSED:-0}"
ASSERTIONS_FAILED="${ASSERTIONS_FAILED:-0}"
TEST_START_TIME="${TEST_START_TIME:-0}"
TEST_START_NS="${TEST_START_NS:-0}"
if [ -z "${TEST_RESULTS_INIT:-}" ]; then
  TEST_RESULTS=()
  TEST_RESULTS_INIT=1
fi

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
      inst_status=$(echo "$health_json" | jq -r '.defaultInstance.status // .status // empty' 2>/dev/null || true)
      if [ "$inst_status" = "running" ] || [ "$inst_status" = "ok" ]; then
        echo -e "  ${GREEN}✓${NC} instance ready at ${base_url}"
        return 0
      fi
    fi

    sleep 1
  done
}

start_test() {
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

  if [ "$ASSERTIONS_FAILED" -eq 0 ]; then
    echo -e "${GREEN}✓ ${CURRENT_TEST} passed${NC} ${MUTED}(${duration}ms)${NC}\n"
    TEST_RESULTS+=("✅ ${CURRENT_TEST}|${duration}ms|passed")
    ((TESTS_PASSED++)) || true
  else
    echo -e "${RED}✗ ${CURRENT_TEST} failed${NC} ${MUTED}(${duration}ms)${NC}\n"
    TEST_RESULTS+=("❌ ${CURRENT_TEST}|${duration}ms|failed")
    ((TESTS_FAILED++)) || true
  fi
  ASSERTIONS_PASSED=0
  ASSERTIONS_FAILED=0
}

_e2e_default_ref_json() {
  local ref_var="${E2E_REF_JSON_VAR:-RESULT}"
  printf '%s' "${!ref_var-}"
}

find_ref_by_role() {
  local role="$1"
  local json="${2:-$(_e2e_default_ref_json)}"
  echo "$json" | jq -r "[.nodes[] | select(.role == \"$role\") | .ref] | first // empty"
}

find_ref_by_name() {
  local name="$1"
  local json="${2:-$(_e2e_default_ref_json)}"
  echo "$json" | jq -r "[.nodes[] | select(.name == \"$name\") | .ref] | first // empty"
}

assert_ref_found() {
  local ref="$1"
  local desc="${2:-ref}"
  if [ -n "$ref" ] && [ "$ref" != "null" ]; then
    echo -e "  ${GREEN}✓${NC} found $desc: $ref"
    ((ASSERTIONS_PASSED++)) || true
    return 0
  fi

  echo -e "  ${YELLOW}⚠${NC} could not find $desc, skipping"
  ((ASSERTIONS_PASSED++)) || true
  return 1
}

assert_json_jq() {
  local json="$1"
  local expr="$2"
  local success_desc="$3"
  local fail_desc="${4:-$3}"
  shift 4
  local -a jq_args=("$@")

  if echo "$json" | jq -e "${jq_args[@]}" "$expr" >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} $success_desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $fail_desc"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_ref_json_jq() {
  local expr="$1"
  local success_desc="$2"
  local fail_desc="${3:-$2}"
  shift 3
  assert_json_jq "$(_e2e_default_ref_json)" "$expr" "$success_desc" "$fail_desc" "$@"
}

print_summary() {
  local total=$((TESTS_PASSED + TESTS_FAILED))
  local total_time=0
  local title="${E2E_SUMMARY_TITLE:-E2E Test Summary}"
  local summary_file="${E2E_SUMMARY_FILE:-summary.txt}"
  local report_file="${E2E_REPORT_FILE:-report.md}"

  local name_width=40
  for result in "${TEST_RESULTS[@]}"; do
    IFS='|' read -r name _ _ <<< "$result"
    local len=${#name}
    [ "$len" -gt "$name_width" ] && name_width=$len
  done
  ((name_width += 2)) || true
  local line_width=$((name_width + 24))
  local separator
  separator=$(printf '─%.0s' $(seq 1 $line_width))

  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo -e "${BLUE}${title}${NC}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
  printf "  %-${name_width}s %10s %10s\n" "Test" "Duration" "Status"
  echo "  ${separator}"

  for result in "${TEST_RESULTS[@]}"; do
    IFS='|' read -r name duration status <<< "$result"
    local time_num=${duration%ms}
    ((total_time += time_num)) || true
    if [ "$status" = "passed" ]; then
      printf "  %-${name_width}s %10s ${GREEN}%10s${NC}\n" "$name" "$duration" "✓"
    else
      printf "  %-${name_width}s %10s ${RED}%10s${NC}\n" "$name" "$duration" "✗"
    fi
  done

  echo "  ${separator}"
  printf "  %-${name_width}s %10s\n" "Total" "${total_time}ms"
  echo ""
  echo -e "  ${GREEN}Passed:${NC} ${TESTS_PASSED}/${total}"
  echo -e "  ${RED}Failed:${NC} ${TESTS_FAILED}/${total}"
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

  if [ -d "${RESULTS_DIR:-}" ]; then
    echo "passed=$TESTS_PASSED" > "${RESULTS_DIR}/${summary_file}"
    echo "failed=$TESTS_FAILED" >> "${RESULTS_DIR}/${summary_file}"
    echo "total_time=${total_time}ms" >> "${RESULTS_DIR}/${summary_file}"
    echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${RESULTS_DIR}/${summary_file}"

    if [ "${E2E_GENERATE_MARKDOWN_REPORT:-0}" = "1" ] && declare -F generate_markdown_report >/dev/null 2>&1; then
      generate_markdown_report > "${RESULTS_DIR}/${report_file}"
    fi
  fi

  if [ "$TESTS_FAILED" -gt 0 ]; then
    exit 1
  fi
}
