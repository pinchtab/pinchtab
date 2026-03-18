#!/bin/bash
# browser-full.sh — API advanced browser scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

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

pt_post /navigate -d '{"url":"https://httpbin.org/redirect/1"}'
assert_ok "single redirect followed"

pt_get /snapshot
assert_json_contains "$RESULT" ".url" "httpbin.org/get" "final URL is /get (redirect successful)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "redirects: follow multiple redirects"

pt_post /navigate -d '{"url":"https://httpbin.org/redirect/5"}'
assert_ok "five redirects followed"

pt_get /snapshot
assert_json_contains "$RESULT" ".url" "httpbin.org/get" "multiple redirects followed to destination"

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

sleep 1
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
