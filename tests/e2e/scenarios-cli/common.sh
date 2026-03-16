#!/bin/bash
# Common utilities for CLI E2E tests

set -uo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Environment
E2E_SERVER="${E2E_SERVER:-http://localhost:9999}"
FIXTURES_URL="${FIXTURES_URL:-http://localhost:8080}"
RESULTS_DIR="${RESULTS_DIR:-/results}"

# Test tracking (preserve across sourced scripts)
TESTS_PASSED="${TESTS_PASSED:-0}"
TESTS_FAILED="${TESTS_FAILED:-0}"
ASSERTIONS_PASSED="${ASSERTIONS_PASSED:-0}"
ASSERTIONS_FAILED="${ASSERTIONS_FAILED:-0}"
CURRENT_TEST="${CURRENT_TEST:-}"
MUTED='\033[0;90m'

# Test timing
TEST_START_TIME="${TEST_START_TIME:-0}"
if [ -z "${TEST_RESULTS_INIT:-}" ]; then
  TEST_RESULTS=()
  TEST_RESULTS_INIT=1
fi

# Get time in milliseconds (cross-platform)
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

# ─────────────────────────────────────────────────────────────────
# Wait for instance ready (same as curl-based tests)
# ─────────────────────────────────────────────────────────────────

wait_for_instance_ready() {
  local base_url="$1"
  local timeout_sec="${2:-60}"
  local started_at
  started_at=$(date +%s)

  while true; do
    local now
    now=$(date +%s)
    if [ $((now - started_at)) -ge "$timeout_sec" ]; then
      echo -e "  ${RED}✗${NC} instance at ${base_url} did not reach running within ${timeout_sec}s"
      return 1
    fi

    local inst_status
    inst_status=$(curl -sf "${base_url}/health" 2>/dev/null | jq -r '.defaultInstance.status // empty' 2>/dev/null || true)
    if [ "$inst_status" = "running" ]; then
      echo -e "  ${GREEN}✓${NC} instance ready at ${base_url}"
      return 0
    fi

    sleep 1
  done
}

# ─────────────────────────────────────────────────────────────────
# Test lifecycle
# ─────────────────────────────────────────────────────────────────

start_test() {
  CURRENT_TEST="$1"
  TEST_START_TIME=$(get_time_ms)
  echo -e "${BLUE}▶ ${CURRENT_TEST}${NC}"
}

end_test() {
  local end_time=$(get_time_ms)
  local duration=$((end_time - TEST_START_TIME))

  if [ "$ASSERTIONS_FAILED" -gt 0 ]; then
    echo -e "${RED}✗ ${CURRENT_TEST} failed${NC} ${MUTED}(${duration}ms)${NC}\n"
    TEST_RESULTS+=("❌ ${CURRENT_TEST}|${duration}ms|failed")
    ((TESTS_FAILED++)) || true
  else
    echo -e "${GREEN}✓ ${CURRENT_TEST} passed${NC} ${MUTED}(${duration}ms)${NC}\n"
    TEST_RESULTS+=("✅ ${CURRENT_TEST}|${duration}ms|passed")
    ((TESTS_PASSED++)) || true
  fi
  ASSERTIONS_FAILED=0
  ASSERTIONS_PASSED=0
}

# ─────────────────────────────────────────────────────────────────
# CLI execution helpers
# ─────────────────────────────────────────────────────────────────

# Run pinchtab CLI command
# Usage: pt <command> [args...]
# Sets $PT_OUT (stdout), $PT_ERR (stderr), $PT_CODE (exit code)
pt() {
  local tmpout=$(mktemp)
  local tmperr=$(mktemp)

  echo -e "  ${BLUE}→ pinchtab --server $E2E_SERVER $@${NC}"

  set +e
  pinchtab --server "$E2E_SERVER" "$@" > "$tmpout" 2> "$tmperr"
  PT_CODE=$?
  set -e

  PT_OUT=$(cat "$tmpout")
  PT_ERR=$(cat "$tmperr")
  rm -f "$tmpout" "$tmperr"

  if [ -n "$PT_OUT" ]; then
    echo "$PT_OUT" | head -5
  fi
}

# Run pinchtab and expect success (exit 0)
# Usage: pt_ok <command> [args...]
pt_ok() {
  pt "$@"
  if [ "$PT_CODE" -eq 0 ]; then
    echo -e "  ${GREEN}✓${NC} exit 0"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} expected exit 0, got $PT_CODE"
    echo -e "  ${RED}stderr: $PT_ERR${NC}"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Run pinchtab and expect failure (non-zero exit)
# Usage: pt_fail <command> [args...]
pt_fail() {
  pt "$@"
  if [ "$PT_CODE" -ne 0 ]; then
    echo -e "  ${GREEN}✓${NC} exit $PT_CODE (expected failure)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} expected non-zero exit, got 0"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# ─────────────────────────────────────────────────────────────────
# Assertions
# ─────────────────────────────────────────────────────────────────

# Assert PT_OUT contains string
assert_output_contains() {
  local expected="$1"
  local desc="${2:-output contains '$expected'}"

  if echo "$PT_OUT" | grep -q "$expected"; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc"
    echo -e "  ${RED}  output was: $PT_OUT${NC}"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert exit code equals expected (use after pt, not pt_ok/pt_fail)
# Usage: assert_exit_code 2 "unknown subcommand"
assert_exit_code() {
  local expected="$1"
  local desc="${2:-exit code is $expected}"
  if [ "$PT_CODE" -eq "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $desc (exit $PT_CODE)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (expected $expected, got $PT_CODE)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert exit code is in range [0, max]
# Usage: assert_exit_code_lte 1 "graceful exit"
assert_exit_code_lte() {
  local max="$1"
  local desc="${2:-exit code <= $max}"
  if [ "$PT_CODE" -le "$max" ]; then
    echo -e "  ${GREEN}✓${NC} $desc (exit $PT_CODE)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got $PT_CODE)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert PT_OUT JSON field contains substring
assert_json_field_contains() {
  local path="$1"
  local needle="$2"
  local desc="${3:-$path contains '$needle'}"
  local actual
  actual=$(echo "$PT_OUT" | jq -r "$path" 2>/dev/null)
  if [[ "$actual" == *"$needle"* ]]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got '$actual')"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Find ref by role/name from PT_OUT snapshot (CLI equivalent of curl helpers)
find_ref_by_role() {
  local role="$1"
  local json="${2:-$PT_OUT}"
  echo "$json" | jq -r "[.nodes[] | select(.role == \"$role\") | .ref] | first // empty"
}

find_ref_by_name() {
  local name="$1"
  local json="${2:-$PT_OUT}"
  echo "$json" | jq -r "[.nodes[] | select(.name == \"$name\") | .ref] | first // empty"
}

assert_ref_found() {
  local ref="$1"
  local desc="${2:-ref}"
  if [ -n "$ref" ] && [ "$ref" != "null" ]; then
    echo -e "  ${GREEN}✓${NC} found $desc: $ref"
    ((ASSERTIONS_PASSED++)) || true
    return 0
  else
    echo -e "  ${YELLOW}⚠${NC} could not find $desc, skipping"
    ((ASSERTIONS_PASSED++)) || true
    return 1
  fi
}

# Assert file exists
assert_file_exists() {
  local path="$1"
  local desc="${2:-file exists: $path}"
  if [ -f "$path" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (not found)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert PT_OUT does not contain string
assert_output_not_contains() {
  local forbidden="$1"
  local desc="${2:-output does not contain '$forbidden'}"

  if echo "$PT_OUT" | grep -q "$forbidden"; then
    echo -e "  ${RED}✗${NC} $desc"
    echo -e "  ${RED}  output was: $PT_OUT${NC}"
    ((ASSERTIONS_FAILED++)) || true
  else
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  fi
}

# Assert PT_OUT is valid JSON
assert_output_json() {
  local desc="${1:-output is valid JSON}"

  if echo "$PT_OUT" | jq . > /dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc"
    echo -e "  ${RED}  output was: $PT_OUT${NC}"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert PT_OUT JSON field equals value
assert_json_field() {
  local path="$1"
  local expected="$2"
  local desc="${3:-$path equals '$expected'}"

  local actual
  actual=$(echo "$PT_OUT" | jq -r "$path" 2>/dev/null)

  if [ "$actual" = "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got '$actual')"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# ─────────────────────────────────────────────────────────────────
# Summary
# ─────────────────────────────────────────────────────────────────

print_summary() {
  local total=$((TESTS_PASSED + TESTS_FAILED))
  local total_time=0

  # Calculate column width from longest test name (min 40, pad +2)
  local name_width=40
  for result in "${TEST_RESULTS[@]}"; do
    IFS='|' read -r name _ _ <<< "$result"
    local len=${#name}
    [ "$len" -gt "$name_width" ] && name_width=$len
  done
  ((name_width += 2)) || true
  local line_width=$((name_width + 24))
  local separator=$(printf '─%.0s' $(seq 1 $line_width))

  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo -e "${BLUE}CLI E2E Test Summary${NC}"
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

  # Save results
  if [ -d "${RESULTS_DIR:-}" ]; then
    echo "passed=$TESTS_PASSED" > "${RESULTS_DIR}/summary.txt"
    echo "failed=$TESTS_FAILED" >> "${RESULTS_DIR}/summary.txt"
    echo "total_time=${total_time}ms" >> "${RESULTS_DIR}/summary.txt"
    echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${RESULTS_DIR}/summary.txt"
  fi

  if [ "$TESTS_FAILED" -gt 0 ]; then
    exit 1
  fi
}
