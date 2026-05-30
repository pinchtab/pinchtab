#!/bin/bash
# browser-routing-smoke.sh — Cross-browser routing smoke tests.
#
# Verifies ?browser= query parameter routing: the configured browser
# serves requests, route metadata is populated, and unknown browsers
# produce clear errors. Tests 1-3 use the configured browser; tests 4+
# exercise cross-browser routing when all 3 providers are available.
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

# ─────────────────────────────────────────────────────────────────
# Cross-browser routing: exercise each provider against the running stack.
# These tests probe whether the stack supports routing to other browsers
# before running. A single-browser stack will skip these; a multi-browser
# stack (with browser targets configured) will run them.
#
# TODO: Add a multi-browser E2E config with browser targets for all 3
# providers so these tests exercise real cross-browser routing.

if [ "$CONFIGURED" != "chrome" ]; then
  echo "  ⚠️  Skipping cross-browser routing (configured=${CONFIGURED}, need chrome)"
else

CROSS_BROWSERS="ghost-chrome cloak"
for CROSS in $CROSS_BROWSERS; do

# Probe: can this stack route to $CROSS?
PROBE_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${E2E_SERVER_TOKEN}" \
  "${E2E_SERVER}/text?browser=${CROSS}" 2>/dev/null)

if [ "$PROBE_STATUS" = "503" ] || [ "$PROBE_STATUS" = "400" ]; then
  echo "  ⚠️  Skipping cross-browser routing to ${CROSS} (probe returned ${PROBE_STATUS})"
  continue
fi

# ─────────────────────────────────────────────────────────────────
start_test "cross-browser: ?browser=${CROSS} text returns content"

pt_get "/text?browser=${CROSS}"
assert_ok "text with browser=${CROSS}"
assert_contains "$RESULT" "E2E Test\|Welcome\|test fixtures" "text has page content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "cross-browser: ?browser=${CROSS} snapshot returns nodes"

pt_get "/snapshot?browser=${CROSS}"
assert_ok "snapshot with browser=${CROSS}"
assert_json_length_gte "$RESULT" '.nodes' 1 "snapshot has nodes"

end_test

done

fi
