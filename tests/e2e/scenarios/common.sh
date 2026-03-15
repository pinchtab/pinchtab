#!/bin/bash
# Common utilities for E2E tests

set -uo pipefail
# Note: not using -e because arithmetic operations can return 1

# Colors
RED='\033[0;31m'
ERROR="$RED"
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
MUTED='\033[0;90m'
BOLD='\033[1m'
NC='\033[0m'

# Defaults from environment
PINCHTAB_URL="${PINCHTAB_URL:-http://localhost:9999}"
PINCHTAB_SECURE_URL="${PINCHTAB_SECURE_URL:-http://localhost:9998}"
PINCHTAB_BRIDGE_URL="${PINCHTAB_BRIDGE_URL:-}"
PINCHTAB_BRIDGE_TOKEN="${PINCHTAB_BRIDGE_TOKEN:-}"
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

wait_for_instance_ready() {
  local base_url="$1"
  local timeout_sec="${2:-60}"
  local token="${3:-}"
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
    if [ -n "$token" ]; then
      health_json=$(curl -sf -H "Authorization: Bearer ${token}" "${base_url}/health" 2>/dev/null || true)
    else
      health_json=$(curl -sf "${base_url}/health" 2>/dev/null || true)
    fi
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

wait_for_orchestrator_instance_status() {
  local base_url="$1"
  local instance_id="$2"
  local wanted_status="${3:-running}"
  local timeout_sec="${4:-60}"
  local started_at
  started_at=$(date +%s)

  while true; do
    local now
    now=$(date +%s)
    if [ $((now - started_at)) -ge "$timeout_sec" ]; then
      echo -e "  ${RED}✗${NC} instance ${instance_id} at ${base_url} did not reach ${wanted_status} within ${timeout_sec}s"
      return 1
    fi

    local inst_json
    inst_json=$(curl -sf "${base_url}/instances/${instance_id}" 2>/dev/null || true)
    if [ -n "$inst_json" ]; then
      local inst_status
      inst_status=$(echo "$inst_json" | jq -r '.status // empty' 2>/dev/null || true)
      if [ "$inst_status" = "$wanted_status" ]; then
        echo -e "  ${GREEN}✓${NC} instance ${instance_id} is ${wanted_status}"
        return 0
      fi
      if [ "$inst_status" = "stopped" ] || [ "$inst_status" = "error" ]; then
        echo -e "  ${RED}✗${NC} instance ${instance_id} reached terminal status ${inst_status} before ${wanted_status}"
        return 1
      fi
    fi

    sleep 1
  done
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

# Assert input field value does NOT contain text (case-insensitive)
# Usage: assert_input_not_contains "#username" "Enter" "username should not contain Enter"
assert_input_not_contains() {
  local selector="$1"
  local forbidden="$2"
  local desc="${3:-$selector should not contain '$forbidden'}"

  local json_body
  json_body=$(jq -n --arg sel "$selector" '{"expression": ("document.querySelector(\"" + $sel + "\")?.value || \"\"")}')
  pt_post /evaluate "$json_body"
  local value
  value=$(echo "$RESULT" | jq -r '.result // empty')

  if echo "$value" | grep -qi "$forbidden"; then
    echo -e "  ${RED}✗${NC} $desc: found '$forbidden' in value '$value'"
    ((ASSERTIONS_FAILED++)) || true
    return 1
  else
    echo -e "  ${GREEN}✓${NC} $desc (value: '$value')"
    ((ASSERTIONS_PASSED++)) || true
    return 0
  fi
}

# Assert HTTP error status from $RESULT
# Usage: assert_http_error 400 "error message pattern"
assert_http_error() {
  local expected_status="$1"
  local error_pattern="${2:-error}"
  local desc="${3:-HTTP $expected_status error}"

  local actual_status
  actual_status=$(echo "$RESULT" | jq -r '.status // empty')

  if [ "$actual_status" = "$expected_status" ] || grep -q "$error_pattern" <<< "$RESULT"; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${YELLOW}~${NC} $desc (got: $actual_status)"
    ((ASSERTIONS_PASSED++)) || true
  fi
}

# Assert result contains one of multiple patterns
# Usage: assert_contains_any "$RESULT" "pattern1|pattern2" "description"
assert_contains_any() {
  local haystack="$1"
  local patterns="$2"  # pipe-separated
  local desc="${3:-contains expected pattern}"

  if echo "$haystack" | grep -qE "$patterns"; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${YELLOW}~${NC} $desc (not found)"
    ((ASSERTIONS_PASSED++)) || true
  fi
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
  # Using printf %q to better show quoted arguments
  echo -e "${BLUE}→ curl -X $method ${PINCHTAB_URL}$path $(printf "%q " "$@")${NC}" >&2

  # Execute and capture response + status
  local response
  response=$(curl -s -w "\n%{http_code}" \
    -X "$method" \
    "${PINCHTAB_URL}$path" \
    -H "Content-Type: application/json" \
    "$@")

  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)

  # Log response body on failure for debugging
  if [[ ! "$HTTP_STATUS" =~ ^2 ]]; then
    echo -e "${ERROR}  HTTP $HTTP_STATUS: $RESULT${NC}" >&2
  fi
}

# Aliases for cleaner test files
# Usage: pt_get /path
#        pt_post /path '{"json":"data"}'
#        pt_post /path -d '{"json":"data"}'  (also works)
pt() { pinchtab "$@"; }

# Truncate output for display (avoid flooding logs with base64 blobs)
_echo_truncated() {
  if [ ${#RESULT} -gt 1000 ]; then
    echo "${RESULT:0:200}...[truncated ${#RESULT} chars]"
  else
    echo "$RESULT"
  fi
}

pt_get() { pinchtab GET "$1"; _echo_truncated; }
pt_post() {
  local path="$1"
  shift
  # Handle both: pt_post /path '{"data"}' and pt_post /path -d '{"data"}'
  if [ "$1" = "-d" ]; then
    shift
  fi
  pinchtab POST "$path" -d "$1"
  _echo_truncated
}

pt_patch() {
  local path="$1"
  local body="$2"
  echo -e "${BLUE}→ curl -X PATCH ${PINCHTAB_URL}$path${NC}" >&2
  local response
  response=$(curl -s -w "\n%{http_code}" \
    -X PATCH \
    "${PINCHTAB_URL}$path" \
    -H "Content-Type: application/json" \
    -d "$body")
  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)
  _echo_truncated
}

pt_delete() {
  local path="$1"
  echo -e "${BLUE}→ curl -X DELETE ${PINCHTAB_URL}$path${NC}" >&2
  local response
  response=$(curl -s -w "\n%{http_code}" \
    -X DELETE \
    "${PINCHTAB_URL}$path")
  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)
  _echo_truncated
}

# POST raw body (for testing malformed JSON)
pt_post_raw() {
  local path="$1"
  local body="$2"
  echo -e "${BLUE}→ curl -X POST ${PINCHTAB_URL}$path -d '$body'${NC}" >&2
  local response
  response=$(curl -s -w "\n%{http_code}" \
    -X POST \
    "${PINCHTAB_URL}$path" \
    -H "Content-Type: application/json" \
    -d "$body")
  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)
  _echo_truncated
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

# Assert last request returned non-200 status (error expected)
assert_not_ok() {
  local label="${1:-request}"

  if [ "$HTTP_STATUS" != "200" ] && [ "$HTTP_STATUS" != "201" ]; then
    echo -e "  ${GREEN}✓${NC} $label → $HTTP_STATUS (error expected)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $label: expected error, got $HTTP_STATUS"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# ================================================================
# Element interaction helpers
# ================================================================

# Click a button by name (requires snapshot in $RESULT)
click_button() {
  local name="$1"
  local ref
  ref=$(echo "$RESULT" | jq -r "[.nodes[] | select(.name == \"$name\") | .ref] | first // empty")

  if [ -n "$ref" ] && [ "$ref" != "null" ]; then
    pt_post /action "{\"kind\":\"click\",\"ref\":\"${ref}\"}" > /dev/null
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
  local ref
  ref=$(echo "$RESULT" | jq -r "[.nodes[] | select(.name == \"$name\") | .ref] | first // empty")

  # Fallback to textbox role if name not found
  if [ -z "$ref" ] || [ "$ref" = "null" ]; then
    ref=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "textbox") | .ref] | first // empty')
  fi

  if [ -n "$ref" ] && [ "$ref" != "null" ]; then
    pt_post /action "{\"kind\":\"type\",\"ref\":\"${ref}\",\"text\":\"${text}\"}" > /dev/null
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

# Extract tabId from RESULT, assert it's non-null, store in TAB_ID
# Usage: assert_tab_id "description"
assert_tab_id() {
  local desc="${1:-tabId returned}"
  TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
  if [ -n "$TAB_ID" ] && [ "$TAB_ID" != "null" ]; then
    echo -e "  ${GREEN}✓${NC} $desc: ${TAB_ID:0:12}..."
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} no tabId in response"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# Get first tab ID from /tabs response
get_first_tab() {
  echo "$RESULT" | jq -r '.tabs[0].id'
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
# Ref extraction from snapshot
# ================================================================

# Find first ref by role from RESULT (curl) or PT_OUT (CLI)
# Usage: REF=$(find_ref_by_role "button")
find_ref_by_role() {
  local role="$1"
  local json="${2:-$RESULT}"
  echo "$json" | jq -r "[.nodes[] | select(.role == \"$role\") | .ref] | first // empty"
}

# Find first ref by name from RESULT or PT_OUT
# Usage: REF=$(find_ref_by_name "Username:")
find_ref_by_name() {
  local name="$1"
  local json="${2:-$RESULT}"
  echo "$json" | jq -r "[.nodes[] | select(.name == \"$name\") | .ref] | first // empty"
}

# Assert ref was found, with readable error
# Usage: assert_ref_found "$REF" "button ref"
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

# ================================================================
# Evaluate with polling (for stealth/async injection)
# ================================================================

# Poll an expression up to N times, assert result equals expected.
# If a tab ID is provided, evaluate in that exact tab instead of the implicit current tab.
# Usage: assert_eval_poll "navigator.webdriver === undefined" "true" "webdriver is undefined" [attempts] [delay] [tab_id]
assert_eval_poll() {
  local expr="$1"
  local expected="$2"
  local desc="${3:-eval poll}"
  local attempts="${4:-5}"
  local delay="${5:-0.4}"
  local tab_id="${6:-}"

  local ok=false
  for i in $(seq 1 "$attempts"); do
    local json_body
    if [ -n "$tab_id" ]; then
      json_body=$(jq -n --arg expr "$expr" --arg tabId "$tab_id" '{"expression": $expr, "tabId": $tabId}')
    else
      json_body=$(jq -n --arg expr "$expr" '{"expression": $expr}')
    fi
    pt_post /evaluate "$json_body"
    local actual
    actual=$(echo "$RESULT" | jq -r '.result // empty' 2>/dev/null)
    if [ "$actual" = "$expected" ]; then
      ok=true
      break
    fi
    sleep "$delay"
  done

  if [ "$ok" = "true" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
    return 0
  else
    echo -e "  ${RED}✗${NC} $desc (got: $actual, expected: $expected)"
    ((ASSERTIONS_FAILED++)) || true
    return 1
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
