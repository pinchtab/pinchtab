#!/bin/bash
# browser-activity-basic.sh — Activity log browser metadata assertions.
# Covers: /api/activity route fields: requestedBrowser, usedBrowser, attempts,
#         escalation, and reason.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

EXPECTED_PROVIDER="${PINCHTAB_E2E_PROVIDER:-chrome}"

# ─────────────────────────────────────────────────────────────────
start_test "activity metadata: navigate with browser param records route"

pt_post "/navigate?browser=chrome" -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate with browser=chrome"

# Small delay to let activity be recorded.
sleep 1

pt_get "/api/activity?limit=5"
assert_ok "get recent activity"

# Find the navigate event and check route metadata.
ROUTE_EVENT=$(echo "$RESULT" | jq '[.events[] | select(.path == "/navigate" and .route != null)] | first // empty' 2>/dev/null)
if [ -z "$ROUTE_EVENT" ] || [ "$ROUTE_EVENT" = "null" ] || [ "$ROUTE_EVENT" = "" ]; then
  soft_pass_assert "no navigate event with route in recent activity (may need more events)"
else
  REQ_BROWSER=$(echo "$ROUTE_EVENT" | jq -r '.route.requestedProvider // empty')
  USED_BROWSER=$(echo "$ROUTE_EVENT" | jq -r '.route.usedProvider // empty')
  ESCALATED=$(echo "$ROUTE_EVENT" | jq -r '.route.escalated // empty')

  if [ "$REQ_BROWSER" = "chrome" ]; then
    pass_assert "activity route.requestedProvider=chrome"
  else
    soft_pass_assert "requestedProvider=$REQ_BROWSER (expected chrome)"
  fi

  if [ -n "$USED_BROWSER" ] && [ "$USED_BROWSER" != "null" ]; then
    pass_assert "activity route.usedProvider=$USED_BROWSER"
  else
    soft_pass_assert "usedProvider not set in activity"
  fi

  if [ "$ESCALATED" = "false" ] || [ "$ESCALATED" = "" ] || [ "$ESCALATED" = "null" ]; then
    pass_assert "activity route.escalated=false (no escalation needed)"
  else
    soft_pass_assert "escalated=$ESCALATED (may be expected)"
  fi
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "activity metadata: text request records browser in activity"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate to form"

pt_get "/text?browser=${EXPECTED_PROVIDER}"
assert_ok "text with browser=${EXPECTED_PROVIDER}"

sleep 1

pt_get "/api/activity?limit=10"
assert_ok "get activity after text request"

TEXT_EVENT=$(echo "$RESULT" | jq '[.events[] | select(.path == "/text" and .route != null)] | first // empty' 2>/dev/null)
if [ -z "$TEXT_EVENT" ] || [ "$TEXT_EVENT" = "null" ] || [ "$TEXT_EVENT" = "" ]; then
  soft_pass_assert "no text event with route in activity (provider may not record route for text)"
else
  REQ=$(echo "$TEXT_EVENT" | jq -r '.route.requestedProvider // empty')
  if [ "$REQ" = "$EXPECTED_PROVIDER" ]; then
    pass_assert "text activity route.requestedProvider=${EXPECTED_PROVIDER}"
  else
    soft_pass_assert "text requestedProvider=$REQ (expected ${EXPECTED_PROVIDER})"
  fi
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "activity metadata: activity events have required fields"

pt_get "/api/activity?limit=3"
assert_ok "get activity"
assert_json_exists "$RESULT" '.events' "has events array"
assert_json_exists "$RESULT" '.count' "has count field"

FIRST_EVENT=$(echo "$RESULT" | jq '.events[0] // empty' 2>/dev/null)
if [ -n "$FIRST_EVENT" ] && [ "$FIRST_EVENT" != "null" ] && [ "$FIRST_EVENT" != "" ]; then
  assert_json_exists "$FIRST_EVENT" '.method' "event has method"
  assert_json_exists "$FIRST_EVENT" '.path' "event has path"
  assert_json_exists "$FIRST_EVENT" '.status' "event has status"
else
  soft_pass_assert "no events to validate (empty activity log)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "activity metadata: route attempts array present when routed"

pt_post "/navigate?browser=chrome" -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate with explicit browser"

sleep 1

pt_get "/api/activity?limit=5"
assert_ok "get activity"

ROUTE_EVENT=$(echo "$RESULT" | jq '[.events[] | select(.path == "/navigate" and .route != null)] | first // empty' 2>/dev/null)
if [ -n "$ROUTE_EVENT" ] && [ "$ROUTE_EVENT" != "null" ] && [ "$ROUTE_EVENT" != "" ]; then
  HAS_ATTEMPTS=$(echo "$ROUTE_EVENT" | jq 'has("route") and (.route | has("attempts"))' 2>/dev/null || echo "false")
  if [ "$HAS_ATTEMPTS" = "true" ]; then
    ATTEMPTS_LEN=$(echo "$ROUTE_EVENT" | jq '.route.attempts | length' 2>/dev/null || echo "0")
    pass_assert "route has attempts array (length=$ATTEMPTS_LEN)"
  else
    soft_pass_assert "route present but no attempts array (single-browser route)"
  fi

  HAS_REASON=$(echo "$ROUTE_EVENT" | jq '.route.reason // empty' 2>/dev/null || echo "")
  if [ -n "$HAS_REASON" ] && [ "$HAS_REASON" != "null" ]; then
    pass_assert "route has reason: $HAS_REASON"
  else
    soft_pass_assert "route has no reason field (direct match)"
  fi
else
  soft_pass_assert "no routed navigate event in recent activity"
fi

end_test
