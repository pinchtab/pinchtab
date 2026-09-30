#!/bin/bash
# vocab-reepoch-basic.sh — every response that re-epochs a tab's ref cache
# publishes the new vocabulary token (X-PinchTab-Vocab) and the tab it belongs
# to (X-PinchTab-Tab-Id): /find, /tabs/{id}/find, /annotate,
# /screenshot?annotate, an action, batch or macro whose semantic selector
# refreshed the cache, and a semantic element read. A response that did not
# re-epoch publishes nothing. The published token is the one the tab's next /snapshot reports, an
# action echoing it is accepted, and a stale one is refused 409.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

PAGE="${FIXTURES_URL}/buttons.html"

# vocab_request METHOD PATH [curl args...] — like pinchtab, plus HDR_VOCAB and
# HDR_TAB from the response headers.
vocab_request() {
  local hdrs
  hdrs=$(mktemp)
  pinchtab "$@" -D "$hdrs"
  HDR_VOCAB=$(grep -i '^X-PinchTab-Vocab:' "$hdrs" | cut -d' ' -f2 | tr -d '\r')
  HDR_TAB=$(grep -i '^X-PinchTab-Tab-Id:' "$hdrs" | cut -d' ' -f2 | tr -d '\r')
  rm -f "$hdrs"
}

fresh_page() {
  pt_post /navigate -d "{\"url\":\"${PAGE}\"}"
  assert_ok "navigate to buttons.html"
  TAB_ID=$(get_tab_id)
}

assert_published() {
  local what="$1"
  if [ -n "$HDR_VOCAB" ]; then
    pass_assert "$what publishes X-PinchTab-Vocab $HDR_VOCAB"
  else
    fail_assert "$what publishes no X-PinchTab-Vocab"
  fi
  if [ "$HDR_TAB" = "$TAB_ID" ]; then
    pass_assert "$what X-PinchTab-Tab-Id names the navigated tab"
  else
    fail_assert "$what X-PinchTab-Tab-Id '$HDR_TAB', want '$TAB_ID'"
  fi
}

# assert_token_is_live TOKEN REF WHAT — the next snapshot reports TOKEN, a
# click echoing it is accepted and a stale token is refused 409.
assert_token_is_live() {
  local token="$1" ref="$2" what="$3"
  vocab_request GET "/snapshot?filter=interactive"
  assert_ok "snapshot after $what"
  if [ "$HDR_VOCAB" = "$token" ]; then
    pass_assert "next /snapshot reports the token $what published"
  else
    fail_assert "next /snapshot reports '$HDR_VOCAB', $what published '$token'"
  fi

  if [ -z "$ref" ] || [ "$ref" = "null" ]; then
    fail_assert "$what returned no ref for Increment"
    return
  fi
  pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${ref}\",\"vocab\":\"stale-pre-${what// /-}-token\"}"
  assert_http_status 409 "a pre-$what token is refused"
  pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${ref}\",\"vocab\":\"${token}\"}"
  assert_ok "click $ref echoing the token $what published is accepted"
}

# ─────────────────────────────────────────────────────────────────
start_test "vocab: POST /find on a fresh page publishes the token it minted"

fresh_page
vocab_request POST /find -d '{"query":"Increment"}'
assert_ok "find Increment"
assert_published "/find"
assert_token_is_live "$HDR_VOCAB" "$(echo "$RESULT" | jq -r '.best_ref')" "find"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "vocab: POST /tabs/{id}/find on a fresh page publishes the token it minted"

fresh_page
vocab_request POST "/tabs/${TAB_ID}/find" -d '{"query":"Increment"}'
assert_ok "tab-scoped find Increment"
assert_published "/tabs/{id}/find"
assert_token_is_live "$HDR_VOCAB" "$(echo "$RESULT" | jq -r '.best_ref')" "tab find"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "vocab: GET /annotate on a fresh page publishes the token it minted"

fresh_page
vocab_request GET /annotate
assert_ok "annotate"
assert_published "/annotate"
TOKEN="$HDR_VOCAB"
REF=$(echo "$RESULT" | jq -r 'first(.annotations[] | select(.name == "Increment") | .ref) // empty')
pt_get "/annotate?clear=true"
assert_token_is_live "$TOKEN" "$REF" "annotate"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "vocab: GET /screenshot?annotate on a fresh page publishes the token it minted"

fresh_page
vocab_request GET "/screenshot?annotate=true&format=png"
assert_ok "annotated screenshot"
assert_published "/screenshot?annotate"
TOKEN="$HDR_VOCAB"
REF=$(echo "$RESULT" | jq -r 'first(.annotations[] | select(.name == "Increment") | .ref) // empty')
assert_token_is_live "$TOKEN" "$REF" "annotated screenshot"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "vocab: an action whose semantic selector refreshed the cache publishes the token"

fresh_page
vocab_request POST /action -d '{"kind":"hover","selector":"semantic:Increment"}'
assert_ok "hover semantic:Increment"
assert_published "semantic hover"
TOKEN="$HDR_VOCAB"
vocab_request POST /find -d '{"query":"Increment"}'
REF=$(echo "$RESULT" | jq -r '.best_ref')
if [ "$HDR_VOCAB" = "$TOKEN" ]; then
  pass_assert "a find on the same DOM keeps the hover's token"
else
  fail_assert "find published '$HDR_VOCAB' after the hover published '$TOKEN'"
fi
assert_token_is_live "$TOKEN" "$REF" "semantic hover"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "vocab: POST /actions whose semantic step refreshed the cache publishes the token"

fresh_page
vocab_request POST /actions -d '{"actions":[{"kind":"hover","selector":"semantic:Increment"}]}'
assert_ok "batch hover semantic:Increment"
assert_json_eq "$RESULT" '.successful' '1' "the semantic batch step succeeded"
assert_published "semantic batch"
TOKEN="$HDR_VOCAB"
vocab_request POST /find -d '{"query":"Increment"}'
assert_token_is_live "$TOKEN" "$(echo "$RESULT" | jq -r '.best_ref')" "semantic batch"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "vocab: POST /macro whose semantic step refreshed the cache publishes the token"

fresh_page
vocab_request POST /macro -d '{"steps":[{"kind":"hover","selector":"semantic:Increment"}]}'
assert_ok "macro hover semantic:Increment"
assert_json_eq "$RESULT" '.successful' '1' "the semantic macro step succeeded"
assert_published "semantic macro"
TOKEN="$HDR_VOCAB"
vocab_request POST /find -d '{"query":"Increment"}'
assert_token_is_live "$TOKEN" "$(echo "$RESULT" | jq -r '.best_ref')" "semantic macro"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "vocab: a semantic element read publishes only the token its resolution minted"

fresh_page
vocab_request GET "/count?selector=semantic:Increment"
assert_ok "count semantic:Increment"
assert_published "semantic /count"
TOKEN="$HDR_VOCAB"

vocab_request GET "/attr?selector=semantic:Increment&name=id"
assert_ok "attr semantic:Increment on the now-current vocabulary"
assert_json_eq "$RESULT" '.value' 'increment' "attr resolved the Increment button"
if [ -z "$HDR_VOCAB" ]; then
  pass_assert "a semantic /attr that did not re-epoch publishes no token"
else
  fail_assert "a semantic /attr that did not re-epoch published '$HDR_VOCAB'"
fi

vocab_request POST /find -d '{"query":"Increment"}'
assert_token_is_live "$TOKEN" "$(echo "$RESULT" | jq -r '.best_ref')" "semantic count"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "vocab: a batch that did not re-epoch publishes no token"

fresh_page
vocab_request GET "/snapshot?filter=interactive"
assert_ok "snapshot"
TOKEN="$HDR_VOCAB"
vocab_request POST /actions -d '{"actions":[{"kind":"hover","selector":"semantic:Increment"}]}'
assert_ok "batch hover on the snapshotted vocabulary"
if [ -z "$HDR_VOCAB" ]; then
  pass_assert "a batch on an unchanged vocabulary publishes no token"
else
  fail_assert "a batch on an unchanged vocabulary published '$HDR_VOCAB' (snapshot token '$TOKEN')"
fi

end_test
