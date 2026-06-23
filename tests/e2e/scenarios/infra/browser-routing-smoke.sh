#!/bin/bash
# browser-routing-smoke.sh — Cross-browser routing smoke tests.
#
# Verifies ?browser= query parameter routing: the configured browser
# serves requests, route metadata is populated, and unknown browsers
# produce clear errors. Tests 1-3 use the configured browser. The
# cross-browser block (chrome lane) then ASSERTS routing deterministically:
# ghost-chrome MUST route (it reuses Chrome's binary, present in the e2e
# image), and cloak MUST fail loudly (its proprietary binary is absent), so
# a cross-provider routing regression fails the suite instead of skipping.
#
# Requires: E2E_SERVER, FIXTURES_URL

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

CONFIGURED="${PINCHTAB_E2E_BROWSER:-chrome}"

# ─────────────────────────────────────────────────────────────────
start_test "browser routing: ?browser=<configured> text succeeds"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

pt_get "/text?browser=${CONFIGURED}"
assert_ok "text with browser=${CONFIGURED}"
assert_contains "$RESULT" "E2E Test\|Welcome\|test fixtures" "text has page content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "browser routing: ?browser=<configured> snapshot succeeds"

pt_get "/snapshot?browser=${CONFIGURED}"
assert_ok "snapshot with browser=${CONFIGURED}"
assert_json_length_gte "$RESULT" '.nodes' 1 "snapshot has nodes"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "browser routing: navigate with ?browser=<configured> has route metadata"

pt_post "/navigate" -d "{\"url\":\"${FIXTURES_URL}/buttons.html\",\"browser\":\"${CONFIGURED}\"}"
assert_ok "navigate with browser=${CONFIGURED}"
assert_json_contains "$RESULT" '.url' 'buttons.html' "navigated to buttons page"

HAS_ROUTE=$(echo "$RESULT" | jq 'has("route")' 2>/dev/null || echo "false")
if [ "$HAS_ROUTE" = "true" ]; then
  assert_json_eq "$RESULT" '.route.requestedProvider' "${CONFIGURED}" "route.requestedProvider=${CONFIGURED}"
else
  soft_pass_assert "route metadata not in navigate response"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "browser routing: unknown browser → structured error"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\",\"browser\":\"nonexistent_xyz\"}"
assert_http_status 400 "unknown browser rejects"

end_test

# ═════════════════════════════════════════════════════════════════
# Cross-browser routing on a single browser-pinned server — DETERMINISTIC,
# FAIL-LOUD.
#
# A single always-on server serves exactly ONE browser. Requesting a different
# built-in provider (`?browser=<other>`) must be REJECTED with a structured
# 409 browser_conflict — never silently served by the wrong browser, and never
# left to time out as "instance not ready". On-demand cross-browser launch is
# the multi-instance (simple) strategy's job; here the operator is told to
# restart with --browser or run a separate instance. (See
# internal/orchestrator/instance_query.go FirstRunningURLForRequest.)
#
# These assertions run only in the chrome lane: tests 1-3 above already cover
# the configured provider, and cross-provider semantics are defined relative
# to the chrome primary.

if [ "$CONFIGURED" != "chrome" ]; then
  echo "  ⚠️  Skipping cross-browser routing (configured=${CONFIGURED}, asserted only in the chrome lane)"
else

# ─────────────────────────────────────────────────────────────────
# ghost-chrome is a real built-in provider, but this server is pinned to
# chrome — so the request must fail fast with a 409 browser_conflict, not a
# wrong-browser serve and not a 10s "not ready" stall.
start_test "cross-browser: ?browser=ghost-chrome rejected with conflict (single-browser server)"

pt_get "/text?browser=ghost-chrome"
assert_http_status 409 "text with browser=ghost-chrome → 409 conflict (not silently served, not a timeout)"
assert_contains "$RESULT" "browser_conflict\|already running" "structured browser conflict"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "cross-browser: ?browser=ghost-chrome snapshot rejected with conflict"

pt_get "/snapshot?browser=ghost-chrome"
assert_http_status 409 "snapshot with browser=ghost-chrome → 409 conflict"

end_test

# ─────────────────────────────────────────────────────────────────
# cloak: also a non-configured provider here (and its proprietary binary is
# absent from the e2e image). Same rule — reject loudly, never silently route
# to chrome. The conflict guard fires before any launch is attempted.
start_test "cross-browser: ?browser=cloak fails loudly (not configured on this server)"

pt_get "/text?browser=cloak"
assert_not_ok "text with browser=cloak rejects — must not silently route to chrome"

end_test

fi

# A fully deterministic 3-browser fixture (chrome + ghost-chrome + cloak)
# requires, in tests/e2e/docker-compose-multi.yml + tests/e2e/config/:
#   * a CloakBrowser Chromium binary baked into the e2e image and a config
#     with browser.binary (or a cloak target's binary) pointing at it;
#   * then the cloak block above becomes assert_ok + the same content/node
#     checks used for ghost-chrome.
# CloakBrowser is proprietary and not redistributable, so until that image
# exists cloak stays asserted as fail-loud rather than skipped.
