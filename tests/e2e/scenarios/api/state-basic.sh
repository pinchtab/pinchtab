#!/bin/bash
# state-basic.sh — tab state endpoint smoke.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

start_test "tab state: complete page reports state"
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/state-loading.html\"}"
assert_ok "open state fixture tab"
TAB_JSON="$RESULT"
TAB_ID=$(get_tab_id "$TAB_JSON")
STATE=$(e2e_curl -s "${E2E_SERVER}/tabs/${TAB_ID}/state")

echo "$STATE" | jq -e '.tabId == "'"$TAB_ID"'"' >/dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} tabId matches"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} tabId mismatch"
  echo "$STATE" | jq .
  ((ASSERTIONS_FAILED++)) || true
fi

echo "$STATE" | jq -e '.load.readyState == "interactive" or .load.readyState == "complete" or .load.readyState == "loading"' >/dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} load.readyState is present"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected load.readyState"
  echo "$STATE" | jq .
  ((ASSERTIONS_FAILED++)) || true
fi

echo "$STATE" | jq -e '.actionability == "ready" or .actionability == "caution"' >/dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} actionability is non-blocked for normal page"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected actionability ready/caution"
  echo "$STATE" | jq .
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

start_test "tab state: dialog blocks actionability"
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "open dialog fixture tab"
TAB_JSON="$RESULT"
TAB_ID=$(get_tab_id "$TAB_JSON")
pt_get "/snapshot?tabId=${TAB_ID}"
assert_ok "snapshot dialog fixture tab"
ALERT_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Trigger Alert")][0].ref // empty')
pt_post /action "{\"tabId\":\"${TAB_ID}\",\"kind\":\"click\",\"ref\":\"${ALERT_REF}\"}" >/dev/null 2>&1 || true
sleep 1
STATE=$(e2e_curl -s "${E2E_SERVER}/tabs/${TAB_ID}/state")

echo "$STATE" | jq -e '.dialogPresent == true' >/dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} dialogPresent=true"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected dialogPresent=true"
  echo "$STATE" | jq .
  ((ASSERTIONS_FAILED++)) || true
fi

echo "$STATE" | jq -e '.actionability == "blocked"' >/dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} actionability=blocked"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected actionability=blocked"
  echo "$STATE" | jq .
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post /dialog -d "{\"tabId\":\"${TAB_ID}\",\"accept\":true}" >/dev/null 2>&1 || true
end_test
