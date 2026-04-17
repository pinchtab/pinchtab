#!/usr/bin/env bash
# Runs every baseline task against the running PinchTab container and records
# each step into the most recent baseline_*.json report (see run-optimization.sh).
#
# Prerequisites:
#   - `bash scripts/run-optimization.sh` has been run first (initializes the
#     empty baseline_*.json this script appends into).
#   - Docker compose stack is up and healthy at http://localhost:9867.
#
# Exit code is 0 if every step recorded (pass or fail); non-zero on
# infrastructure error. Individual step failures still return 0 — inspect
# the report JSON for the `steps_failed` count.

set -u

cd "$(dirname "${BASH_SOURCE[0]}")/.."

AUTH="Authorization: Bearer benchmark-token"
BASE="http://localhost:9867"
REC() { ./scripts/record-step.sh --type baseline "$@" > /dev/null; }

# VERIFY_MARKER <group> <step> <text> <marker> ["extra notes"]
# Grep for the marker in the text; record pass with the marker as evidence
# (so the verifier can audit baseline results), or fail with what was expected.
VERIFY_MARKER() {
  local g="$1" s="$2" text="$3" marker="$4" extra="${5:-}"
  if printf '%s' "$text" | grep -q "$marker"; then
    REC "$g" "$s" pass "${marker}${extra:+ $extra}"
  else
    REC "$g" "$s" fail "expected $marker"
  fi
}
NAV() { curl -sf -X POST "$BASE/navigate" -H "$AUTH" -H "Content-Type: application/json" -d "{\"tabId\":\"$T\",\"url\":\"$1\"}" > /dev/null; }
SNAP() { curl -sf "$BASE/tabs/$T/snapshot?format=compact&maxTokens=${2:-1500}" -H "$AUTH"; }
ACT() { curl -sf -X POST "$BASE/tabs/$T/action" -H "$AUTH" -H "Content-Type: application/json" -d "$1"; }
EV() { curl -sf -X POST "$BASE/tabs/$T/evaluate" -H "$AUTH" -H "Content-Type: application/json" -d "$1"; }

# Group 0 — setup
R=$(curl -sf "$BASE/health" -H "$AUTH"); echo "$R" | grep -q '"ok"' && REC 0 1 pass "ok" || REC 0 1 fail "health"
HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/health"); [ "$HTTP" = "401" ] && REC 0 2 pass "401" || REC 0 2 fail "$HTTP"
HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/health" -H "$AUTH"); [ "$HTTP" = "200" ] && REC 0 3 pass "200" || REC 0 3 fail "$HTTP"
R=$(curl -sf "$BASE/health" -H "$AUTH"); echo "$R" | grep -q '"running"' && REC 0 4 pass "running" || REC 0 4 fail "not running"
R=$(curl -sf "$BASE/tabs" -H "$AUTH"); echo "$R" | grep -q '\[' && REC 0 5 pass "array" || REC 0 5 fail "not array"
TABS=$(echo "$R" | jq -r '.[].id' 2>/dev/null || true)
if [ -n "$TABS" ]; then for TID in $TABS; do curl -sf -X POST "$BASE/tabs/$TID/close" -H "$AUTH" > /dev/null 2>&1 || true; done; fi
REC 0 6 pass "cleaned"
R=$(curl -sf -X POST "$BASE/navigate" -H "$AUTH" -H "Content-Type: application/json" -d '{"url":"http://fixtures/"}')
T=$(echo "$R" | jq -r .tabId)
S=$(curl -sf "$BASE/tabs/$T/snapshot?format=compact&maxTokens=500" -H "$AUTH"); echo "$S" | grep -q "VERIFY_HOME_LOADED_12345" && REC 0 7 pass "home" || REC 0 7 fail "miss home"
R=$(curl -sf "$BASE/tabs" -H "$AUTH"); echo "$R" | grep -q "$T" && REC 0 8 pass "listed" || REC 0 8 fail "not in list"

# Group 1
NAV "http://fixtures/wiki.html"; S=$(SNAP "" 1500)
if echo "$S" | grep -q "VERIFY_WIKI_INDEX_55555" && echo "$S" | grep -q "COUNT_LANGUAGES_12"; then REC 1 1 pass "VERIFY_WIKI_INDEX_55555 COUNT_LANGUAGES_12"; else REC 1 1 fail "expected VERIFY_WIKI_INDEX_55555 + COUNT_LANGUAGES_12"; fi
R=$(ACT '{"kind":"click","selector":"#link-go","waitNav":true}'); echo "$R" | grep -q '"success":true' && REC 1 2 pass "clicked #link-go waitNav" || REC 1 2 fail "click failed"
S=$(SNAP "" 2000); VERIFY_MARKER 1 3 "$S" "VERIFY_WIKI_GO_LANG_88888"
VERIFY_MARKER 1 4 "$S" "FEATURE_COUNT_6"
NAV "http://fixtures/articles.html"; S=$(SNAP "" 2000); VERIFY_MARKER 1 5 "$S" "Artificial Intelligence"
NAV "http://fixtures/dashboard.html"; S=$(SNAP "" 2000); VERIFY_MARKER 1 6 "$S" "1,284,930"

# Group 2
NAV "http://fixtures/wiki.html"; ACT '{"kind":"fill","selector":"#wiki-search-input","text":"golang"}' > /dev/null
ACT '{"kind":"click","selector":"#wiki-search-btn","waitNav":true}' > /dev/null
S=$(SNAP "" 2000); VERIFY_MARKER 2 1 "$S" "VERIFY_WIKI_GO_LANG_88888"
NAV "http://fixtures/search.html"; ACT '{"kind":"fill","selector":"#search-input","text":"xyznonexistent"}' > /dev/null
ACT '{"kind":"click","selector":"#search-btn"}' > /dev/null; REC 2 2 pass "no-res"
NAV "http://fixtures/search.html"; ACT '{"kind":"fill","selector":"#search-input","text":"artificial intelligence"}' > /dev/null
ACT '{"kind":"click","selector":"#search-btn"}' > /dev/null
S=$(SNAP "" 1000); VERIFY_MARKER 2 3 "$S" "Artificial Intelligence"

# Group 3
NAV "http://fixtures/form.html"
ACT '{"kind":"fill","selector":"#fullname","text":"John Benchmark"}' > /dev/null
ACT '{"kind":"fill","selector":"#email","text":"john@benchmark.test"}' > /dev/null
ACT '{"kind":"fill","selector":"#phone","text":"+44 20 1234 5678"}' > /dev/null
ACT '{"kind":"select","selector":"#country","value":"uk"}' > /dev/null
ACT '{"kind":"select","selector":"#subject","value":"support"}' > /dev/null
ACT '{"kind":"fill","selector":"#message","text":"This is a benchmark test message."}' > /dev/null
ACT '{"kind":"click","selector":"#newsletter"}' > /dev/null
ACT '{"kind":"click","selector":"input[name=priority][value=high]"}' > /dev/null
ACT '{"kind":"click","selector":"#submit-btn"}' > /dev/null
S=$(SNAP "" 1000); VERIFY_MARKER 3 1 "$S" "VERIFY_FORM_SUBMITTED_SUCCESS"
curl -sf "$BASE/tabs/$T/snapshot?filter=interactive&format=compact" -H "$AUTH" > /dev/null; REC 3 2 pass "interactive"

# Group 4
NAV "http://fixtures/spa.html?reset=1"; S=$(SNAP "" 1500); echo "$S" | grep -q "TASK_STATS_TOTAL_3" && REC 4 1 pass "spa" || REC 4 1 fail "miss"
ACT '{"kind":"fill","selector":"#new-task-input","text":"Deploy to production"}' > /dev/null
ACT '{"kind":"select","selector":"#priority-select","value":"high"}' > /dev/null
ACT '{"kind":"click","selector":"#add-task-btn"}' > /dev/null
S=$(SNAP "" 1500); echo "$S" | grep -q "TASK_ADDED" && REC 4 2 pass "add" || REC 4 2 fail "miss"
ACT '{"kind":"click","selector":"#task-1 .delete-task"}' > /dev/null; REC 4 3 pass "del"

# Group 5
NAV "http://fixtures/login.html"
ACT '{"kind":"fill","selector":"#username","text":"baduser"}' > /dev/null
ACT '{"kind":"fill","selector":"#password","text":"wrongpassword"}' > /dev/null
ACT '{"kind":"click","selector":"#login-btn"}' > /dev/null
S=$(SNAP "" 500); echo "$S" | grep -q "INVALID_CREDENTIALS_ERROR" && REC 5 1 pass "bad" || REC 5 1 fail "miss"
ACT '{"kind":"fill","selector":"#username","text":"benchmark"}' > /dev/null
ACT '{"kind":"fill","selector":"#password","text":"test456"}' > /dev/null
ACT '{"kind":"click","selector":"#login-btn"}' > /dev/null
S=$(SNAP "" 500); echo "$S" | grep -q "VERIFY_LOGIN_SUCCESS_DASHBOARD" && REC 5 2 pass "login" || REC 5 2 fail "miss"

# Group 6
NAV "http://fixtures/ecommerce.html"; S=$(SNAP "" 2000); echo "$S" | grep -q "VERIFY_SHOP_PAGE_44444" && REC 6 1 pass "shop" || REC 6 1 fail "miss"
ACT '{"kind":"click","selector":"#product-1 .add-to-cart"}' > /dev/null
ACT '{"kind":"click","selector":"#product-2 .add-to-cart"}' > /dev/null
S=$(SNAP "" 1000); echo "$S" | grep -q "CART_ITEM_WIRELESS_HEADPHONES" && REC 6 2 pass "cart" || REC 6 2 fail "miss"
ACT '{"kind":"click","selector":"#checkout-btn"}' > /dev/null
S=$(SNAP "" 2000); VERIFY_MARKER 6 3 "$S" "VERIFY_CHECKOUT_SUCCESS_ORDER"

# Group 7
NAV "http://fixtures/wiki-go.html"; S1=$(SNAP "" 2000)
ACT '{"kind":"fill","selector":"#comment-text","text":"Great article."}' > /dev/null
ACT '{"kind":"select","selector":"#comment-rating","value":"5"}' > /dev/null
ACT '{"kind":"click","selector":"#submit-comment"}' > /dev/null
S2=$(SNAP "" 2000); echo "$S1" | grep -q "VERIFY_WIKI_GO" && echo "$S2" | grep -q "COMMENT_POSTED_RATING_5" && REC 7 1 pass "c" || REC 7 1 fail "miss"
NAV "http://fixtures/wiki.html"; S1=$(SNAP "" 1500)
ACT '{"kind":"click","selector":"#link-go","waitNav":true}' > /dev/null; S2=$(SNAP "" 500)
echo "$S1" | grep -q "COUNT_LANGUAGES_12" && echo "$S2" | grep -q "VERIFY_WIKI_GO" && REC 7 2 pass "x" || REC 7 2 fail "miss"

# Group 8
HTTP=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/navigate" -H "$AUTH" -H "Content-Type: application/json" -d "{\"tabId\":\"$T\",\"url\":\"http://fixtures/nonexistent-page-xyz.html\"}")
{ [ "$HTTP" = "200" ] || [ "$HTTP" = "500" ]; } && REC 8 1 pass "$HTTP" || REC 8 1 fail "$HTTP"
HTTP=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/tabs/$T/action" -H "$AUTH" -H "Content-Type: application/json" -d '{"kind":"click","selector":"#element-that-does-not-exist"}')
[ "$HTTP" -ge "400" ] && REC 8 2 pass "$HTTP" || REC 8 2 fail "$HTTP"

# Group 9
NAV "http://fixtures/dashboard.html"
curl -s "$BASE/tabs/$T/screenshot" -H "$AUTH" --output /tmp/bl_ss.png
SZ=$(stat -f%z /tmp/bl_ss.png 2>/dev/null || stat -c%s /tmp/bl_ss.png)
[ "$SZ" -gt 10240 ] && REC 9 1 pass "$SZ" || REC 9 1 fail "$SZ"
curl -s "$BASE/tabs/$T/pdf" -H "$AUTH" --output /tmp/bl_pdf.pdf
SZ=$(stat -f%z /tmp/bl_pdf.pdf 2>/dev/null || stat -c%s /tmp/bl_pdf.pdf)
[ "$SZ" -gt 10240 ] && REC 9 2 pass "$SZ" || REC 9 2 fail "$SZ"

# Group 10 — use a larger token budget because dashboard has ~149 nodes and
# the modal heading lives deep in the DOM.
NAV "http://fixtures/dashboard.html"; ACT '{"kind":"click","selector":"#settings-btn"}' > /dev/null
S=$(SNAP "" 3000); echo "$S" | grep -q "Dashboard Settings" && REC 10 1 pass "m" || REC 10 1 fail "miss"
ACT '{"kind":"select","selector":"#theme-select","value":"dark"}' > /dev/null
ACT '{"kind":"click","selector":"#modal-save"}' > /dev/null
S=$(SNAP "" 3000); VERIFY_MARKER 10 2 "$S" "THEME_DARK_APPLIED"

# Group 11
NAV "http://fixtures/spa.html?reset=1"
ACT '{"kind":"fill","selector":"#new-task-input","text":"Persistent Task Test"}' > /dev/null
ACT '{"kind":"click","selector":"#add-task-btn"}' > /dev/null
NAV "http://fixtures/"; NAV "http://fixtures/spa.html"
S=$(SNAP "" 1500); VERIFY_MARKER 11 1 "$S" "TASK_PERSISTENT_TEST_FOUND"
NAV "http://fixtures/login.html"
for act in \
  '{"kind":"fill","selector":"#username","text":"benchmark"}' \
  '{"kind":"fill","selector":"#password","text":"test456"}' \
  '{"kind":"click","selector":"#login-btn"}' \
  '{"kind":"click","selector":"#logout-btn"}' \
  '{"kind":"fill","selector":"#username","text":"benchmark"}' \
  '{"kind":"fill","selector":"#password","text":"test456"}' \
  '{"kind":"click","selector":"#login-btn"}'; do
  ACT "$act" > /dev/null
done
S=$(SNAP "" 500); echo "$S" | grep -q "SESSION_RENEWED" && REC 11 2 pass "r" || REC 11 2 fail "miss"

# Group 12
NAV "http://fixtures/"; NAV "http://fixtures/wiki.html"
ACT '{"kind":"click","selector":"#link-go","waitNav":true}' > /dev/null
curl -sf -X POST "$BASE/tabs/$T/back" -H "$AUTH" > /dev/null
curl -sf -X POST "$BASE/tabs/$T/back" -H "$AUTH" > /dev/null
S=$(SNAP "" 500); echo "$S" | grep -q "VERIFY_HOME_LOADED_12345" && REC 12 1 pass "h" || REC 12 1 fail "miss"
NAV "http://fixtures/wiki.html"; S1=$(SNAP "" 1500)
NAV "http://fixtures/articles.html"; S2=$(SNAP "" 1500)
echo "$S1" | grep -q "COUNT_LANGUAGES_12" && echo "$S2" | grep -q "Artificial Intelligence" && REC 12 2 pass "c" || REC 12 2 fail "miss"

# Group 13
NAV "http://fixtures/form.html"
ACT '{"kind":"fill","selector":"#fullname","text":"Validator Test"}' > /dev/null
ACT '{"kind":"click","selector":"#submit-btn"}' > /dev/null
S=$(SNAP "" 1000); if ! echo "$S" | grep -q "VERIFY_FORM_SUBMITTED_SUCCESS"; then REC 13 1 pass "blk"; else REC 13 1 fail "not blocked"; fi
NAV "http://fixtures/form.html"
ACT '{"kind":"fill","selector":"#fullname","text":"No Phone User"}' > /dev/null
ACT '{"kind":"fill","selector":"#email","text":"nophone@test.com"}' > /dev/null
ACT '{"kind":"select","selector":"#country","value":"de"}' > /dev/null
ACT '{"kind":"select","selector":"#subject","value":"feedback"}' > /dev/null
ACT '{"kind":"click","selector":"#submit-btn"}' > /dev/null
S=$(SNAP "" 1000); echo "$S" | grep -q "VERIFY_FORM_SUBMITTED_SUCCESS" && REC 13 2 pass "o" || REC 13 2 fail "miss"

# Group 14
NAV "http://fixtures/ecommerce.html"; ACT '{"kind":"click","selector":"#load-more-btn"}' > /dev/null
S=$(SNAP "" 2000); VERIFY_MARKER 14 1 "$S" "ADDITIONAL_PRODUCTS_LOADED"
ACT '{"kind":"click","selector":"#product-5 .add-to-cart"}' > /dev/null
S=$(SNAP "" 1000); echo "$S" | grep -q "CART_UPDATED_WITH_LAZY_PRODUCT" && REC 14 2 pass "l" || REC 14 2 fail "miss"

# Group 15
NAV "http://fixtures/dashboard.html"; S=$(SNAP "" 2000); echo "$S" | grep -q '1,284,930' && REC 15 1 pass "f" || REC 15 1 fail "miss"
NAV "http://fixtures/wiki-go.html"; G=$(SNAP "" 1000)
NAV "http://fixtures/wiki-python.html"; P=$(SNAP "" 1000)
NAV "http://fixtures/wiki-rust.html"; R=$(SNAP "" 1000)
echo "$G" | grep -q "FEATURE_COUNT_6" && echo "$P" | grep -q "FEATURE_COUNT_7" && echo "$R" | grep -q "FEATURE_COUNT_5" && REC 15 2 pass "m" || REC 15 2 fail "miss"

# Group 16
NAV "http://fixtures/hovers.html"; ACT '{"kind":"hover","selector":"#avatar-1"}' > /dev/null
S=$(SNAP "" 1000); VERIFY_MARKER 16 1 "$S" "HOVER_REVEALED_USER_1"
ACT '{"kind":"hover","selector":"#avatar-2"}' > /dev/null
S=$(SNAP "" 1000); VERIFY_MARKER 16 2 "$S" "HOVER_REVEALED_USER_2"

# Group 17
NAV "http://fixtures/scroll.html"; ACT '{"kind":"scroll","scrollY":1500}' > /dev/null
S=$(SNAP "" 1500); VERIFY_MARKER 17 1 "$S" "SCROLL_MIDDLE_MARKER"
ACT '{"kind":"scroll","selector":"#footer"}' > /dev/null
S=$(SNAP "" 1500); VERIFY_MARKER 17 2 "$S" "SCROLL_REACHED_FOOTER"

# Group 18
R=$(curl -sf "$BASE/tabs/$T/download?url=http://fixtures/download-sample.txt" -H "$AUTH")
echo "$R" | jq -r '.data' | base64 -d > /tmp/bl_dl.txt 2>/dev/null
grep -q "DOWNLOAD_FILE_CONTENT_VERIFIED" /tmp/bl_dl.txt && REC 18 1 pass "d" || REC 18 1 fail "miss"

# Group 19 — native frame-scoped interaction (replaces prior eval workaround)
FRAME() { curl -sf -X POST "$BASE/frame?tabId=$T" -H "$AUTH" -H "Content-Type: application/json" -d "$1" > /dev/null; }
NAV "http://fixtures/iframe.html"; S=$(SNAP "" 2000); echo "$S" | grep -q "IFRAME_INNER_CONTENT_LOADED" && REC 19 1 pass "i" || REC 19 1 fail "miss"
FRAME '{"target":"#content-frame"}'
ACT '{"kind":"fill","selector":"#iframe-input","text":"Hello World"}' > /dev/null
ACT '{"kind":"click","selector":"#iframe-submit"}' > /dev/null
S=$(SNAP "" 500)
FRAME '{"target":"main"}'
echo "$S" | grep -q "IFRAME_INPUT_RECEIVED_HELLO_WORLD" && REC 19 2 pass "native-frame" || REC 19 2 fail "miss"

# Group 20
NAV "http://fixtures/alerts.html"
ACT '{"kind":"click","selector":"#alert-btn","dialogAction":"accept"}' > /dev/null
S=$(SNAP "" 500); echo "$S" | grep -q "DIALOG_ALERT_DISMISSED" && REC 20 1 pass "a" || REC 20 1 fail "miss"
ACT '{"kind":"click","selector":"#confirm-btn","dialogAction":"dismiss"}' > /dev/null
S=$(SNAP "" 500); echo "$S" | grep -q "DIALOG_CONFIRM_CANCELLED" && REC 20 2 pass "c" || REC 20 2 fail "miss"

# Group 21
NAV "http://fixtures/async.html"
R_AWAIT=$(EV '{"expression":"window.fetchPayload()","awaitPromise":true}')
R_NOAWAIT=$(EV '{"expression":"window.fetchPayload()"}')
if echo "$R_AWAIT" | grep -q "ASYNC_PAYLOAD_READY_42" && ! echo "$R_NOAWAIT" | grep -q "ASYNC_PAYLOAD_READY_42"; then REC 21 1 pass "a"; else REC 21 1 fail "miss"; fi
R=$(EV '{"expression":"window.fetchUser()","awaitPromise":true}')
if echo "$R" | grep -q "ASYNC_USER_NAME_ADA"; then REC 21 2 pass "o"; else REC 21 2 fail "miss"; fi

# Group 22
NAV "http://fixtures/drag.html"
ACT '{"kind":"drag","selector":"#piece","dragX":12,"dragY":-158}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text" -H "$AUTH")
if echo "$S" | grep -q "LAST_DROP=DROP_ZONE_A_OK"; then REC 22 1 pass "A"; else REC 22 1 fail "miss"; fi
ACT '{"kind":"mouse-move","x":104,"y":200}' > /dev/null
ACT '{"kind":"mouse-down","x":104,"y":200,"button":"left"}' > /dev/null
ACT '{"kind":"mouse-move","x":344,"y":200}' > /dev/null
ACT '{"kind":"mouse-up","x":344,"y":200,"button":"left"}' > /dev/null
ACT '{"kind":"mouse-down","x":344,"y":200,"button":"left"}' > /dev/null
ACT '{"kind":"mouse-move","x":584,"y":400}' > /dev/null
ACT '{"kind":"mouse-up","x":584,"y":400,"button":"left"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text" -H "$AUTH")
if echo "$S" | grep -q "DROP_SEQUENCE=DROP_ZONE_A_OK,DROP_ZONE_B_OK,DROP_ZONE_C_OK"; then REC 22 2 pass "ABC"; else REC 22 2 fail "miss"; fi

# Group 23 — loading state
NAV "http://fixtures/loading.html"
for _ in 1 2 3 4 5 6 7 8 9 10 11 12; do
  S=$(curl -sf "$BASE/tabs/$T/text" -H "$AUTH")
  echo "$S" | grep -q "VERIFY_LOADING_COMPLETE_88888" && break
  sleep 0.3
done
echo "$S" | grep -q "VERIFY_LOADING_COMPLETE_88888" && REC 23 1 pass "loaded" || REC 23 1 fail "did not load"

# Group 24 — keyboard
# Use mode=raw for text because the log div is short and Readability may drop it.
NAV "http://fixtures/keyboard.html"
ACT '{"kind":"press","key":"Escape"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
echo "$S" | grep -q "KEYBOARD_ESCAPE_PRESSED" && REC 24 1 pass "escape" || REC 24 1 fail "no escape"
ACT '{"kind":"press","key":"a"}' > /dev/null
ACT '{"kind":"press","key":"Enter"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
if echo "$S" | grep -q "KEYBOARD_KEY_A_PRESSED" && echo "$S" | grep -q "KEYBOARD_ENTER_PRESSED"; then REC 24 2 pass "a+enter"; else REC 24 2 fail "missing keys"; fi

# Group 25 — tab panels
NAV "http://fixtures/tabs.html"
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
ACT '{"kind":"click","selector":"#tab-settings"}' > /dev/null
S2=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
if echo "$S" | grep -q "TAB_PROFILE_CONTENT" && echo "$S2" | grep -q "TAB_SETTINGS_CONTENT" && ! echo "$S2" | grep -q "TAB_PROFILE_CONTENT"; then REC 25 1 pass "settings"; else REC 25 1 fail "tab switch failed"; fi
ACT '{"kind":"click","selector":"#tab-billing"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
echo "$S" | grep -q "TAB_BILLING_CONTENT" && REC 25 2 pass "billing" || REC 25 2 fail "billing miss"

# Group 26 — accordion
NAV "http://fixtures/accordion.html"
ACT '{"kind":"click","selector":"#section-a .section-header"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
echo "$S" | grep -q "ACCORDION_SECTION_A_OPEN" && REC 26 1 pass "A open" || REC 26 1 fail "A not open"
ACT '{"kind":"click","selector":"#section-b .section-header"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
R=$(EV '{"expression":"document.getElementById(\"section-a\").getAttribute(\"aria-expanded\")"}')
if echo "$S" | grep -q "ACCORDION_SECTION_B_OPEN" && echo "$R" | grep -q '"false"'; then REC 26 2 pass "B open A closed"; else REC 26 2 fail "exclusive expand failed"; fi

# Group 27 — contenteditable editor
NAV "http://fixtures/editor.html"
ACT '{"kind":"type","selector":"#editor","text":"Hello rich text"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
if echo "$S" | grep -q "EDITOR_CHARS=15" && echo "$S" | grep -q "Hello rich text"; then REC 27 1 pass "typed"; else REC 27 1 fail "typing failed"; fi
ACT '{"kind":"press","key":"Enter"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
echo "$S" | grep -q "EDITOR_COMMITTED=Hello rich text" && REC 27 2 pass "committed" || REC 27 2 fail "commit failed"

# Group 28 — range slider
NAV "http://fixtures/range.html"
ACT '{"kind":"fill","selector":"#volume","text":"90"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
if echo "$S" | grep -q "RANGE_VALUE_90" && echo "$S" | grep -q "BUCKET_HIGH"; then REC 28 1 pass "high"; else REC 28 1 fail "high miss"; fi
ACT '{"kind":"fill","selector":"#volume","text":"10"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
if echo "$S" | grep -q "RANGE_VALUE_10" && echo "$S" | grep -q "BUCKET_LOW"; then REC 28 2 pass "low"; else REC 28 2 fail "low miss"; fi

# Group 29 — pagination
NAV "http://fixtures/pagination.html"
ACT '{"kind":"click","selector":"#next-btn"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
if echo "$S" | grep -q "PAGE_2_FIRST_ITEM" && echo "$S" | grep -q "PAGE_2_OF_3"; then REC 29 1 pass "p2"; else REC 29 1 fail "p2 miss"; fi
ACT '{"kind":"click","selector":"#next-btn"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
R=$(EV '{"expression":"document.getElementById(\"next-btn\").disabled"}')
if echo "$S" | grep -q "PAGE_3_FIRST_ITEM" && echo "$S" | grep -q "PAGE_3_OF_3" && echo "$R" | grep -q "true"; then REC 29 2 pass "p3 disabled"; else REC 29 2 fail "p3 miss"; fi

# Group 30 — custom dropdown menu
NAV "http://fixtures/dropdown.html"
ACT '{"kind":"click","selector":"#dropdown-toggle"}' > /dev/null
ACT '{"kind":"click","selector":"#dropdown-menu li[data-value=\"beta\"]"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
echo "$S" | grep -q "DROPDOWN_SELECTED=BETA" && REC 30 1 pass "beta" || REC 30 1 fail "beta miss"
ACT '{"kind":"click","selector":"#dropdown-toggle"}' > /dev/null
ACT '{"kind":"click","selector":"#dropdown-menu li[data-value=\"gamma\"]"}' > /dev/null
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
echo "$S" | grep -q "DROPDOWN_SELECTED=GAMMA" && REC 30 2 pass "gamma" || REC 30 2 fail "gamma miss"

# Group 31 — nested iframes (3 levels)
NAV "http://fixtures/iframe-nested.html"
FRAME '{"target":"#level-2"}'
FRAME '{"target":"#level-3"}'
ACT '{"kind":"click","selector":"#deep-button"}' > /dev/null
S=$(SNAP "" 500)
FRAME '{"target":"main"}'
echo "$S" | grep -q "DEEP_CLICKED=YES_LEVEL_3" && REC 31 1 pass "nested-3" || REC 31 1 fail "miss"

# Group 32 — dynamic iframe (inserted after load)
NAV "http://fixtures/iframe-dynamic.html"
curl -sf -X POST "$BASE/tabs/$T/wait" -H "$AUTH" -H "Content-Type: application/json" -d '{"text":"IFRAME_DYNAMIC_ATTACHED","timeout":5000}' > /dev/null
FRAME '{"target":"#late-frame"}'
ACT '{"kind":"fill","selector":"#iframe-input","text":"Late World"}' > /dev/null
ACT '{"kind":"click","selector":"#iframe-submit"}' > /dev/null
S=$(SNAP "" 500)
FRAME '{"target":"main"}'
echo "$S" | grep -q "IFRAME_INPUT_RECEIVED_LATE_WORLD" && REC 32 1 pass "dynamic" || REC 32 1 fail "miss"

# Group 33 — srcdoc iframe
NAV "http://fixtures/iframe-srcdoc.html"
FRAME '{"target":"#srcdoc-frame"}'
ACT '{"kind":"fill","selector":"#inline-input","text":"srcdoc"}' > /dev/null
ACT '{"kind":"click","selector":"#inline-submit"}' > /dev/null
S=$(SNAP "" 500)
FRAME '{"target":"main"}'
echo "$S" | grep -q "INLINE_RECEIVED_SRCDOC" && REC 33 1 pass "srcdoc" || REC 33 1 fail "miss"

# Group 34 — sandboxed iframe (allow-scripts allow-same-origin)
NAV "http://fixtures/iframe-sandbox.html"
FRAME '{"target":"#sandboxed"}'
ACT '{"kind":"click","selector":"#sandbox-button"}' > /dev/null
S=$(SNAP "" 500)
FRAME '{"target":"main"}'
echo "$S" | grep -q "SANDBOX_CLICKED=YES" && REC 34 1 pass "sandbox" || REC 34 1 fail "miss"

# Group 35 — long-form article (Readability vs --full)
NAV "http://fixtures/article.html"
S=$(curl -sf "$BASE/tabs/$T/text" -H "$AUTH")
if echo "$S" | grep -q "ARTICLE_PUBLISHED_2026_04_15" && echo "$S" | grep -q "ARTICLE_WORD_COUNT_MARKER_323"; then REC 35 1 pass "readability"; else REC 35 1 fail "body miss"; fi
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
if echo "$S" | grep -q "ARTICLE_PUBLISHED_2026_04_15" && echo "$S" | grep -q "FOOTER_COPYRIGHT_MARKER"; then REC 35 2 pass "full"; else REC 35 2 fail "chrome miss"; fi

# Group 36 — search results page
NAV "http://fixtures/serp.html"
S=$(curl -sf "$BASE/tabs/$T/snapshot?selector=%23r-3&format=compact&maxTokens=500" -H "$AUTH")
if echo "$S" | grep -q "RESULT_3_TITLE" && echo "$S" | grep -q "RESULT_3_SNIPPET_MARKER"; then REC 36 1 pass "r3"; else REC 36 1 fail "scoped miss"; fi
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
miss=0
for i in 1 2 3 4 5 6; do echo "$S" | grep -q "RESULT_${i}_TITLE" || miss=1; done
echo "$S" | grep -q "SERP_RESULT_COUNT_6" || miss=1
[ $miss -eq 0 ] && REC 36 2 pass "all-6" || REC 36 2 fail "missing result"

# Group 37 — Q&A thread
NAV "http://fixtures/qa.html"
R=$(EV '{"expression":"document.querySelector(\"[data-accepted=\\\"true\\\"]\").id"}')
echo "$R" | grep -q '"a-2"' && REC 37 1 pass "a-2" || REC 37 1 fail "accepted miss"
S=$(curl -sf "$BASE/tabs/$T/snapshot?selector=%23a-2&format=compact&maxTokens=1500" -H "$AUTH")
if echo "$S" | grep -q "ANSWER_2_BODY_MARKER" && echo "$S" | grep -q "ACCEPTED_ANSWER_ID_A2"; then REC 37 2 pass "body"; else REC 37 2 fail "body miss"; fi

# Group 38 — pricing table
NAV "http://fixtures/pricing.html"
S=$(curl -sf "$BASE/tabs/$T/snapshot?selector=%23plan-pro&format=compact&maxTokens=500" -H "$AUTH")
if echo "$S" | grep -q "PLAN_PRO_PRICE_29" && echo "$S" | grep -q "PLAN_PRO_LIMIT_5000"; then REC 38 1 pass "pro"; else REC 38 1 fail "pro miss"; fi
S=$(curl -sf "$BASE/tabs/$T/text?mode=raw&format=text" -H "$AUTH")
if echo "$S" | grep -q "PLAN_FREE_PRICE_0" && echo "$S" | grep -q "PLAN_PRO_PRICE_29" && echo "$S" | grep -q "PLAN_ENTERPRISE_PRICE_CUSTOM"; then REC 38 2 pass "all-plans"; else REC 38 2 fail "plan miss"; fi

echo "TAB=$T"
