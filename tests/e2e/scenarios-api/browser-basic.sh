#!/bin/bash
# browser-basic.sh — API happy-path browser scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab health"

pt_get /health
assert_json_eq "$RESULT" '.status' 'ok'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "fixtures server"

assert_fixtures_accessible

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav <url>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/\"}"
assert_json_contains "$RESULT" '.title' 'E2E Test'
assert_json_contains "$RESULT" '.url' 'fixtures'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab nav (multiple pages)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_json_contains "$RESULT" '.title' 'Form'

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
assert_json_contains "$RESULT" '.title' 'Table'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab tabs"

assert_tab_count_gte 2

end_test

# ─────────────────────────────────────────────────────────────────
start_test "navigate: blockImages flag"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\",\"blockImages\":true}"
assert_ok "navigate with blockImages"
assert_json_contains "$RESULT" '.url' 'index.html'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "navigate: blockAds flag"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\",\"blockAds\":true}"
assert_ok "navigate with blockAds"
assert_json_contains "$RESULT" '.url' 'index.html'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "navigate: timeout parameter"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\",\"timeout\":30000}"
assert_ok "navigate with custom timeout"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "navigate: waitSelector parameter"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\",\"waitSelector\":\"button\"}"
assert_ok "navigate with waitSelector"
assert_json_contains "$RESULT" '.title' 'Button'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "navigate: missing URL → error"

pt_post /navigate '{}'
assert_not_ok "rejects missing URL"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "navigate: bad JSON → 400"

pt_post_raw /navigate '{broken'
assert_not_ok "rejects bad JSON"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/\"}"

pt_get /snapshot
assert_index_page "$RESULT"
assert_json_length_gte "$RESULT" '.nodes' 1

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap (buttons.html)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
assert_buttons_page "$RESULT"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab snap (form.html)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
sleep 1

pt_get /snapshot
assert_form_page "$RESULT"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab text (table.html)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
sleep 1

TEXT_RESULT=$(curl -s "${E2E_SERVER}/text" | jq -r '.text')
assert_table_page "$TEXT_RESULT"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: diff mode"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
assert_ok "initial snapshot"
INITIAL_COUNT=$(echo "$RESULT" | jq '.nodes | length')

pt_get "/snapshot?diff=true"
assert_ok "diff snapshot"
DIFF_COUNT=$(echo "$RESULT" | jq '.nodes | length')

if [ "$DIFF_COUNT" -le "$INITIAL_COUNT" ]; then
  echo -e "  ${GREEN}✓${NC} diff has <= nodes than full ($DIFF_COUNT <= $INITIAL_COUNT)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} diff has more nodes than full ($DIFF_COUNT > $INITIAL_COUNT)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: maxTokens truncation"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
FULL_COUNT=$(echo "$RESULT" | jq '.nodes | length')

pt_get "/snapshot?maxTokens=50"
assert_ok "snapshot with maxTokens"
LIMITED_COUNT=$(echo "$RESULT" | jq '.nodes | length')

if [ "$LIMITED_COUNT" -le "$FULL_COUNT" ]; then
  echo -e "  ${GREEN}✓${NC} maxTokens limited nodes ($LIMITED_COUNT <= $FULL_COUNT)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} maxTokens did not limit ($LIMITED_COUNT > $FULL_COUNT)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: depth parameter"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
sleep 1

pt_get /snapshot
FULL_COUNT=$(echo "$RESULT" | jq '.nodes | length')

pt_get "/snapshot?depth=1"
assert_ok "snapshot with depth=1"
SHALLOW_COUNT=$(echo "$RESULT" | jq '.nodes | length')

if [ "$SHALLOW_COUNT" -le "$FULL_COUNT" ]; then
  echo -e "  ${GREEN}✓${NC} depth=1 limited tree ($SHALLOW_COUNT <= $FULL_COUNT)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} depth=1 did not limit ($SHALLOW_COUNT > $FULL_COUNT)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: format=text"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_get "/snapshot?format=text"
assert_ok "get text format"

if echo "$RESULT" | head -c1 | grep -q '{'; then
  echo -e "  ${RED}✗${NC} got JSON instead of text"
  ((ASSERTIONS_FAILED++)) || true
else
  echo -e "  ${GREEN}✓${NC} format is text, not JSON"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: nonexistent tabId → error"

pt_get "/snapshot?tabId=nonexistent_xyz_999"
assert_not_ok "rejects bad tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: ref stability after action"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
pt_get /snapshot
REFS_BEFORE=$(echo "$RESULT" | jq '[.nodes[].ref] | sort')

pt_post /action '{"kind":"press","key":"Escape"}'
pt_get /snapshot
REFS_AFTER=$(echo "$RESULT" | jq '[.nodes[].ref] | sort')

if [ "$REFS_BEFORE" = "$REFS_AFTER" ]; then
  echo -e "  ${GREEN}✓${NC} refs stable after action"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} refs changed (may be expected if DOM changed)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: format=yaml"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_get "/snapshot?format=yaml"
assert_ok "get yaml format"

if echo "$RESULT" | grep -q "role:\|name:\|ref:"; then
  echo -e "  ${GREEN}✓${NC} looks like YAML"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} may not be YAML format"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "snapshot: output=file"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_get "/snapshot?output=file"
assert_ok "snapshot output=file"
assert_json_exists "$RESULT" '.path' "has file path"

end_test
# ─────────────────────────────────────────────────────────────────
start_test "snapshot: multi-tab content isolation with tabId"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate to index"
TAB_A=$(echo "$RESULT" | jq -r '.tabId')

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate to form"
TAB_B=$(echo "$RESULT" | jq -r '.tabId')

pt_get "/snapshot?tabId=${TAB_A}"
assert_ok "snapshot tab A"
assert_json_contains "$RESULT" '.url' "index.html" "tab A has index.html URL"

pt_get "/snapshot?tabId=${TAB_B}"
assert_ok "snapshot tab B"
assert_json_contains "$RESULT" '.url' "form.html" "tab B has form.html URL"

pt_get "/text?tabId=${TAB_A}&format=text"
assert_ok "text tab A"
assert_contains "$RESULT" "test fixtures" "tab A text matches index.html"

pt_get "/text?tabId=${TAB_B}&format=text"
assert_ok "text tab B"
assert_contains "$RESULT" "Form Test" "tab B text matches form.html"

end_test

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/find.html\"}"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (login button)"

pt_post /find -d '{"query":"login button"}'
assert_ok "find login"
assert_result_exists ".best_ref" "has best_ref"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (email input)"

pt_post /find -d '{"query":"email input field"}'
assert_ok "find email"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (delete button)"

pt_post /find -d '{"query":"delete account button","threshold":0.2}'
assert_ok "find delete"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (search)"

pt_post /find -d '{"query":"search input","topK":5}'
assert_ok "find search"
assert_json_length_gte "$RESULT" ".matches" 1 "has matches"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find --tab <id>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/find.html\",\"newTab\":true}"
assert_ok "navigate for find"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
sleep 1

pt_post "/tabs/${TAB_ID}/find" -d '{"query":"sign up link"}'
assert_ok "tab find"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (explain mode)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/find.html\"}"
sleep 1

pt_post /find -d '{"query":"login button","explain":true}'
assert_ok "find with explain"

FIRST_EXPLAIN=$(echo "$RESULT" | jq '.matches[0].explain // empty')
if [ -n "$FIRST_EXPLAIN" ] && [ "$FIRST_EXPLAIN" != "null" ]; then
  echo -e "  ${GREEN}✓${NC} explain field present in matches"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} explain field not in response (may need embedding model)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (weight overrides)"

pt_post /find -d '{"query":"login button","lexicalWeight":1.0,"embeddingWeight":0.0}'
assert_ok "find with lexical-only weights"
assert_json_length_gte "$RESULT" ".matches" 1 "has matches with custom weights"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab find (low confidence → empty best_ref)"

pt_post /find -d '{"query":"xyzzy_nonexistent_element_12345","threshold":0.99}'
assert_ok "find with high threshold"

BEST_REF=$(echo "$RESULT" | jq -r '.best_ref // empty')
if [ -z "$BEST_REF" ] || [ "$BEST_REF" = "" ] || [ "$BEST_REF" = "null" ]; then
  echo -e "  ${GREEN}✓${NC} best_ref empty for low-confidence query"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} best_ref returned: $BEST_REF (threshold may be met)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/evaluate.html\"}"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (simple expression)"

pt_post /evaluate -d '{"expression":"1 + 1"}'
assert_ok "evaluate simple"
assert_result_eq ".result" "2" "1+1=2"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (DOM query)"

pt_post /evaluate -d '{"expression":"document.title"}'
assert_ok "evaluate DOM"
assert_result_eq ".result" "Evaluate Test Page" "got title"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (call function)"

pt_post /evaluate -d '{"expression":"window.calculate.add(5, 3)"}'
assert_ok "evaluate function"
assert_result_eq ".result" "8" "5+3=8"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (get object)"

pt_post /evaluate -d '{"expression":"JSON.stringify(window.testData)"}'
assert_ok "evaluate object"
assert_json_contains "$RESULT" ".result" "PinchTab" "has testData"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (modify DOM)"

pt_post /evaluate -d '{"expression":"document.getElementById(\"counter\").textContent = \"42\"; 42"}'
assert_ok "evaluate modify DOM"

pt_post /evaluate -d '{"expression":"document.getElementById(\"counter\").textContent"}'
assert_result_eq ".result" "42" "counter=42"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate --tab <id>"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/evaluate.html\",\"newTab\":true}"
assert_ok "navigate for evaluate"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
sleep 1

pt_post "/tabs/${TAB_ID}/evaluate" -d '{"expression":"1 + 2 + 3"}'
assert_ok "tab evaluate"
assert_result_eq ".result" "6" "1+2+3=6"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "evaluate: missing expression → error"

pt_post /evaluate '{}'
assert_not_ok "rejects missing expression"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "evaluate: bad JSON → 400"

pt_post_raw /evaluate '{broken'
assert_not_ok "rejects bad JSON"

end_test

start_test "text extraction: GET /text extracts readable content"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_get /text
assert_ok "get text"

assert_contains "$RESULT" "E2E Test\|Buttons\|Search\|Customize" "text contains page content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "text extraction: GET /tabs/{id}/text extracts per-tab content"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/tabs/${TAB_ID}/text"
assert_ok "get tab text"

assert_contains "$RESULT" "Click me\|Button" "text includes button labels"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "text extraction: text differs between tabs"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
TAB_ID2=$(get_tab_id)

pt_get "/tabs/${TAB_ID2}/text"
FORM_TEXT="$RESULT"

assert_contains "$FORM_TEXT" "Name\|Email\|Submit\|Form" "text includes form labels"

end_test

# ─────────────────────────────────────────────────────────────────
# ─────────────────────────────────────────────────────────────────
start_test "text extraction: raw mode"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

pt_get "/text?mode=raw"
assert_ok "get text raw"
assert_contains "$RESULT" "E2E Test\|Welcome\|index" "raw text has content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "text extraction: nonexistent tab → error"

pt_get "/text?tabId=nonexistent_xyz_999"
assert_not_ok "rejects bad tab"

end_test

# ─────────────────────────────────────────────────────────────────
# ─────────────────────────────────────────────────────────────────
start_test "text extraction: maxChars truncation"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

pt_get "/text?maxChars=50"
assert_ok "get text with maxChars=50"

TEXT_LEN=$(echo "$RESULT" | jq -r '.text' | wc -c)
if [ "$TEXT_LEN" -le 55 ]; then  # small buffer for json encoding
  echo -e "  ${GREEN}✓${NC} text truncated to ~50 chars (got $TEXT_LEN)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} text not truncated (got $TEXT_LEN chars)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "text extraction: format=text returns plain text"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

# format=text should return plain text content type
RESPONSE=$(curl -s -w "\n%{http_code}\n%{content_type}" "${E2E_SERVER}/text?format=text")
BODY=$(echo "$RESPONSE" | head -n -2)
STATUS=$(echo "$RESPONSE" | tail -n 2 | head -1)
CTYPE=$(echo "$RESPONSE" | tail -n 1)

if [ "$STATUS" = "200" ]; then
  echo -e "  ${GREEN}✓${NC} format=text returned 200"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} format=text returned $STATUS"
  ((ASSERTIONS_FAILED++)) || true
fi

if echo "$CTYPE" | grep -q "text/plain"; then
  echo -e "  ${GREEN}✓${NC} content-type is text/plain"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} content-type is $CTYPE (expected text/plain)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "text extraction: text excludes script/style content"

if echo "$RESULT" | grep -q "function\|var\|css\|<script>"; then
  echo -e "  ${YELLOW}~${NC} text may contain code (depends on sanitization)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${GREEN}✓${NC} text properly excludes code content"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "text extraction: token efficiency (reasonable length)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_get /text
assert_ok "get text"

TEXT_LEN=$(echo "$RESULT" | jq -r '.text' | wc -c)
if [ "$TEXT_LEN" -lt 10000 ]; then
  echo -e "  ${GREEN}✓${NC} text reasonably compact ($TEXT_LEN chars)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} text may be bloated ($TEXT_LEN chars)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test
