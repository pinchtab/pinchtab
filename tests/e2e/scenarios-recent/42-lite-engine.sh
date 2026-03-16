#!/bin/bash
# Test: Lite engine (no Chrome, DOM-only)
# Tests against E2E_LITE_SERVER — a pinchtab instance with engine=lite in bridge mode
source "$(dirname "$0")/common.sh"

LITE_URL="${E2E_LITE_SERVER:-}"
if [ -z "$LITE_URL" ]; then
  echo "  ⚠️  E2E_LITE_SERVER not set, skipping lite engine tests"
  return 0 2>/dev/null || exit 0
fi

# Helper: make requests against the lite instance (mirrors pinchtab() but different base URL)
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

# Navigate to a page with a non-navigating button (type=button)
lite_post /navigate "{\"url\":\"${FIXTURES_URL}/lite-test.html\"}"
assert_ok "navigate to lite test page"
ACTION_TAB=$(echo "$RESULT" | jq -r '.tabId // empty')

# Get interactive snapshot to find the button
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

# Reuse the lite-test page which has a textbox
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

# Verify different tab IDs
if [ "$TAB_A" != "$TAB_B" ]; then
  echo -e "  ${GREEN}✓${NC} different tab IDs: $TAB_A vs $TAB_B"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} tabs should have different IDs"
  ((ASSERTIONS_FAILED++)) || true
fi

# Verify tab A still returns page 1 content
lite_get "/text?tabId=${TAB_A}&format=text"
assert_ok "text for tab A"
assert_contains "$RESULT" "E2E Test Suite" "tab A returns index.html content"

# Verify tab B returns page 2 content
lite_get "/text?tabId=${TAB_B}&format=text"
assert_ok "text for tab B"
assert_contains "$RESULT" "Form" "tab B returns form.html content"

end_test
