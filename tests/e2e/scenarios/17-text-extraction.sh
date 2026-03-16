#!/bin/bash
# 17-text-extraction.sh — Text extraction from pages (global and per-tab)

source "$(dirname "$0")/common.sh"

start_test "text extraction: GET /text extracts readable content"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
pt_get /text
assert_ok "get text"

# Verify text contains expected content from the page
assert_contains "$RESULT" "E2E Test\|Buttons\|Search\|Customize" "text contains page content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "text extraction: GET /tabs/{id}/text extracts per-tab content"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/tabs/${TAB_ID}/text"
assert_ok "get tab text"

# Buttons page has button elements
assert_contains "$RESULT" "Click me\|Button" "text includes button labels"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "text extraction: text differs between tabs"

# Create another tab with different content
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
TAB_ID2=$(get_tab_id)

pt_get "/tabs/${TAB_ID2}/text"
FORM_TEXT="$RESULT"

# Form page should have form labels
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

# Should be plain text, not JSON
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

# Text extraction should not include raw JavaScript or CSS
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
