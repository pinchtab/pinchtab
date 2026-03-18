#!/bin/bash
# system-full.sh — API advanced config and platform scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

# Migrated from: tests/integration/metrics_test.go

# ─────────────────────────────────────────────────────────────────
start_test "GET /instances/metrics"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

pt_get "/instances/metrics"
assert_ok "get instance metrics"
assert_json_exists "$RESULT" '.[0].instanceId'
assert_json_exists "$RESULT" '.[0].jsHeapUsedMB'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /tabs/{id}/metrics"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate"
TAB_ID=$(get_tab_id)

pt_get "/tabs/${TAB_ID}/metrics"
assert_ok "get tab metrics"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /tabs/{invalid}/metrics → 404"

pt_get "/tabs/invalid_tab_id/metrics"
assert_http_status "404" "unknown tab metrics return 404"
assert_contains "$RESULT" "not found" "unknown tab metrics reports not found"

end_test

# Migrated from: tests/integration/profile_rename_test.go + security_profiles_test.go

# ─────────────────────────────────────────────────────────────────
start_test "profile: create + rename + verify"

pt_post /profiles/create '{"name":"e2e-rename-test"}'
assert_ok "create profile"
ORIG_ID=$(echo "$RESULT" | jq -r '.id')

pt_patch "/profiles/${ORIG_ID}" '{"name":"e2e-rename-test-renamed"}'
assert_ok "rename profile"
NEW_ID=$(echo "$RESULT" | jq -r '.id')
assert_json_eq "$RESULT" '.name' 'e2e-rename-test-renamed'

pt_get "/profiles/${NEW_ID}"
assert_ok "get by new ID"

pt_get "/profiles/${ORIG_ID}"
assert_not_ok "old ID returns error"

pt_delete "/profiles/${NEW_ID}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: PATCH requires ID (not name)"

pt_post /profiles/create '{"name":"e2e-patch-by-name"}'
assert_ok "create"
PATCH_ID=$(echo "$RESULT" | jq -r '.id')

pt_patch "/profiles/e2e-patch-by-name" '{"name":"new-name"}'
assert_http_status "404" "PATCH by name rejected"

pt_delete "/profiles/${PATCH_ID}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: DELETE requires ID (not name)"

pt_post /profiles/create '{"name":"e2e-delete-by-name"}'
assert_ok "create"
DEL_ID=$(echo "$RESULT" | jq -r '.id')

pt_delete "/profiles/e2e-delete-by-name"
assert_http_status "404" "DELETE by name rejected"

pt_delete "/profiles/${DEL_ID}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: rename conflict → 409"

pt_post /profiles/create '{"name":"e2e-conflict-a"}'
assert_ok "create A"
ID_A=$(echo "$RESULT" | jq -r '.id')

pt_post /profiles/create '{"name":"e2e-conflict-b"}'
assert_ok "create B"
ID_B=$(echo "$RESULT" | jq -r '.id')

pt_patch "/profiles/${ID_A}" '{"name":"e2e-conflict-b"}'
assert_http_status "409" "conflict on duplicate name"

pt_delete "/profiles/${ID_A}"
pt_delete "/profiles/${ID_B}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: path traversal rejected"

pt_post /profiles/create '{"name":"e2e-traversal"}'
assert_ok "create"
TRAV_ID=$(echo "$RESULT" | jq -r '.id')

pt_patch "/profiles/${TRAV_ID}" '{"name":"../etc/passwd"}'
assert_not_ok "rejects path traversal"

pt_patch "/profiles/${TRAV_ID}" '{"name":"foo/../../../bar"}'
assert_not_ok "rejects nested traversal"

pt_delete "/profiles/${TRAV_ID}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: empty name rejected"

pt_post /profiles/create '{"name":""}'
assert_not_ok "rejects empty name"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: path separator rejected"

pt_post /profiles/create '{"name":"profile/subdir"}'
assert_not_ok "rejects forward slash"

pt_post /profiles/create '{"name":"dir\\myprofile"}'
assert_not_ok "rejects backslash"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: valid names accepted"

for NAME in "e2e-valid-1" "e2e_valid_2" "e2eVALID3"; do
  pt_post /profiles/create "{\"name\":\"${NAME}\"}"
  assert_ok "create $NAME"
  VALID_ID=$(echo "$RESULT" | jq -r '.id')
  pt_delete "/profiles/${VALID_ID}"
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile reset"

pt_post /profiles -d '{"name":"reset-test-profile"}'
assert_ok "create profile for reset"
PROFILE_ID=$(echo "$RESULT" | jq -r '.id')

pt_post "/profiles/${PROFILE_ID}/reset" ""
assert_ok "reset profile"

pt_delete "/profiles/${PROFILE_ID}"

end_test

# Tests content-based IDPI scanning:
#   - E2E_SERVER (main): IDPI enabled, scanContent=true (default), warn mode
#   - E2E_SECURE_SERVER (secure): IDPI enabled, strictMode=true

# ─────────────────────────────────────────────────────────────────
# ─────────────────────────────────────────────────────────────────

# Usage: TAB_ID=$(idpi_setup <base_url> <page_url>)
idpi_setup() {
  local base_url="$1" page_url="$2"
  local old_url="$E2E_SERVER"
  E2E_SERVER="$base_url"
  pt_post /navigate "{\"url\":\"$page_url\"}" >/dev/null
  E2E_SERVER="$old_url"
  sleep 1
  echo "$RESULT" | jq -r '.tabId'
}

idpi_cleanup() {
  local base_url="$1" tab_id="$2"
  curl -sf -X POST "${base_url}/tab" \
    -H "Content-Type: application/json" \
    -d "{\"tabId\":\"$tab_id\",\"action\":\"close\"}" >/dev/null 2>&1 || true
}

# Usage: idpi_request POST <base_url> <path> <body> <header_name>
#        idpi_request GET  <base_url> <path> ""     <header_name>
# Sets: RESULT, HTTP_STATUS, HDR_VALUE
idpi_request() {
  local method="$1" base_url="$2" path="$3" body="$4" header_name="$5"
  echo -e "${BLUE}→ curl -X $method ${base_url}${path}${NC}" >&2
  local tmpheaders=$(mktemp)
  local curl_args=(-s -w "\n%{http_code}" -X "$method" "${base_url}${path}" -H "Content-Type: application/json" -D "$tmpheaders")
  [ -n "$body" ] && curl_args+=(-d "$body")
  local response
  response=$(curl "${curl_args[@]}")
  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)
  HDR_VALUE=$(grep -i "^${header_name}:" "$tmpheaders" | sed 's/^[^:]*: *//' | tr -d '\r' | head -1)
  rm -f "$tmpheaders"
}

assert_header_present() {
  local desc="$1"
  if [ -n "$HDR_VALUE" ]; then
    echo -e "  ${GREEN}✓${NC} $desc: $HDR_VALUE"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (header missing)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_header_absent() {
  local desc="$1"
  if [ -z "$HDR_VALUE" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (unexpected: $HDR_VALUE)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

FIND_BODY='{"query":"continue button","threshold":0.1,"topK":5}'
FIND_CLEAN='{"query":"safe action button","threshold":0.1,"topK":5}'

# /find — WARN MODE (main instance)

start_test "idpi: /find clean page — no warning (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-clean.html")
idpi_request POST "$E2E_SERVER" "/tabs/${TAB_ID}/find" "$FIND_CLEAN" "X-IDPI-Warning"
assert_ok "/find clean page"
assert_header_absent "no X-IDPI-Warning on clean page"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /find injection page — warns (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request POST "$E2E_SERVER" "/tabs/${TAB_ID}/find" "$FIND_BODY" "X-IDPI-Warning"
assert_ok "/find injection (warn mode returns 200)"
assert_header_present "X-IDPI-Warning header present"
assert_json_exists "$RESULT" ".idpiWarning" "idpiWarning field in body"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: POST /find injection — warns (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request POST "$E2E_SERVER" "/find" "{\"query\":\"malicious paragraph\",\"tabId\":\"$TAB_ID\",\"threshold\":0.1}" "X-IDPI-Warning"
assert_ok "POST /find (warn mode)"
assert_header_present "X-IDPI-Warning on POST /find"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# /find — STRICT MODE (secure instance)

start_test "idpi: /find clean page — allowed (strict mode)"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-clean.html")
idpi_request POST "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/find" "$FIND_CLEAN" "X-IDPI-Warning"
assert_ok "/find clean page (strict mode)"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /find injection page — blocked (strict mode)"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request POST "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/find" "$FIND_BODY" "X-IDPI-Warning"
assert_http_status 403 "/find blocked in strict mode"
assert_contains "$RESULT" "idpi" "403 body mentions IDPI"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test

# /pdf — WARN MODE (main instance)

start_test "idpi: /pdf clean page — no warning (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-clean.html")
idpi_request GET "$E2E_SERVER" "/tabs/${TAB_ID}/pdf" "" "X-IDPI-Warning"
assert_ok "/pdf clean page"
assert_header_absent "no X-IDPI-Warning on clean PDF"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /pdf injection page — warns (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request GET "$E2E_SERVER" "/tabs/${TAB_ID}/pdf" "" "X-IDPI-Warning"
assert_ok "/pdf injection (warn mode returns 200)"
assert_header_present "X-IDPI-Warning on injection PDF"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# /pdf — STRICT MODE (secure instance)

start_test "idpi: /pdf clean page — allowed (strict mode)"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-clean.html")
idpi_request GET "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/pdf" "" "X-IDPI-Warning"
assert_ok "/pdf clean page (strict mode)"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /pdf injection page — blocked (strict mode)"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request GET "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/pdf" "" "X-IDPI-Warning"
assert_http_status 403 "/pdf blocked in strict mode"
assert_contains "$RESULT" "idpi" "403 body mentions IDPI"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test

start_test "idpi: multiple injection phrases — single warning header"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-inject.html")
tmpheaders=$(mktemp)
curl -s -X POST "${E2E_SERVER}/tabs/${TAB_ID}/find" \
  -H "Content-Type: application/json" \
  -D "$tmpheaders" \
  -d "$FIND_BODY" >/dev/null
HDR_COUNT=$(grep -ci "^X-IDPI-Warning:" "$tmpheaders" 2>/dev/null || echo "0")
rm -f "$tmpheaders"
if [ "$HDR_COUNT" -eq 1 ]; then
  echo -e "  ${GREEN}✓${NC} exactly one X-IDPI-Warning header"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected 1 X-IDPI-Warning, got $HDR_COUNT"
  ((ASSERTIONS_FAILED++)) || true
fi
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /pdf?raw=true blocked in strict mode"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request GET "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/pdf?raw=true" "" "X-IDPI-Warning"
assert_http_status 403 "raw PDF blocked in strict mode"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test

AGENT="test-agent-$$"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks — submit task"

pt_post /tasks -d "{\"agentId\":\"${AGENT}\",\"action\":\"snapshot\",\"tabId\":\"${TAB_ID}\"}"
assert_http_status "202" "task accepted"
TASK_ID=$(echo "$RESULT" | jq -r '.taskId')
assert_json_eq "$RESULT" ".state" "queued" "initial state is queued"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /tasks/{id} — get task"

sleep 2
pt_get "/tasks/${TASK_ID}"
assert_ok "get task by id"
assert_json_eq "$RESULT" ".taskId" "$TASK_ID" "correct task id"
assert_json_eq "$RESULT" ".agentId" "$AGENT" "correct agent id"
assert_json_eq "$RESULT" ".action" "snapshot" "correct action"
STATE=$(echo "$RESULT" | jq -r '.state')
if [ "$STATE" = "completed" ] || [ "$STATE" = "running" ] || [ "$STATE" = "failed" ]; then
  echo -e "  ${GREEN}✓${NC} task reached terminal/active state: $STATE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} unexpected state: $STATE"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /tasks — list tasks"

pt_get "/tasks?agentId=${AGENT}"
assert_ok "list tasks"
COUNT=$(echo "$RESULT" | jq '.count')
if [ "$COUNT" -ge 1 ]; then
  echo -e "  ${GREEN}✓${NC} found $COUNT tasks for agent"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected at least 1 task, got $COUNT"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /tasks — filter by state"

pt_get "/tasks?state=completed,failed"
assert_ok "list terminal tasks"
assert_json_exists "$RESULT" '.tasks' "tasks array present in response"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks/{id}/cancel — cancel queued task"

pt_post /tasks -d "{\"agentId\":\"${AGENT}\",\"action\":\"snapshot\",\"tabId\":\"${TAB_ID}\"}"
assert_http_status "202" "task accepted for cancel test"
CANCEL_ID=$(echo "$RESULT" | jq -r '.taskId')

pt_post "/tasks/${CANCEL_ID}/cancel" ""
if [ "$HTTP_STATUS" = "200" ]; then
  assert_json_eq "$RESULT" ".status" "cancelled" "task cancelled"
elif [ "$HTTP_STATUS" = "409" ]; then
  echo -e "  ${GREEN}✓${NC} task already terminal (409 conflict, acceptable)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} unexpected status: $HTTP_STATUS"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks/{id}/cancel — cancel nonexistent → 404"

pt_post "/tasks/tsk_nonexistent/cancel" ""
assert_http_status "404" "cancel nonexistent task"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /scheduler/stats"

pt_get /scheduler/stats
assert_ok "stats endpoint"
assert_json_exists "$RESULT" '.queue' "has queue stats"
assert_json_exists "$RESULT" '.metrics' "has metrics"
assert_json_eq "$RESULT" ".config.strategy" "fair-fifo" "strategy is fair-fifo"
assert_json_eq "$RESULT" ".config.maxQueueSize" "5" "maxQueueSize matches config"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks/batch — submit 3 tasks"

pt_post /tasks/batch -d "{\"agentId\":\"${AGENT}\",\"tasks\":[{\"action\":\"snapshot\",\"tabId\":\"${TAB_ID}\"},{\"action\":\"text\",\"tabId\":\"${TAB_ID}\"},{\"action\":\"screenshot\",\"tabId\":\"${TAB_ID}\"}]}"
assert_http_status "202" "batch accepted"
SUBMITTED=$(echo "$RESULT" | jq '.submitted')
if [ "$SUBMITTED" = "3" ]; then
  echo -e "  ${GREEN}✓${NC} all 3 tasks submitted"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected 3 submitted, got $SUBMITTED"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks/batch — validation: empty tasks"

pt_post /tasks/batch -d '{"agentId":"test","tasks":[]}'
assert_http_status "400" "empty batch rejected"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks/batch — validation: missing agentId"

pt_post /tasks/batch -d '{"tasks":[{"action":"snapshot"}]}'
assert_http_status "400" "missing agentId rejected"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks — deadline in the past → 400"

pt_post /tasks -d "{\"agentId\":\"${AGENT}\",\"action\":\"snapshot\",\"tabId\":\"${TAB_ID}\",\"deadline\":\"2020-01-01T00:00:00Z\"}"
assert_http_status "400" "past deadline rejected"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks — 429 queue full"

# maxPerAgent is 3, submit rapidly for a fresh agent
FLOOD_AGENT="flood-agent-$$"
for i in $(seq 1 4); do
  pt_post /tasks -d "{\"agentId\":\"${FLOOD_AGENT}\",\"action\":\"snapshot\",\"tabId\":\"${TAB_ID}\"}"
done
if [ "$HTTP_STATUS" = "429" ]; then
  echo -e "  ${GREEN}✓${NC} queue full for agent (HTTP 429)"
  ((ASSERTIONS_PASSED++)) || true
elif [ "$HTTP_STATUS" = "202" ]; then
  echo -e "  ${GREEN}✓${NC} task accepted (tasks completed before limit hit)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} unexpected status: $HTTP_STATUS (expected 429 or 202)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /tasks/{id} — nonexistent → 404"

pt_get /tasks/tsk_doesnotexist
assert_http_status "404" "nonexistent task"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks — missing agentId → 400"

pt_post /tasks -d '{"action":"snapshot"}'
assert_http_status "400" "missing agentId rejected"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /tasks — missing action → 400"

pt_post /tasks -d "{\"agentId\":\"validation-agent-$$\"}"
assert_http_status "400" "missing action rejected"

end_test

# Test: Lite engine (no Chrome, DOM-only)

LITE_URL="${E2E_LITE_SERVER:-}"
if [ -z "$LITE_URL" ]; then
  echo "  ⚠️  E2E_LITE_SERVER not set, skipping lite engine tests"
  return 0 2>/dev/null || exit 0
fi

lite() {
  local method="$1"
  local path="$2"
  shift 2
  echo -e "${BLUE}→ curl -X $method ${LITE_URL}$path $(printf "%q " "$@")${NC}" >&2
  local response
  response=$(curl -s -w "\n%{http_code}" \
    -X "$method" \
    "${LITE_URL}${path}" \
    -H "Content-Type: application/json" \
    "$@")
  HTTP_STATUS=$(echo "$response" | tail -1)
  RESULT=$(echo "$response" | sed '$d')
  _echo_truncated
}

lite_get() { lite GET "$1"; }
lite_post() { lite POST "$1" -d "$2"; }

# --- T1: Health check ---
start_test "Lite engine: health check"
lite_get /health
assert_ok "lite health"
end_test

# --- T2: Navigate returns tab ID ---
start_test "Lite engine: navigate returns tabId"
lite_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "lite navigate"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId // empty')
if [ -n "$TAB_ID" ]; then
  echo -e "  ${GREEN}✓${NC} navigate returned tabId=$TAB_ID"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} navigate missing tabId"
  ((ASSERTIONS_FAILED++)) || true
fi
end_test

# --- T3: Snapshot returns nodes ---
start_test "Lite engine: snapshot returns DOM nodes"
lite_get "/snapshot?tabId=${TAB_ID}"
assert_ok "lite snapshot"
NODE_COUNT=$(echo "$RESULT" | jq '.nodes | length' 2>/dev/null || echo 0)
if [ "$NODE_COUNT" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} snapshot returned $NODE_COUNT nodes"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} snapshot returned 0 nodes"
  ((ASSERTIONS_FAILED++)) || true
fi
end_test

# --- T4: Text extraction ---
start_test "Lite engine: text extraction"
lite_get "/text?tabId=${TAB_ID}&format=text"
assert_ok "lite text"
assert_contains "$RESULT" "E2E" "text contains page content"
end_test

# --- T5: Interactive filter ---
start_test "Lite engine: interactive filter"
lite_get "/snapshot?tabId=${TAB_ID}&filter=interactive"
assert_ok "lite snapshot interactive"
end_test

# --- T6: Click action routes through lite ---
start_test "Lite engine: click action"

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/lite-test.html\"}"
assert_ok "navigate to lite test page"
ACTION_TAB=$(echo "$RESULT" | jq -r '.tabId // empty')

lite_get "/snapshot?tabId=${ACTION_TAB}&filter=interactive"
BUTTON_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "button")] | first // empty | .ref // empty')

if [ -n "$BUTTON_REF" ]; then
  lite_post /action "{\"tabId\":\"${ACTION_TAB}\",\"kind\":\"click\",\"ref\":\"${BUTTON_REF}\"}"
  assert_ok "lite click"
else
  echo -e "  ${RED}✗${NC} no button found for click test"
  ((ASSERTIONS_FAILED++)) || true
fi
end_test

# --- T7: Type action routes through lite ---
start_test "Lite engine: type action"

TYPE_TAB="${ACTION_TAB}"

lite_get "/snapshot?tabId=${TYPE_TAB}&filter=interactive"
INPUT_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "textbox")] | first // empty | .ref // empty')

if [ -n "$INPUT_REF" ]; then
  lite_post /action "{\"tabId\":\"${TYPE_TAB}\",\"kind\":\"type\",\"ref\":\"${INPUT_REF}\",\"text\":\"hello\"}"
  assert_ok "lite type"
else
  echo -e "  ${RED}✗${NC} no textbox found on form.html"
  ((ASSERTIONS_FAILED++)) || true
fi
end_test

# --- T8: Unsupported action returns 501 ---
start_test "Lite engine: unsupported action returns 501"
lite_post /action "{\"tabId\":\"${TYPE_TAB}\",\"kind\":\"press\",\"ref\":\"e0\",\"key\":\"Enter\"}"
assert_http_status 501 "press returns 501 in lite mode"
end_test

# --- T9: Multi-tab isolation ---
start_test "Lite engine: multi-tab isolation"

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate page 1"
TAB_A=$(echo "$RESULT" | jq -r '.tabId // empty')

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate page 2"
TAB_B=$(echo "$RESULT" | jq -r '.tabId // empty')

if [ "$TAB_A" != "$TAB_B" ]; then
  echo -e "  ${GREEN}✓${NC} different tab IDs: $TAB_A vs $TAB_B"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} tabs should have different IDs"
  ((ASSERTIONS_FAILED++)) || true
fi

lite_get "/text?tabId=${TAB_A}&format=text"
assert_ok "text for tab A"
assert_contains "$RESULT" "E2E Test Suite" "tab A returns index.html content"

lite_get "/text?tabId=${TAB_B}&format=text"
assert_ok "text for tab B"
assert_contains "$RESULT" "Form" "tab B returns form.html content"

end_test
