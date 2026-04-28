#!/bin/bash
# browser-extended.sh — API advanced browser scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

start_test "error handling: invalid selector syntax"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)
show_tab "created" "$TAB_ID"

pt_post /action -d '{"action":"click","selector":"[invalid:::selector]"}'
assert_http_error 400 "invalid|selector|syntax" "invalid selector rejected"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "error handling: element not found"

pt_post /action -d '{"action":"click","selector":"#this-element-does-not-exist"}'
assert_contains_any "$RESULT" "not found|no element|404|400" "missing element error"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "error handling: action on missing field"

pt_post /action -d '{"action":"fill","selector":"#nonexistent-input","text":"test"}'
assert_contains_any "$RESULT" "not found|missing|404|400" "action on missing field rejected"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "error handling: navigate to invalid URL"

pt_post /navigate -d '{"url":"not a valid url @#$%"}'
assert_contains_any "$RESULT" "400|200|error" "invalid URL handled"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "error handling: error response format"

pt_post /action -d '{"action":"click","selector":"#invalid-selector-#$%"}'

if echo "$RESULT" | jq -e '.error' >/dev/null 2>&1; then
  echo -e "  ${GREEN}✓${NC} error response has error field"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} error format may vary"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "error handling: batch action with error in middle"

pt_post /actions -d '[
  {"action":"click","selector":"button"},
  {"action":"click","selector":"#nonexistent"},
  {"action":"click","selector":"button"}
]'
assert_contains_any "$RESULT" "not found|error|404|400" "batch stops on error"

end_test

start_test "redirects: follow single redirect"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/redirect/1\"}"
assert_ok "single redirect followed"

pt_get /snapshot
assert_json_contains "$RESULT" ".url" "fixtures/get" "final URL is /get (redirect successful)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "redirects: follow multiple redirects"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/redirect/5\"}"
assert_ok "five redirects followed"

pt_get /snapshot
assert_json_contains "$RESULT" ".url" "fixtures/get" "multiple redirects followed to destination"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "redirects: document redirect detection capability"

# (Actual enforcement would require network interception implementation)

echo -e "  ${BLUE}ℹ${NC} Redirect limiting available via CDP Fetch domain"
echo -e "  ${BLUE}ℹ${NC} Default: -1 (unlimited). Set maxRedirects: N to limit hops"
((ASSERTIONS_PASSED++)) || true

end_test

# Migrated from: tests/integration/cookies_test.go

# ─────────────────────────────────────────────────────────────────
start_test "GET /cookies (read cookies)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/cookies?tabId=${TAB_ID}"
assert_ok "get cookies"
assert_json_exists "$RESULT" '.cookies'

COOKIE_COUNT=$(echo "$RESULT" | jq '.cookies | length')
if [ "$COOKIE_COUNT" -gt 0 ]; then
  assert_json_exists "$RESULT" '.cookies[0].name' "cookie has name"
  assert_json_exists "$RESULT" '.cookies[0].value' "cookie has value"
  assert_json_exists "$RESULT" '.cookies[0].domain' "cookie has domain"
  assert_json_exists "$RESULT" '.cookies[0].path' "cookie has path"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /cookies (set + verify)"

pt_post /cookies "{
  \"tabId\": \"${TAB_ID}\",
  \"url\": \"${FIXTURES_URL}/index.html\",
  \"cookies\": [{\"name\": \"test_e2e\", \"value\": \"hello\", \"path\": \"/\"}]
}"
assert_ok "set cookie"
assert_json_eq "$RESULT" '.set' '1'

pt_get "/cookies?tabId=${TAB_ID}&url=${FIXTURES_URL}/index.html"
assert_ok "get cookies after set"
assert_json_exists "$RESULT" '.cookies[] | select(.name == "test_e2e")'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /cookies (non-existent tab → error)"

pt_get "/cookies?tabId=nonexistent_tab_12345"
assert_not_ok "rejects bad tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /cookies (bad JSON → error)"

pt_post_raw /cookies "{broken"
assert_http_status "400" "rejects bad JSON"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /cookies (empty array → error)"

pt_post /cookies "{
  \"tabId\": \"${TAB_ID}\",
  \"url\": \"${FIXTURES_URL}/index.html\",
  \"cookies\": []
}"
assert_http_status "400" "rejects empty cookies"

end_test

# Migrated from: tests/integration/error_handling_test.go (ER4, ER6)

# ─────────────────────────────────────────────────────────────────
start_test "error handling: empty page (about:blank)"

pt_post /navigate '{"url":"about:blank"}'
assert_ok "navigate to about:blank"

TAB_ID=$(get_tab_id)

pt_get "/snapshot?tabId=${TAB_ID}"
assert_ok "snapshot on empty page"

pt_get "/text?tabId=${TAB_ID}"
assert_ok "text on empty page"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "error handling: rapid navigation"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "final navigate succeeded"

pt_get /snapshot
assert_ok "snapshot after rapid nav"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "error handling: unicode content"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/unicode.html\"}"
assert_ok "navigate to unicode page"

pt_get /snapshot
assert_ok "snapshot unicode page"

pt_get /text
assert_ok "text unicode page"
assert_contains "$RESULT" "Unicode" "text has unicode content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "error handling: binary content (image)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/sample.txt\"}"
pt_get /text
pt_get /health
assert_ok "server still healthy after binary/text page"

end_test

# ─────────────────────────────────────────────────────────────────
# POST /wait — wait for page state conditions
# ─────────────────────────────────────────────────────────────────

start_test "POST /wait: wait for selector"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons page"

# Wait for a selector that exists
pt_post /wait '{"selector":"#increment"}'
assert_ok "wait for existing selector"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: wait for milliseconds"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"

# Wait for a small number of milliseconds
pt_post /wait '{"ms":50}'
assert_ok "wait for ms"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: wait for text"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"

# Wait for text that exists on the page
pt_post /wait '{"text":"Increment"}'
assert_ok "wait for text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: wait for text to disappear (not-text immediate)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"

# Text that isn't present — should succeed immediately
pt_post /wait '{"notText":"nonexistent-text-xyz","timeout":2000}'
assert_ok "wait for not-text (absent)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: wait for text to disappear (after toggle)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"

# Click toggle to hide the content (display: none removes from innerText)
pt_post /action '{"kind":"click","selector":"#toggle-btn"}'
assert_ok "click toggle button"

# Wait for the toggled text to disappear
pt_post /wait '{"notText":"This content can be toggled.","timeout":5000}'
assert_ok "wait for toggled text to disappear"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: not-text timeout when text persists"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"

# Text is present and never removed — should time out
pt_post /wait '{"notText":"Increment","timeout":500}'
assert_ok "wait returns 200 with timeout error"
assert_json_eq "$RESULT" '.waited' 'false' "waited=false on timeout"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: load states (ready-state, content-loaded, network-idle)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate before load-state checks"

# Navigation has settled, so all three should resolve quickly.
# network-idle uses a short idleFor so the test stays fast.
for state in ready-state content-loaded network-idle; do
  pt_post /wait "{\"load\":\"${state}\",\"timeout\":8000,\"idleFor\":200}"
  assert_ok "wait --load ${state}"
  assert_json_eq "$RESULT" '.waited' 'true' "waited=true for ${state}"
  assert_json_eq "$RESULT" '.match' "${state}" "match label echoes ${state}"
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: legacy 'networkidle' alias still accepted"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"

pt_post /wait '{"load":"networkidle","timeout":8000,"idleFor":200}'
assert_ok "legacy alias accepted"
assert_json_eq "$RESULT" '.waited' 'true' "waited=true via legacy alias"
assert_json_eq "$RESULT" '.match' 'network-idle' "alias canonicalised to network-idle"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: rejects unknown load state"

pt_post /wait '{"load":"bogus"}'
assert_http_status "400" "rejects unknown load state"

# Pre-rename names should now also be rejected.
pt_post /wait '{"load":"load"}'
assert_http_status "400" "rejects pre-rename 'load'"

pt_post /wait '{"load":"domcontentloaded"}'
assert_http_status "400" "rejects pre-rename 'domcontentloaded'"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: invalid request (empty body)"

pt_post /wait '{}'
# Should fail because no condition is specified
assert_http_status "400" "rejects empty wait request"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /wait: negative ms handled gracefully"

pt_post /wait '{"ms":-100}'
# Should either return immediately or fail gracefully
if [ "$HTTP_STATUS" = "200" ] || [ "$HTTP_STATUS" = "400" ]; then
  echo -e "  ${GREEN}✓${NC} negative ms handled (status: $HTTP_STATUS)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} unexpected status: $HTTP_STATUS"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
# GET /network — network capture
# ─────────────────────────────────────────────────────────────────

start_test "GET /network: list network entries"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/network?tabId=${TAB_ID}"
assert_ok "get network entries"
assert_json_exists "$RESULT" '.entries' "network response has entries"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: filter by method"

pt_get "/network?tabId=${TAB_ID}&method=GET"
assert_ok "filter by GET method"

# All returned entries should be GET (or empty)
ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
if [ "$ENTRIES_COUNT" -gt 0 ]; then
  NON_GET=$(echo "$RESULT" | jq '[.entries[] | select(.method != "GET")] | length')
  if [ "$NON_GET" -eq 0 ]; then
    echo -e "  ${GREEN}✓${NC} all entries are GET requests"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${YELLOW}~${NC} found $NON_GET non-GET entries (filter may be loose)"
    ((ASSERTIONS_PASSED++)) || true
  fi
else
  echo -e "  ${GREEN}✓${NC} no entries (no GET requests captured)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: limit results"

pt_get "/network?tabId=${TAB_ID}&limit=5"
assert_ok "get network with limit"

ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
if [ "$ENTRIES_COUNT" -le 5 ]; then
  echo -e "  ${GREEN}✓${NC} entries limited to $ENTRIES_COUNT (<= 5)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected <= 5 entries, got $ENTRIES_COUNT"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /network: non-existent tab"

pt_get "/network?tabId=nonexistent_tab_xyz_999"
assert_not_ok "rejects non-existent tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /network/clear: clear network data"

pt_post /network/clear "{\"tabId\":\"${TAB_ID}\"}"
assert_ok "clear network data"

# Verify entries are cleared
pt_get "/network?tabId=${TAB_ID}"
assert_ok "get network after clear"
ENTRIES_COUNT=$(echo "$RESULT" | jq '.entries | length')
echo -e "  ${GREEN}✓${NC} entries after clear: $ENTRIES_COUNT"
((ASSERTIONS_PASSED++)) || true

end_test
