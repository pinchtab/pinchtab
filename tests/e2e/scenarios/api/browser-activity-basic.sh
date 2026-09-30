#!/bin/bash
# browser-activity-basic.sh — what the activity log records about a routed
# request through the orchestrator front door: the response carries the route
# metadata and the event carries the request. The bridge topology, where that
# metadata reaches the event itself, needs a second server and is asserted in
# browser-activity-extended.sh so the PR tier keeps its one-server stack.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

EXPECTED_BROWSER="${PINCHTAB_E2E_BROWSER:-chrome}"
ACTIVITY_TIMEOUT="${E2E_ACTIVITY_TIMEOUT:-10}"

# assert_route_metadata holds route metadata to the shape routeMetadataFor
# builds for every provider: a provider served the request, the attempts trail
# ends on an accepted one, and escalation shows as an extra attempt rather than
# as a silently different answer. Measured across chrome and ghost-chrome:
# usedProvider is NOT always the trail's last provider (a static-first navigate
# that escalates keeps the requested provider in usedProvider), so the trail is
# checked on acceptance rather than on identity.
assert_route_metadata() {
  local route="$1" label="$2"
  local used escalated attempts_len last_accepted

  used=$(echo "$route" | jq -r '.usedProvider // empty')
  escalated=$(echo "$route" | jq -r 'if has("escalated") then .escalated else "missing" end')
  attempts_len=$(echo "$route" | jq '.attempts | length' 2>/dev/null || echo "0")
  last_accepted=$(echo "$route" | jq -r '.attempts[-1].accepted' 2>/dev/null)

  if [ -n "$used" ]; then
    pass_assert "$label route.usedProvider=$used"
  else
    fail_assert "$label route.usedProvider is empty — no provider recorded as serving the request: $route"
  fi

  if [ "$attempts_len" -ge 1 ] && [ "$last_accepted" = "true" ]; then
    pass_assert "$label route.attempts ends on an accepted provider ($attempts_len attempt(s))"
  else
    fail_assert "$label route.attempts=$attempts_len with last accepted='$last_accepted', want a trail ending on an accepted provider: $route"
  fi

  case "$escalated" in
    false)
      if [ "$attempts_len" -eq 1 ]; then
        pass_assert "$label not escalated: one attempt"
      else
        fail_assert "$label escalated=false but records $attempts_len attempts: $route"
      fi
      ;;
    true)
      if [ "$attempts_len" -ge 2 ]; then
        pass_assert "$label escalated after $attempts_len attempts"
      else
        fail_assert "$label escalated=true but records only $attempts_len attempt: $route"
      fi
      ;;
    *)
      fail_assert "$label route.escalated is '$escalated', want true or false"
      ;;
  esac
}

# assert_requested_provider is asserted on RESPONSES only. The route recorded on
# an activity event can be a later phase of the same request (measured: a
# ghost-chrome navigate logs the post-escalation route), so pinning the caller's
# browser there would fail for a reason that is not about the activity log.
assert_requested_provider() {
  local route="$1" label="$2"
  local requested
  requested=$(echo "$route" | jq -r '.requestedProvider // empty')
  if [ "$requested" = "$EXPECTED_BROWSER" ]; then
    pass_assert "$label route.requestedProvider=$EXPECTED_BROWSER"
  else
    fail_assert "$label route.requestedProvider=$requested, want $EXPECTED_BROWSER"
  fi
}

# newest_activity_event prints the newest activity event for a path, or nothing
# when the log has not caught up yet. Events come back oldest→newest.
newest_activity_event() {
  local path="$1"
  # pt_get echoes the response body on stdout, and this function's own stdout is
  # what the caller captures — so the fetch is silenced and only the selected
  # event is printed.
  pt_get "/api/activity?limit=25" >/dev/null 2>&1
  echo "$RESULT" | jq -c --arg path "$path" '[.events[] | select(.path == $path)] | last // empty'
}

# activity_event_landed succeeds once an event for this path is newer than the
# timestamp captured before the request, so the wait tracks THIS request rather
# than a leftover from an earlier scenario. It publishes the event in
# ACTIVITY_EVENT for the assertions that follow.
activity_event_landed() {
  local path="$1" before="$2"
  ACTIVITY_EVENT=$(newest_activity_event "$path")
  [ -n "$ACTIVITY_EVENT" ] || return 1
  [ "$(echo "$ACTIVITY_EVENT" | jq -r '.timestamp')" != "$before" ]
}

# ─────────────────────────────────────────────────────────────────
start_test "activity metadata: navigate with browser param records route"

BEFORE_TS=$(newest_activity_event /navigate | jq -r '.timestamp // empty')

pt_post "/navigate?browser=${EXPECTED_BROWSER}" -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate with browser=${EXPECTED_BROWSER}"
NAV_ROUTE=$(echo "$RESULT" | jq -c '.route')
assert_requested_provider "$NAV_ROUTE" "navigate response"
assert_route_metadata "$NAV_ROUTE" "navigate response"
NAV_TAB=$(get_tab_id)

if wait_until "activity_event_landed /navigate '$BEFORE_TS'" "$ACTIVITY_TIMEOUT"; then
  assert_json_eq "$ACTIVITY_EVENT" '.method' "POST" "activity event method=POST"
  assert_json_eq "$ACTIVITY_EVENT" '.action' "navigate" "activity event action=navigate"
  assert_json_eq "$ACTIVITY_EVENT" '.status' "200" "activity event status=200"
  assert_json_eq "$ACTIVITY_EVENT" '.tabId' "$NAV_TAB" "activity event carries the navigated tab"
  assert_json_contains "$ACTIVITY_EVENT" '.url' "buttons.html" "activity event carries the navigated URL"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "activity metadata: text request records browser in activity"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate to form"
TEXT_TAB=$(get_tab_id)

BEFORE_TS=$(newest_activity_event /text | jq -r '.timestamp // empty')

pt_get "/text?browser=${EXPECTED_BROWSER}&tabId=${TEXT_TAB}"
assert_ok "text with browser=${EXPECTED_BROWSER}"
TEXT_ROUTE=$(echo "$RESULT" | jq -c '.route')
assert_requested_provider "$TEXT_ROUTE" "text response"
assert_route_metadata "$TEXT_ROUTE" "text response"

if wait_until "activity_event_landed /text '$BEFORE_TS'" "$ACTIVITY_TIMEOUT"; then
  assert_json_eq "$ACTIVITY_EVENT" '.action' "text" "activity event action=text"
  assert_json_eq "$ACTIVITY_EVENT" '.status' "200" "activity event status=200"
  assert_json_eq "$ACTIVITY_EVENT" '.tabId' "$TEXT_TAB" "activity event carries the read tab"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "activity metadata: activity events have required fields"

pt_get "/api/activity?limit=25"
assert_ok "get activity"
assert_json_exists "$RESULT" '.count' "has count field"
assert_json_jq "$RESULT" '.events | length >= 1' \
  "activity log is not empty after the requests above" \
  "activity log is empty after two requests that must have been recorded"
assert_json_jq "$RESULT" '.events | all(has("method") and has("path") and has("status"))' \
  "every event has method, path and status" \
  "an activity event is missing method, path or status"

end_test
