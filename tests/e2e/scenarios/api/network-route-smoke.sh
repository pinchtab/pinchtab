#!/bin/bash
# network-route-smoke.sh — End-to-end interception against a real Chrome.
# Verifies abort / fulfill / inverted-allowlist behaviour when a page
# actually issues fetches. Slower than the extended scenario; tagged "smoke".
#
# Cross-origin caveat: fulfill responses are deliberately not decorated with
# CORS headers (that would be the bypass we are guarding against). Under the
# default fetch mode 'cors', a missing ACAO would reject the page promise
# with a TypeError — indistinguishable from a real DNS failure — so the page
# shim uses mode:'no-cors' to get an opaque resolution instead. "Rule fired"
# is then detected by the absence of a thrown error. Same-origin fulfill
# (which under the inverted policy is always blocked, because the page must
# be on an allowlisted host) is verified by reading the response body.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

if [ -z "${E2E_FULL_SERVER:-}" ]; then
  echo "  E2E_FULL_SERVER not set, skipping network/route smoke scenarios"
  return 0 2>/dev/null || exit 0
fi

netroute_smoke_tests() {
  # Land on a known fixture so any subsequent fetches share that origin (the
  # fixtures host is in allowedDomains, which is what we exercise below).
  pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
  assert_ok "navigate to fixture"
  TAB_ID=$(get_tab_id)

  # Drive a fetch from inside the page and capture {ok,status,text} or {error}.
  # Uses mode:'no-cors' so a cross-origin fulfill (which we deliberately do NOT
  # decorate with ACAO headers) resolves opaquely instead of being rejected by
  # CORS — otherwise the page-level promise would throw a TypeError that is
  # indistinguishable from a real network failure, and we couldn't tell whether
  # the rule fired. Same-origin fetches (covered by the allowlisted-host case)
  # are unaffected: text() still returns the real body.
  fetch_in_page() {
    local url="$1"
    local expr
    expr=$(jq -n --arg u "$url" '
      {
        expression: ("(async () => { try { const r = await fetch(\($u | tojson), {mode:\"no-cors\"}); return JSON.stringify({ok:r.ok,status:r.status,type:r.type,text:(await r.text()).slice(0,200)}); } catch(e) { return JSON.stringify({error:String(e)}); } })()"),
        awaitPromise: true
      }')
    pt_post /evaluate -d "$expr"
  }

  # ─────────────────────────────────────────────────────────────────
  start_test "abort rule turns matching fetches into network errors"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"abort-target.invalid","action":"abort"}'
  assert_ok "install abort rule"

  fetch_in_page "https://abort-target.invalid/api"
  # fetch() throws TypeError on a network-level abort; the shim returns the
  # error string in that case.
  assert_json_contains "$RESULT" '.result' 'error' "fetch errored (request aborted)"

  pt_delete "/tabs/${TAB_ID}/network/route" >/dev/null
  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "fulfill on UNLISTED host: rule fires (fetch does not throw)"

  # Baseline: without the rule, the fetch to a .invalid host throws (DNS
  # fails). Establish that first so the with-rule comparison is meaningful.
  fetch_in_page "https://mock-target.invalid/api"
  assert_json_contains "$RESULT" '.result' 'error' "baseline: bare fetch to .invalid throws"

  # Install fulfill on the unlisted host. Cross-origin response is opaque to
  # the page (we don't set ACAO — that would be the CORS bypass we explicitly
  # guard against), so we can't read the body. We *can* verify the rule
  # intercepted by checking fetch resolved instead of throwing.
  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"mock-target.invalid","action":"fulfill","body":"{\"forged\":true}","status":200,"contentType":"application/json"}'
  assert_ok "install fulfill rule on unlisted host"

  fetch_in_page "https://mock-target.invalid/api"
  # After fulfill: fetch resolves (with an opaque response). The shim's catch
  # branch — the only path that emits an "error" key — does NOT fire.
  assert_json_eq "$RESULT" '(.result | contains("error"))' 'false' "fetch resolved (rule intercepted)"

  pt_delete "/tabs/${TAB_ID}/network/route" >/dev/null
  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "fulfill on ALLOWLISTED host falls through to real fetch"

  # allowedDomains in pinchtab-full-permissive includes the fixtures host. Per
  # the inverted policy, fulfill must NOT win here — the real fixtures content
  # must come back. Same-origin fetch, so the body is fully readable.
  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"buttons.html","action":"fulfill","body":"{\"forged\":true}","status":200,"contentType":"application/json"}'
  assert_ok "install fulfill rule on allowlisted host"

  fetch_in_page "${FIXTURES_URL}/buttons.html"
  # If fulfill had won, the body would be {"forged":true}. We assert the real
  # fixture HTML came through instead.
  assert_json_eq "$RESULT" '(.result | fromjson | .text | contains("forged") | not)' 'true' "fulfill blocked: body is real fixture, not forged"

  pt_delete "/tabs/${TAB_ID}/network/route" >/dev/null
  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "fulfill with --resource-type=script does not match a fetch()"

  # Scope the rule to ResourceType=script. fetch() carries resource type
  # "fetch" or "xhr" — neither matches "script", so the rule must NOT fire,
  # the request must reach the (nonexistent) host, and the page must see a
  # network error.
  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"resource-target.invalid","action":"fulfill","body":"{\"forged\":true}","status":200,"contentType":"application/json","resourceType":"script"}'
  assert_ok "install resource-type-scoped rule"

  fetch_in_page "https://resource-target.invalid/api"
  assert_json_contains "$RESULT" '.result' 'error' "resource-type filter excludes fetch()"

  pt_delete "/tabs/${TAB_ID}/network/route" >/dev/null
  end_test
}

with_server "$E2E_FULL_SERVER" netroute_smoke_tests
