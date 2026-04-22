#!/bin/bash
# plugin-basic.sh — Tests core endpoints used by the OpenClaw plugin.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "plugin: health check"

pt_get /health
assert_json_eq "$RESULT" '.status' 'ok'
assert_result_exists ".version" "has version"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: navigate"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate"
assert_result_exists ".tabId" "returns tabId"
assert_json_contains "$RESULT" '.title' 'Form'
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: snapshot with tabId"

pt_get "/snapshot?tabId=${TAB_ID}"
assert_ok "snapshot"
assert_result_exists ".title" "has title"
assert_result_exists ".url" "has url"
assert_json_length_gte "$RESULT" '.nodes' 1

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: action click"

pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"click\",\"selector\":\"#submit-btn\"}"
assert_ok "click action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: action type"

pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"type\",\"selector\":\"#username\",\"text\":\"testuser\"}"
assert_ok "type action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: action scroll"

pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"scroll\",\"direction\":\"down\"}"
assert_ok "scroll action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: action press key"

pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"press\",\"key\":\"Escape\"}"
assert_ok "press key"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: action select"

pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"select\",\"selector\":\"#country\",\"value\":\"us\"}"
assert_ok "select action"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: screenshot"

RESPONSE=$(e2e_curl -s -w "\n%{http_code}" "${E2E_SERVER}/screenshot?tabId=${TAB_ID}")
STATUS=$(echo "$RESPONSE" | tail -n 1)

if [ "$STATUS" = "200" ]; then
  echo -e "  ${GREEN}✓${NC} screenshot returned 200"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} screenshot returned $STATUS"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: wait for selector"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_post /wait -d "{\"tabId\":\"${TAB_ID}\",\"selector\":\"button\"}"
assert_ok "wait for selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: wait for text"

pt_post /wait -d "{\"tabId\":\"${TAB_ID}\",\"text\":\"Click me\"}"
assert_ok "wait for text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: tabs list"

pt_get /tabs
assert_ok "tabs list"
assert_json_length_gte "$RESULT" '.' 1 "has at least one tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: tab new"

pt_post /tab -d '{"action":"new"}'
assert_ok "new tab"
NEW_TAB=$(echo "$RESULT" | jq -r '.tabId')
assert_result_exists ".tabId" "returns tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: tab close"

pt_post /tab -d "{\"action\":\"close\",\"tabId\":\"${NEW_TAB}\"}"
assert_ok "close tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: evaluate JavaScript"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/evaluate.html\"}"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
sleep 1

pt_post /evaluate -d "{\"tabId\":\"${TAB_ID}\",\"expression\":\"document.title\"}"
assert_ok "evaluate"
assert_result_eq ".result" "Evaluate Test Page" "got title"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: text extraction"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_get "/text?tabId=${TAB_ID}"
assert_ok "text extraction"
assert_result_exists ".text" "has text field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: PDF generation"

RESPONSE=$(e2e_curl -s -w "\n%{http_code}" "${E2E_SERVER}/pdf?tabId=${TAB_ID}")
STATUS=$(echo "$RESPONSE" | tail -n 1)

if [ "$STATUS" = "200" ]; then
  echo -e "  ${GREEN}✓${NC} pdf returned 200"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} pdf returned $STATUS"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: network log"

pt_get "/network?tabId=${TAB_ID}"
assert_ok "network log"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: instances list"

pt_get /instances
assert_ok "instances"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "plugin: profiles list"

pt_get /profiles
assert_ok "profiles"

end_test
