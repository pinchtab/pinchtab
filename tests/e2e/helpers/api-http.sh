#!/bin/bash
# HTTP/API helpers for curl-based E2E tests.

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
    inst_json=$(e2e_curl -sf "${base_url}/instances/${instance_id}" 2>/dev/null || true)
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

wait_for_instances_gone() {
  local base_url="$1"
  local timeout_sec="${2:-10}"
  shift 2
  local -a instance_ids=("$@")
  local started_at
  started_at=$(date +%s)

  while true; do
    local remaining=0
    for instance_id in "${instance_ids[@]}"; do
      local inst_json
      inst_json=$(e2e_curl -sf "${base_url}/instances/${instance_id}" 2>/dev/null || true)
      if [ -n "$inst_json" ]; then
        local inst_status
        inst_status=$(echo "$inst_json" | jq -r '.status // empty' 2>/dev/null || true)
        if [ "$inst_status" != "stopped" ] && [ "$inst_status" != "" ] && [ "$inst_status" != "null" ]; then
          ((remaining++)) || true
        fi
      fi
    done

    if [ "$remaining" -eq 0 ]; then
      return 0
    fi

    local now
    now=$(date +%s)
    if [ $((now - started_at)) -ge "$timeout_sec" ]; then
      return 1
    fi

    sleep 1
  done
}

wait_for_instances_running() {
  local base_url="$1"
  local timeout_sec="${2:-30}"
  shift 2
  local -a instance_ids=("$@")
  local started_at
  started_at=$(date +%s)

  while true; do
    local ready=0
    for instance_id in "${instance_ids[@]}"; do
      local inst_json
      inst_json=$(e2e_curl -sf "${base_url}/instances/${instance_id}" 2>/dev/null || true)
      if [ -n "$inst_json" ]; then
        local inst_status
        inst_status=$(echo "$inst_json" | jq -r '.status // empty' 2>/dev/null || true)
        if [ "$inst_status" = "running" ]; then
          ((ready++)) || true
        fi
      fi
    done

    if [ "$ready" -eq "${#instance_ids[@]}" ]; then
      return 0
    fi

    local now
    now=$(date +%s)
    if [ $((now - started_at)) -ge "$timeout_sec" ]; then
      return 1
    fi

    sleep 1
  done
}

RESULT=""
HTTP_STATUS=""

pinchtab() {
  local method="$1"
  local path="$2"
  shift 2

  echo -e "${BLUE}→ curl -X $method ${E2E_SERVER}$path $(printf "%q " "$@")${NC}" >&2

  local response
  response=$(e2e_curl -s -w "\n%{http_code}" \
    -X "$method" \
    "${E2E_SERVER}$path" \
    -H "Content-Type: application/json" \
    "$@")

  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)

  if [[ ! "$HTTP_STATUS" =~ ^2 ]]; then
    echo -e "${ERROR}  HTTP $HTTP_STATUS: $RESULT${NC}" >&2
  fi
}

pt() { pinchtab "$@"; }

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
  if [ "$1" = "-d" ]; then
    shift
  fi
  pinchtab POST "$path" -d "$1"
  _echo_truncated
}

pt_patch() {
  local path="$1"
  local body="$2"
  echo -e "${BLUE}→ curl -X PATCH ${E2E_SERVER}$path${NC}" >&2
  local response
  response=$(e2e_curl -s -w "\n%{http_code}" \
    -X PATCH \
    "${E2E_SERVER}$path" \
    -H "Content-Type: application/json" \
    -d "$body")
  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)
  _echo_truncated
}

pt_delete() {
  local path="$1"
  echo -e "${BLUE}→ curl -X DELETE ${E2E_SERVER}$path${NC}" >&2
  local response
  response=$(e2e_curl -s -w "\n%{http_code}" \
    -X DELETE \
    "${E2E_SERVER}$path")
  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)
  _echo_truncated
}

pt_post_raw() {
  local path="$1"
  local body="$2"
  echo -e "${BLUE}→ curl -X POST ${E2E_SERVER}$path -d '$body'${NC}" >&2
  local response
  response=$(e2e_curl -s -w "\n%{http_code}" \
    -X POST \
    "${E2E_SERVER}$path" \
    -H "Content-Type: application/json" \
    -d "$body")
  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)
  _echo_truncated
}

assert_url_accessible() {
  local url="$1"
  local label="${2:-$url}"

  if curl -sf "$url" >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} GET $label"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} GET $label (not accessible)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_fixtures_accessible() {
  assert_url_accessible "${FIXTURES_URL}/" "fixtures/"
  assert_url_accessible "${FIXTURES_URL}/form.html" "fixtures/form.html"
  assert_url_accessible "${FIXTURES_URL}/table.html" "fixtures/table.html"
  assert_url_accessible "${FIXTURES_URL}/buttons.html" "fixtures/buttons.html"
}

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

click_button() {
  local name="$1"
  local ref
  ref=$(echo "$RESULT" | jq -r "[.nodes[] | select(.name == \"$name\") | .ref] | first // empty")

  if [ -n "$ref" ] && [ "$ref" != "null" ]; then
    pt_post /action "{\"kind\":\"click\",\"ref\":\"${ref}\"}" >/dev/null
    echo -e "  ${GREEN}✓${NC} clicked '$name' (ref: $ref)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} button '$name' not found"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

type_into() {
  local name="$1"
  local text="$2"
  local ref
  ref=$(echo "$RESULT" | jq -r "[.nodes[] | select(.name == \"$name\") | .ref] | first // empty")

  if [ -z "$ref" ] || [ "$ref" = "null" ]; then
    ref=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "textbox") | .ref] | first // empty')
  fi

  if [ -n "$ref" ] && [ "$ref" != "null" ]; then
    pt_post /action "{\"kind\":\"type\",\"ref\":\"${ref}\",\"text\":\"${text}\"}" >/dev/null
    echo -e "  ${GREEN}✓${NC} typed '$text' into '$name' (ref: $ref)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} input '$name' not found"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

press_key() {
  local key="$1"
  pt_post /action -d "{\"kind\":\"press\",\"key\":\"${key}\"}" >/dev/null
  echo -e "  ${GREEN}✓${NC} pressed '$key'"
  ((ASSERTIONS_PASSED++)) || true
}

get_tab_count() {
  e2e_curl -s "${E2E_SERVER}/tabs" | jq '.tabs | length'
}

get_tab_id() {
  echo "$RESULT" | jq -r '.tabId'
}

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

assert_instance_list_contains() {
  local instance_id="$1"
  local present_msg="$2"
  local missing_msg="$3"
  local found
  found=$(echo "$RESULT" | jq -r ".[] | select(.id == \"$instance_id\") | .id")
  if [ "$found" = "$instance_id" ]; then
    echo -e "  ${GREEN}✓${NC} $present_msg"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $missing_msg"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_instance_list_absent() {
  local instance_id="$1"
  local absent_msg="$2"
  local present_msg="$3"
  local found
  found=$(echo "$RESULT" | jq -r ".[] | select(.id == \"$instance_id\") | .id")
  if [ -z "$found" ] || [ "$found" = "null" ]; then
    echo -e "  ${GREEN}✓${NC} $absent_msg"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $present_msg"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_instance_id_prefix() {
  local instance_id="$1"
  if echo "$instance_id" | grep -q "^inst_"; then
    echo -e "  ${GREEN}✓${NC} instance ID has inst_ prefix: $instance_id"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} bad ID format: $instance_id"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

get_first_tab() {
  echo "$RESULT" | jq -r '.tabs[0].id'
}

show_tab() {
  local label="$1"
  local id="$2"
  echo -e "  ${MUTED}$label: ${id:0:12}...${NC}"
}

assert_tab_count() {
  local expected="$1"
  local actual
  actual=$(get_tab_count)

  if [ "$actual" -eq "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} tab count = $actual"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} tab count: expected $expected, got $actual"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_tab_count_gte() {
  local min="$1"
  local actual
  actual=$(get_tab_count)

  if [ "$actual" -ge "$min" ]; then
    echo -e "  ${GREEN}✓${NC} tab count $actual >= $min"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} tab count: expected >= $min, got $actual"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_tab_closed() {
  local before="$1"
  local actual
  actual=$(get_tab_count)

  if [ "$actual" -lt "$before" ]; then
    echo -e "  ${GREEN}✓${NC} tab closed (before: $before, after: $actual)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} tab not closed (before: $before, after: $actual)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_eval_poll() {
  local expr="$1"
  local expected="$2"
  local desc="${3:-eval poll}"
  local attempts="${4:-5}"
  local delay="${5:-0.4}"
  local tab_id="${6:-}"

  local ok=false
  local actual=""
  for i in $(seq 1 "$attempts"); do
    local json_body
    if [ -n "$tab_id" ]; then
      json_body=$(jq -n --arg expr "$expr" --arg tabId "$tab_id" '{"expression": $expr, "tabId": $tabId}')
    else
      json_body=$(jq -n --arg expr "$expr" '{"expression": $expr}')
    fi
    pt_post /evaluate "$json_body"
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
  fi

  echo -e "  ${RED}✗${NC} $desc (got: $actual, expected: $expected)"
  ((ASSERTIONS_FAILED++)) || true
  return 1
}
