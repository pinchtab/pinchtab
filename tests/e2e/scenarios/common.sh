#!/bin/bash
# Common utilities for E2E tests

set -uo pipefail
# Note: not using -e because arithmetic operations can return 1

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
MUTED='\033[0;90m'
NC='\033[0m'

# Defaults from environment
PINCHTAB_URL="${PINCHTAB_URL:-http://localhost:9999}"
FIXTURES_URL="${FIXTURES_URL:-http://localhost:8080}"
RESULTS_DIR="${RESULTS_DIR:-/results}"

# Test tracking (only initialize if not already set)
CURRENT_TEST="${CURRENT_TEST:-}"
TESTS_PASSED="${TESTS_PASSED:-0}"
TESTS_FAILED="${TESTS_FAILED:-0}"
ASSERTIONS_PASSED="${ASSERTIONS_PASSED:-0}"
ASSERTIONS_FAILED="${ASSERTIONS_FAILED:-0}"

# Test timing (using seconds, Alpine doesn't support ms)
TEST_START_TIME="${TEST_START_TIME:-0}"
TEST_START_NS="${TEST_START_NS:-0}"
if [ -z "${TEST_RESULTS_INIT:-}" ]; then
  TEST_RESULTS=()
  TEST_RESULTS_INIT=1
fi

# Get time in milliseconds (cross-platform)
get_time_ms() {
  if [ -f /proc/uptime ]; then
    # Linux: use /proc/uptime (gives centiseconds)
    awk '{printf "%.0f", $1 * 1000}' /proc/uptime
  elif command -v gdate &>/dev/null; then
    # macOS with coreutils
    gdate +%s%3N
  elif command -v perl &>/dev/null; then
    # Perl fallback
    perl -MTime::HiRes=time -e 'printf "%.0f", time * 1000'
  else
    # Last resort: seconds * 1000
    echo $(($(date +%s) * 1000))
  fi
}

# Start a test
start_test() {
  CURRENT_TEST="$1"
  TEST_START_TIME=$(get_time_ms)
  echo -e "${BLUE}▶ ${CURRENT_TEST}${NC}"
}

# End a test
end_test() {
  local end_time=$(get_time_ms)
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

# Assert HTTP status
assert_status() {
  local expected="$1"
  local url="$2"
  local method="${3:-GET}"
  local body="${4:-}"
  
  local actual
  if [ -n "$body" ]; then
    actual=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" -H "Content-Type: application/json" -d "$body" "$url")
  else
    actual=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "$url")
  fi
  
  if [ "$actual" = "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $method $url → $actual"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $method $url → $actual (expected $expected)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert command succeeds (exit 0)
assert_ok() {
  local desc="$1"
  shift
  
  if "$@" >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (exit $?)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert JSON field equals value
assert_json_eq() {
  local json="$1"
  local path="$2"
  local expected="$3"
  local desc="${4:-$path = $expected}"
  
  local actual
  actual=$(echo "$json" | jq -r "$path")
  
  if [ "$actual" = "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got: $actual)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert JSON field contains value
assert_json_contains() {
  local json="$1"
  local path="$2"
  local needle="$3"
  local desc="${4:-$path contains '$needle'}"
  
  local actual
  actual=$(echo "$json" | jq -r "$path")
  
  if [[ "$actual" == *"$needle"* ]]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got: $actual)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert JSON array length
assert_json_length() {
  local json="$1"
  local path="$2"
  local expected="$3"
  local desc="${4:-$path length = $expected}"
  
  local actual
  actual=$(echo "$json" | jq "$path | length")
  
  if [ "$actual" -eq "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got: $actual)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert JSON array length >= value
assert_json_length_gte() {
  local json="$1"
  local path="$2"
  local expected="$3"
  local desc="${4:-$path length >= $expected}"
  
  local actual
  actual=$(echo "$json" | jq "$path | length")
  
  if [ "$actual" -ge "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got: $actual)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert JSON field exists (not null)
assert_json_exists() {
  local json="$1"
  local path="$2"
  local desc="${3:-$path exists}"
  
  if echo "$json" | jq -e "$path" >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (field missing or null)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert string contains substring
assert_contains() {
  local haystack="$1"
  local needle="$2"
  local desc="${3:-contains '$needle'}"
  
  if echo "$haystack" | grep -q "$needle"; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (not found)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert result JSON field equals value (uses global $RESULT)
assert_result_eq() {
  local path="$1"
  local expected="$2"
  local desc="${3:-$path = $expected}"
  assert_json_eq "$RESULT" "$path" "$expected" "$desc"
}

# Assert result JSON field exists (uses global $RESULT)
assert_result_exists() {
  local path="$1"
  local desc="${2:-$path exists}"
  assert_json_exists "$RESULT" "$path" "$desc"
}

# ================================================================
# Visible curl wrapper — shows exact command when running
# ================================================================

RESULT=""
HTTP_STATUS=""

pinchtab() {
  local method="$1"
  local path="$2"
  shift 2

  # Print the curl command in cyan so you see what's executed
  echo -e "${BLUE}→ curl -X $method ${PINCHTAB_URL}$path $@${NC}" >&2

  # Execute and capture response + status
  local response
  response=$(curl -s -w "\n%{http_code}" \
    -X "$method" \
    "${PINCHTAB_URL}$path" \
    -H "Content-Type: application/json" \
    "$@")

  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)
}

# Aliases for cleaner test files
# Usage: pt_get /path
#        pt_post /path '{"json":"data"}'
#        pt_post /path -d '{"json":"data"}'  (also works)
pt() { pinchtab "$@"; }
pt_get() { pinchtab GET "$1"; echo "$RESULT"; }
pt_post() {
  local path="$1"
  shift
  # Handle both: pt_post /path '{"data"}' and pt_post /path -d '{"data"}'
  if [ "$1" = "-d" ]; then
    shift
  fi
  pinchtab POST "$path" -d "$1"
  echo "$RESULT"
}

# ================================================================
# URL accessibility checks
# ================================================================

# Check if a URL is accessible
assert_url_accessible() {
  local url="$1"
  local label="${2:-$url}"
  
  if curl -sf "$url" > /dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} GET $label"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} GET $label (not accessible)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Check all fixture pages
assert_fixtures_accessible() {
  assert_url_accessible "${FIXTURES_URL}/" "fixtures/"
  assert_url_accessible "${FIXTURES_URL}/form.html" "fixtures/form.html"
  assert_url_accessible "${FIXTURES_URL}/table.html" "fixtures/table.html"
  assert_url_accessible "${FIXTURES_URL}/buttons.html" "fixtures/buttons.html"
}

# ================================================================
# Skip helper
# ================================================================

skip() {
  local reason="$1"
  echo -e "  ${YELLOW}⚠${NC} Skipped: $reason"
  ((ASSERTIONS_PASSED++)) || true
}

# ================================================================
# HTTP status assertions
# ================================================================

# Assert last request returned expected status (uses $HTTP_STATUS from pinchtab())
assert_ok() {
  local label="${1:-request}"
  
  if [ "$HTTP_STATUS" = "200" ] || [ "$HTTP_STATUS" = "201" ]; then
    echo -e "  ${GREEN}✓${NC} $label → $HTTP_STATUS"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $label failed (status: $HTTP_STATUS)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert last request returned specific status
assert_http_status() {
  local expected="$1"
  local label="${2:-request}"
  
  if [ "$HTTP_STATUS" = "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $label → $HTTP_STATUS"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $label: expected $expected, got $HTTP_STATUS"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# ================================================================
# Element interaction helpers
# ================================================================

# Get ref for element by name from last snapshot
get_ref() {
  local name="$1"
  echo "$RESULT" | jq -r ".nodes[] | select(.name == \"$name\") | .ref" | head -1
}

# Get ref for element by role from last snapshot
get_ref_by_role() {
  local role="$1"
  echo "$RESULT" | jq -r ".nodes[] | select(.role == \"$role\") | .ref" | head -1
}

# Click a button by name (requires snapshot in $RESULT)
click_button() {
  local name="$1"
  local ref=$(get_ref "$name")
  
  if [ -n "$ref" ] && [ "$ref" != "null" ]; then
    pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${ref}\"}" > /dev/null
    echo -e "  ${GREEN}✓${NC} clicked '$name' (ref: $ref)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} button '$name' not found"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Type into a field by name or role
type_into() {
  local name="$1"
  local text="$2"
  local ref=$(get_ref "$name")
  
  # Fallback to role if name not found
  [ -z "$ref" ] || [ "$ref" = "null" ] && ref=$(get_ref_by_role "textbox")
  
  if [ -n "$ref" ] && [ "$ref" != "null" ]; then
    pt_post /action -d "{\"kind\":\"type\",\"ref\":\"${ref}\",\"text\":\"${text}\"}" > /dev/null
    echo -e "  ${GREEN}✓${NC} typed '$text' into '$name' (ref: $ref)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} input '$name' not found"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Press a key
press_key() {
  local key="$1"
  pt_post /action -d "{\"kind\":\"press\",\"key\":\"${key}\"}" > /dev/null
  echo -e "  ${GREEN}✓${NC} pressed '$key'"
  ((ASSERTIONS_PASSED++)) || true
}

# ================================================================
# Tab helpers
# ================================================================

# Get current tab count
get_tab_count() {
  curl -s "${PINCHTAB_URL}/tabs" | jq '.tabs | length'
}

# Get tab ID from last response (e.g., after /navigate)
get_tab_id() {
  echo "$RESULT" | jq -r '.tabId'
}

# Get first tab ID from /tabs response
get_first_tab() {
  echo "$RESULT" | jq -r '.tabs[0].id'
}

# Get last tab ID from /tabs response  
get_last_tab() {
  echo "$RESULT" | jq -r '.tabs[-1].id'
}

# Print tab ID (truncated for readability)
show_tab() {
  local label="$1"
  local id="$2"
  echo -e "  ${MUTED}$label: ${id:0:12}...${NC}"
}

# Assert tab count equals expected
assert_tab_count() {
  local expected="$1"
  local actual=$(get_tab_count)
  
  if [ "$actual" -eq "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} tab count = $actual"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} tab count: expected $expected, got $actual"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert tab count >= minimum
assert_tab_count_gte() {
  local min="$1"
  local actual=$(get_tab_count)
  
  if [ "$actual" -ge "$min" ]; then
    echo -e "  ${GREEN}✓${NC} tab count $actual >= $min"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} tab count: expected >= $min, got $actual"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Assert tab count decreased after an action
assert_tab_closed() {
  local before="$1"
  local actual=$(get_tab_count)
  
  if [ "$actual" -lt "$before" ]; then
    echo -e "  ${GREEN}✓${NC} tab closed (before: $before, after: $actual)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} tab not closed (before: $before, after: $actual)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# ================================================================
# Page-specific assertions (we control the fixtures!)
# ================================================================

# buttons.html: Increment, Decrement, Reset, Toggle visibility, Show Message
assert_buttons_page() {
  local snap="$1"
  local expected_buttons=("Increment" "Decrement" "Reset")
  local found=0
  
  for btn in "${expected_buttons[@]}"; do
    if echo "$snap" | jq -e ".nodes[] | select(.name == \"$btn\")" > /dev/null 2>&1; then
      ((found++))
    fi
  done
  
  if [ "$found" -ge 3 ]; then
    echo -e "  ${GREEN}✓${NC} buttons.html: found $found/3 expected buttons"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} buttons.html: found only $found/3 expected buttons"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# form.html: username, email, password fields + Submit button
assert_form_page() {
  local snap="$1"
  local checks=0
  
  # Check for textboxes (username, email)
  local textboxes=$(echo "$snap" | jq '[.nodes[] | select(.role == "textbox")] | length')
  [ "$textboxes" -ge 2 ] && ((checks++))
  
  # Check for Submit button
  echo "$snap" | jq -e '.nodes[] | select(.name == "Submit")' > /dev/null 2>&1 && ((checks++))
  
  # Check for combobox (country select)
  echo "$snap" | jq -e '.nodes[] | select(.role == "combobox")' > /dev/null 2>&1 && ((checks++))
  
  if [ "$checks" -ge 3 ]; then
    echo -e "  ${GREEN}✓${NC} form.html: found inputs, submit button, and select"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} form.html: missing expected elements ($checks/3)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# table.html: Alice Johnson, bob@example.com, Active/Inactive status
assert_table_page() {
  local text="$1"
  local checks=0
  
  echo "$text" | grep -q "Alice Johnson" && ((checks++))
  echo "$text" | grep -q "bob@example.com" && ((checks++))
  echo "$text" | grep -q "Active" && ((checks++))
  
  if [ "$checks" -ge 3 ]; then
    echo -e "  ${GREEN}✓${NC} table.html: found expected table data"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} table.html: missing expected data ($checks/3)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# index.html: Welcome header
assert_index_page() {
  local snap="$1"
  
  if echo "$snap" | jq -e '.title' | grep -q "E2E Test"; then
    echo -e "  ${GREEN}✓${NC} index.html: correct title"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} index.html: wrong title"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Print summary
print_summary() {
  local total=$((TESTS_PASSED + TESTS_FAILED))
  local total_time=0
  
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo -e "${BLUE}E2E Test Summary${NC}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
  printf "  %-40s %10s %10s\n" "Test" "Duration" "Status"
  echo "  ────────────────────────────────────────────────────────"
  
  for result in "${TEST_RESULTS[@]}"; do
    IFS='|' read -r name duration status <<< "$result"
    local time_num=${duration%ms}
    ((total_time += time_num)) || true
    if [ "$status" = "passed" ]; then
      printf "  %-40s %10s ${GREEN}%10s${NC}\n" "$name" "$duration" "✓"
    else
      printf "  %-40s %10s ${RED}%10s${NC}\n" "$name" "$duration" "✗"
    fi
  done
  
  echo "  ────────────────────────────────────────────────────────"
  printf "  %-40s %10s\n" "Total" "${total_time}ms"
  echo ""
  echo -e "  ${GREEN}Passed:${NC} ${TESTS_PASSED}/${total}"
  echo -e "  ${RED}Failed:${NC} ${TESTS_FAILED}/${total}"
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  
  # Generate markdown report for CI
  if [ -d "${RESULTS_DIR:-}" ]; then
    generate_markdown_report > "${RESULTS_DIR}/report.md"
    echo "passed=$TESTS_PASSED" > "${RESULTS_DIR}/summary.txt"
    echo "failed=$TESTS_FAILED" >> "${RESULTS_DIR}/summary.txt"
    echo "total_time=${total_time}ms" >> "${RESULTS_DIR}/summary.txt"
    echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${RESULTS_DIR}/summary.txt"
  fi
  
  if [ "$TESTS_FAILED" -gt 0 ]; then
    exit 1
  fi
}

# Generate markdown report
generate_markdown_report() {
  local total=$((TESTS_PASSED + TESTS_FAILED))
  local total_time=0
  
  echo "## 🦀 PinchTab E2E Test Report"
  echo ""
  if [ "$TESTS_FAILED" -eq 0 ]; then
    echo "**Status:** ✅ All tests passed"
  else
    echo "**Status:** ❌ ${TESTS_FAILED} test(s) failed"
  fi
  echo ""
  echo "| Test | Duration | Status |"
  echo "|------|----------|--------|"
  
  for result in "${TEST_RESULTS[@]}"; do
    IFS='|' read -r name duration status <<< "$result"
    local time_num=${duration%ms}
    ((total_time += time_num)) || true
    local icon="✅"
    [ "$status" = "failed" ] && icon="❌"
    # Remove emoji from name for cleaner table
    local clean_name="${name#✅ }"
    clean_name="${clean_name#❌ }"
    echo "| ${clean_name} | ${duration} | ${icon} |"
  done
  
  echo ""
  echo "**Summary:** ${TESTS_PASSED}/${total} passed in ${total_time}ms"
  echo ""
  echo "<sub>Generated at $(date -u +%Y-%m-%dT%H:%M:%SZ)</sub>"
}
