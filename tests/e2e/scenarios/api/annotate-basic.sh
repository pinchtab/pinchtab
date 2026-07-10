#!/bin/bash
# annotate-basic.sh — persistent, clickable annotation overlay.
#
# Covers the public contract of GET /annotate (and ?clear=true):
#   - JSON envelope shape (annotated count + annotations array)
#   - the persistent __pinchtab_annotations_interactive__ overlay is injected
#     and, unlike the screenshot overlay, STAYS on the page
#   - clicking a label copies a reference block (page + ref + CSS + XPath) —
#     verified by driving the label's click handler and reading what it staged
#   - returned ref is usable for follow-up actions in the same tab
#   - ?clear=true removes the overlay

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

OVERLAY_ID='__pinchtab_annotations_interactive__'

# ─────────────────────────────────────────────────────────────────
start_test "annotate: JSON envelope shape"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"

pt_get "/annotate"
assert_ok "annotate inject"

assert_result_exists '.annotated'   'envelope.annotated present'
assert_result_exists '.annotations' 'envelope.annotations present'
assert_json_length_gte "$RESULT" '.annotations' 1 'has at least one annotation'
assert_result_jq \
  '.annotations[0].ref | test("^[a-z]+[0-9]+$")' \
  'annotations[0].ref looks like e<digits>' \
  'annotations[0].ref is malformed'

FIRST_REF=$(echo "$RESULT" | jq -r '.annotations[0].ref')

end_test

# ─────────────────────────────────────────────────────────────────
start_test "annotate: overlay persists on the live page"

# Opposite of the screenshot overlay: this one must remain after the call so a
# human can look at it and click a label.
pt_post /evaluate -d "{\"expression\":\"document.getElementById('${OVERLAY_ID}') !== null\"}"
assert_ok "evaluate overlay-present probe"
assert_result_eq '.result' 'true' 'persistent overlay node present after inject'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "annotate: clicking a label copies a reference block"

# Drive the label's real click handler (same code path as a human click) and
# read window.__ptLastCopied, which the handler stages synchronously before the
# async clipboard write. This is the "click the item and get the data" contract.
CLICK_EXPR="(function(){var r=document.getElementById('${OVERLAY_ID}');if(!r)return 'NO_OVERLAY';var t=null;r.querySelectorAll('div').forEach(function(l){if(l.textContent==='${FIRST_REF}')t=l;});if(!t)return 'NO_LABEL';t.dispatchEvent(new MouseEvent('click',{bubbles:true}));return window.__ptLastCopied||'NO_COPY';})()"
CLICK_PAYLOAD=$(jq -nc --arg e "$CLICK_EXPR" '{expression:$e}')

pt_post /evaluate -d "$CLICK_PAYLOAD"
assert_ok "evaluate label click"

# The copied block must carry every field an LLM needs to locate the element.
assert_result_jq \
  ".result | contains(\"Element: ${FIRST_REF}\")" \
  'copied block names the clicked ref' \
  'copied block missing the ref'
assert_result_jq \
  '.result | contains("Page: ")' \
  'copied block includes the page title/url' \
  'copied block missing page context'
assert_result_jq \
  '.result | contains("CSS: ")' \
  'copied block includes a CSS selector' \
  'copied block missing CSS selector'
assert_result_jq \
  '.result | contains("XPath: ")' \
  'copied block includes an XPath' \
  'copied block missing XPath'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "annotate: returned ref is clickable"

# annotate seeds the tab ref cache, so a follow-up action resolves the ref
# without a separate snapshot.
if [ -n "$FIRST_REF" ] && [ "$FIRST_REF" != "null" ]; then
  pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${FIRST_REF}\"}"
  if echo "$RESULT" | grep -qi "ref.*not.*found\|unknown.*ref"; then
    fail_assert "ref ${FIRST_REF} not recognised after annotate"
  else
    pass_assert "ref ${FIRST_REF} resolved by click handler"
  fi
else
  fail_assert "no ref returned to click"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "annotate: clear removes the overlay"

# Re-inject first (the click test may have navigated the ref's target away).
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
pt_get "/annotate"
assert_ok "annotate re-inject before clear"

pt_get "/annotate?clear=true"
assert_ok "annotate clear"
assert_result_eq '.cleared' 'true' 'clear returns cleared=true'

pt_post /evaluate -d "{\"expression\":\"document.getElementById('${OVERLAY_ID}') === null\"}"
assert_ok "evaluate overlay-removed probe"
assert_result_eq '.result' 'true' 'overlay node removed after clear'

end_test
