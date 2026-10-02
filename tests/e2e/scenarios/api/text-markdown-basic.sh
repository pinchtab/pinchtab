#!/bin/bash
# text-markdown-basic.sh — GET /text?mode=markdown: page-to-Markdown conversion,
# format/content-type, line-boundary truncation, frame scope, refusal, and the
# IDPI guard parity with the readability path.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "markdown: GET /text?mode=markdown returns converter Markdown"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/markdown.html\"}"
assert_ok "navigate to markdown.html"

pt_get "/text?mode=markdown"
assert_ok "text markdown"
assert_json_eq "$RESULT" '.extraction' 'markdown' "extraction echoes markdown"

TITLE=$(echo "$RESULT" | jq -r '.title')
if [ -n "$TITLE" ] && [ "$TITLE" != "null" ]; then
  pass_assert "title present ($TITLE)"
else
  fail_assert "title missing"
fi

BODY=$(echo "$RESULT" | jq -r '.text')
assert_contains "$BODY" "# " "heading survives as Markdown"
assert_contains "$BODY" "## " "subheading survives as Markdown"
assert_contains "$BODY" "](https://example.com/link)" "inline link survives as Markdown"
assert_contains "$BODY" "- " "list item survives as Markdown"
assert_contains "$BODY" "|" "table survives as Markdown"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "markdown: format=text returns text/markdown content type"

RESPONSE=$(e2e_curl -s -w "\n%{http_code}\n%{content_type}" "${E2E_SERVER}/text?mode=markdown&format=text")
STATUS=$(echo "$RESPONSE" | tail -n 2 | head -1)
CTYPE=$(echo "$RESPONSE" | tail -n 1)

HTTP_STATUS="$STATUS"; assert_http_status 200 "format=text returned 200"
if echo "$CTYPE" | grep -q "text/markdown"; then
  pass_assert "content-type is text/markdown"
else
  fail_assert "content-type is $CTYPE (expected text/markdown)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "markdown: maxChars truncates on a line boundary"

pt_get "/text?mode=markdown&maxChars=120"
assert_ok "text markdown maxChars=120"
assert_json_eq "$RESULT" '.truncated' 'true' "response marked truncated"

# Truncation rule: whole lines are kept, the final overrunning line is a rune-cut
# prefix, a table row is dropped whole, and a cut pulls back to before a [..](..)
# link. Assert every line but the last is a whole source line; the last is a
# prefix of some source line; no returned line is a partial table row; and the
# result leaves no link half-open.
FULL=$(pt_get "/text?mode=markdown" >/dev/null; echo "$RESULT" | jq -r '.text')
CUT=$(pt_get "/text?mode=markdown&maxChars=120" >/dev/null; echo "$RESULT" | jq -r '.text')
CUT_LINES=()
while IFS= read -r line; do CUT_LINES+=("$line"); done <<<"$CUT"
LAST_IDX=$(( ${#CUT_LINES[@]} - 1 ))
LINE_RULE_OK=1
for i in "${!CUT_LINES[@]}"; do
  line="${CUT_LINES[$i]}"
  [ -z "$line" ] && continue
  if [ "$i" -lt "$LAST_IDX" ]; then
    if ! grep -Fqx -- "$line" <<<"$FULL"; then
      LINE_RULE_OK=0; echo -e "  ${RED}✗${NC} non-final line not whole: $line"
    fi
  else
    PREFIX_OK=0
    while IFS= read -r fline; do
      case "$fline" in "$line"*) PREFIX_OK=1; break;; esac
    done <<<"$FULL"
    if [ "$PREFIX_OK" -eq 0 ]; then
      LINE_RULE_OK=0; echo -e "  ${RED}✗${NC} final line is not a prefix of any source line: $line"
    fi
  fi
  case "$line" in
    "|"*) grep -Fqx -- "$line" <<<"$FULL" || { LINE_RULE_OK=0; echo -e "  ${RED}✗${NC} partial table row: $line"; } ;;
  esac
done
# No half-open link anywhere in the result: brackets balance and every "](" has a
# closing ")".
OPEN=$(tr -cd '[' <<<"$CUT" | wc -c); CLOSE=$(tr -cd ']' <<<"$CUT" | wc -c)
LOPEN=$(grep -o '](' <<<"$CUT" | wc -l); RPAREN=$(tr -cd ')' <<<"$CUT" | wc -c)
if [ "$OPEN" -ne "$CLOSE" ] || [ "$LOPEN" -gt "$RPAREN" ]; then
  LINE_RULE_OK=0; echo -e "  ${RED}✗${NC} unbalanced link markup: [=$OPEN ]=$CLOSE ](=$LOPEN )=$RPAREN"
fi
if [ "$LINE_RULE_OK" -eq 1 ]; then
  pass_assert "truncated lines obey the cut rule (whole lines, prefix last line, no split row or link)"
else
  fail_assert "truncated lines break the cut rule"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "markdown: frame scope converts the selected iframe"

pt_post /frame -d '{"target":"#md-frame"}'
assert_ok "select iframe scope"
assert_json_eq "$RESULT" '.scoped' 'true' "frame scope enabled"

pt_get "/text?mode=markdown"
assert_ok "frame-scoped markdown"
FRAME_BODY=$(echo "$RESULT" | jq -r '.text')
assert_contains "$FRAME_BODY" "Card number" "iframe document converted, not the parent"
assert_not_contains "$FRAME_BODY" "Markdown Fixture" "parent article excluded from frame scope"

pt_post /frame -d '{"target":"main"}'
assert_ok "reset frame scope"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "markdown: unknown mode is refused with a 400"

pt_get "/text?mode=bogus"
assert_http_status 400 "unknown mode refused"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "markdown: IDPI guard applies as on the readability path"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-inject.html\"}"
assert_ok "navigate to idpi-inject.html"

# Warn mode (default api service): the request is answered and the advisory
# rides on the X-IDPI-Warning header and the idpiWarning body field, identical
# to the readability path on the same page.
TMPH=$(mktemp)
RESPONSE=$(e2e_curl -s -w "\n%{http_code}" -D "$TMPH" "${E2E_SERVER}/text?mode=markdown")
MD_STATUS=$(echo "$RESPONSE" | tail -n 1)
MD_BODY=$(echo "$RESPONSE" | head -n -1)
MD_WARN_HDR=$(grep -i "^X-IDPI-Warning:" "$TMPH" | sed 's/^[^:]*: *//' | tr -d '\r' | head -1)
rm -f "$TMPH"

HTTP_STATUS="$MD_STATUS"; assert_http_status 200 "markdown returns in warn mode"
if [ -n "$MD_WARN_HDR" ]; then
  pass_assert "X-IDPI-Warning header present"
else
  fail_assert "X-IDPI-Warning header missing"
fi
assert_json_exists "$MD_BODY" ".idpiWarning" "idpiWarning field in markdown body"

# Parity: the readability path warns on the same page.
TMPH2=$(mktemp)
e2e_curl -s -D "$TMPH2" "${E2E_SERVER}/text" >/dev/null
RD_WARN_HDR=$(grep -i "^X-IDPI-Warning:" "$TMPH2" | sed 's/^[^:]*: *//' | tr -d '\r' | head -1)
rm -f "$TMPH2"
if [ -n "$RD_WARN_HDR" ]; then
  pass_assert "readability path warns identically on the same page"
else
  fail_assert "readability path did not warn; guard parity broken"
fi

end_test
