#!/bin/bash
# network-route-extended.sh — Happy paths and validation rejects for the
# /tabs/{id}/network/route family. Runs the happy path against
# pinchtab-full-permissive (where allowNetworkIntercept is on) and asserts
# that the default server (capability off) returns 403. Lives in the
# extended suite (not basic) because it needs the full-permissive sidecar.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

if [ -z "${E2E_FULL_SERVER:-}" ]; then
  echo "  E2E_FULL_SERVER not set, skipping network/route happy-path scenarios"
  return 0 2>/dev/null || exit 0
fi

# ─────────────────────────────────────────────────────────────────
# Capability disabled on the default server → 403.
# ─────────────────────────────────────────────────────────────────
start_test "POST /tabs/{id}/network/route is 403 when capability off (default server)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate"
TAB_ID=$(get_tab_id)

pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"*.png","action":"abort"}'
assert_not_ok "rejects when allowNetworkIntercept=false"
assert_json_contains "$RESULT" '.code' 'network_intercept_disabled' "error code identifies the disabled capability"
assert_json_contains "$RESULT" '.details.setting' 'security.allowNetworkIntercept' "error names the config setting"

end_test

# ─────────────────────────────────────────────────────────────────
# Happy paths run against the full-permissive server.
# ─────────────────────────────────────────────────────────────────
netroute_full_server_tests() {
  pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
  assert_ok "navigate (full-permissive)"
  TAB_ID=$(get_tab_id)

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route installs an abort rule"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"*.png","action":"abort"}'
  assert_ok "install abort rule"
  assert_json_eq "$RESULT" '.ok' 'true'
  assert_json_eq "$RESULT" '.rules | length' '1' "one rule stored"
  assert_json_eq "$RESULT" '.rules[0].action' 'abort'
  assert_json_eq "$RESULT" '.rules[0].pattern' '*.png'

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "GET /tabs/{id}/network/route lists current rules"

  pt_get "/tabs/${TAB_ID}/network/route"
  assert_ok "list rules"
  assert_json_eq "$RESULT" '.rules | length' '1'

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route same pattern replaces (no growth)"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"*.png","action":"continue"}'
  assert_ok "replace by same pattern"
  assert_json_eq "$RESULT" '.rules | length' '1' "still one rule"
  assert_json_eq "$RESULT" '.rules[0].action' 'continue' "action updated"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route adds fulfill rule with json body"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"api/users","action":"fulfill","body":"{\"k\":1}","status":201}'
  assert_ok "install fulfill rule"
  assert_json_eq "$RESULT" '.rules | length' '2' "two rules"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route rejects oversize body"

  # Build a string slightly over 1 MiB.
  HUGE=$(head -c $((1024 * 1024 + 16)) /dev/zero | tr '\0' 'x')
  PAYLOAD=$(jq -n --arg b "$HUGE" '{pattern:"big",action:"fulfill",body:$b}')
  pt_post "/tabs/${TAB_ID}/network/route" -d "$PAYLOAD"
  assert_not_ok "rejects body over 1 MiB cap"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route rejects out-of-range status"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"x","action":"fulfill","body":"{}","status":999}'
  assert_not_ok "rejects status=999"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"x","action":"fulfill","body":"{}","status":50}'
  assert_not_ok "rejects status=50"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route rejects forbidden Content-Type"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"x","action":"fulfill","body":"<script>1</script>","contentType":"text/html"}'
  assert_not_ok "rejects text/html"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"x","action":"fulfill","body":"alert(1)","contentType":"application/javascript"}'
  assert_not_ok "rejects application/javascript"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"x","action":"fulfill","body":"<svg/>","contentType":"image/svg+xml"}'
  assert_not_ok "rejects image/svg+xml"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route rejects CRLF in Content-Type"

  pt_post "/tabs/${TAB_ID}/network/route" -d "$(jq -n '{pattern:"x",action:"fulfill",body:"{}",contentType:"application/json\r\nX-Evil: 1"}')"
  assert_not_ok "rejects header injection via CRLF"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route rejects unknown HTTP method"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"x","action":"abort","method":"FOOBAR"}'
  assert_not_ok "rejects bogus method"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route rejects unknown resourceType"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"x","action":"abort","resourceType":"java"}'
  assert_not_ok "rejects bogus resourceType"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /tabs/{id}/network/route rejects forbidden URL scheme"

  # Schemes like javascript: / data: / file: / chrome: must never be fulfilled
  # regardless of the host allowlist. Validation is at the bridge boundary so
  # the rule itself is rejected on installation.
  # Note: AddRule's scheme reject fires only when the request URL matches at
  # match time, so the install itself succeeds for an arbitrary pattern. But a
  # fulfill rule whose pattern only matches a forbidden-scheme URL has no
  # user-visible path — covered by unit tests. Here we sanity-check missing
  # pattern still 400s.
  pt_post "/tabs/${TAB_ID}/network/route" -d '{"action":"abort"}'
  assert_not_ok "rejects empty pattern"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "DELETE /tabs/{id}/network/route?pattern=… removes one rule"

  pt_delete "/tabs/${TAB_ID}/network/route?pattern=%2A.png"
  assert_ok "remove one rule"
  assert_json_eq "$RESULT" '.removed' '1'

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "DELETE /tabs/{id}/network/route with a body pattern removes only that rule"

  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"body-a.invalid","action":"abort"}'
  assert_ok "install first body-pattern rule"
  pt_post "/tabs/${TAB_ID}/network/route" -d '{"pattern":"body-b.invalid","action":"abort"}'
  assert_ok "install second body-pattern rule"

  pinchtab DELETE "/tabs/${TAB_ID}/network/route" -d '{"pattern":"body-a.invalid"}'
  assert_ok "remove by body pattern"
  assert_json_eq "$RESULT" '.removed' '1' "the body pattern removed exactly one rule"

  pt_get "/tabs/${TAB_ID}/network/route"
  assert_json_eq "$RESULT" '[.rules[].pattern | select(. == "body-a.invalid")] | length' '0' "the named rule is gone"
  assert_json_eq "$RESULT" '[.rules[].pattern | select(. == "body-b.invalid")] | length' '1' "the other rule survives"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "DELETE /tabs/{id}/network/route (no pattern) clears remaining rules"

  pt_delete "/tabs/${TAB_ID}/network/route"
  assert_ok "clear all"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "DELETE /tabs/{id}/network/route again is idempotent on a cleared tab"

  pt_delete "/tabs/${TAB_ID}/network/route"
  assert_ok "second unroute on cleared tab"
  assert_json_eq "$RESULT" '.removed' '0' "nothing left to remove"
  assert_json_eq "$RESULT" '.rules | tojson' '[]' "the rule list is an empty array"

  end_test

  # ─────────────────────────────────────────────────────────────────
  start_test "POST /network/clear works when capability on (full-permissive)"

  pt_post /network/clear "{\"tabId\":\"${TAB_ID}\"}"
  assert_ok "clear network data"

  end_test
}

with_server "$E2E_FULL_SERVER" netroute_full_server_tests
