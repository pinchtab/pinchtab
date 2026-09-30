#!/bin/bash
# instance-navigate-extended.sh — `pinchtab instance navigate` opens the page in
# one tabs/open call: the tab lands on the URL, is listed by the instance, and
# no blank target is left behind (PIN-428).

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

NAVIGATE_INST_ID=""

scenario_cleanup() {
  if [ -n "$NAVIGATE_INST_ID" ]; then
    e2e_curl -s -X POST "${E2E_SERVER}/instances/${NAVIGATE_INST_ID}/stop" >/dev/null 2>&1 || true
  fi
}

# Every page target in the Chrome of the instance owning tab $1, blank ones
# included: /tabs hides transient URLs, /screencast/tabs does not, and the tabId
# query routes the request to the tab's owner.
navigate_instance_targets() {
  e2e_curl -s "${E2E_SERVER}/screencast/tabs?tabId=$1"
}

navigate_blank_count() {
  jq '[.[] | select(.url == "about:blank")] | length' <<< "$1"
}

# The page URL as Chrome reports it: the default :80 port is dropped.
navigate_landed_url() {
  printf '%s' "${1/:80\//\/}"
}

# Opens a baseline tab on instance $1 through the API, runs
# `instance navigate $1 <fixture>`, and asserts the verb printed a tab that
# landed on the page, the instance lists it, and it added exactly one Chrome
# target and no about:blank one.
check_instance_navigate() {
  local inst="$1"
  local url="${FIXTURES_URL}/buttons.html"
  local landed
  landed=$(navigate_landed_url "$url")

  local baseline_tab
  local baseline_resp
  baseline_resp=$(e2e_curl -s -X POST "${E2E_SERVER}/instances/${inst}/tabs/open" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"${FIXTURES_URL}/index.html\"}")
  baseline_tab=$(get_tab_id "$baseline_resp")
  if [ -z "$baseline_tab" ]; then
    fail_assert "open a baseline tab on ${inst}"
    return 0
  fi
  local before
  before=$(navigate_instance_targets "$baseline_tab")
  if ! jq -e 'type == "array"' <<< "$before" >/dev/null 2>&1; then
    fail_assert "read the instance's page targets (got $before)"
    return 0
  fi

  pt_ok instance navigate "$inst" "$url"
  assert_output_json "instance navigate prints JSON"
  local tab
  tab=$(get_tab_id "$PT_OUT")
  if [ -n "$tab" ]; then
    pass_assert "output names the tab: ${tab:0:12}..."
  else
    fail_assert "output names the tab (got $PT_OUT)"
    return 0
  fi
  assert_json_field '.url' "$landed" "output reports the landed URL"
  assert_json_field '.title' "E2E Test - Buttons" "output reports the landed page title"
  assert_output_not_contains "about:blank" "output does not report a blank tab"

  local listed
  listed=$(e2e_curl -s "${E2E_SERVER}/instances/${inst}/tabs")
  if jq -e --arg id "$tab" --arg url "$landed" \
      'map(select(.id == $id and .url == $url)) | length == 1' <<< "$listed" >/dev/null 2>&1; then
    pass_assert "GET /instances/${inst}/tabs lists the tab on the URL"
  else
    fail_assert "GET /instances/${inst}/tabs lists the tab on the URL (got $listed)"
  fi

  local after before_total after_total before_blank after_blank
  after=$(navigate_instance_targets "$tab")
  before_total=$(jq 'length' <<< "$before")
  after_total=$(jq 'length' <<< "$after")
  before_blank=$(navigate_blank_count "$before")
  after_blank=$(navigate_blank_count "$after")
  if [ "$after_total" -eq $((before_total + 1)) ]; then
    pass_assert "instance navigate adds exactly one Chrome target ($before_total -> $after_total)"
  else
    fail_assert "instance navigate adds exactly one Chrome target ($before_total -> $after_total): $after"
  fi
  if [ "$after_blank" -eq "$before_blank" ]; then
    pass_assert "instance navigate leaves no about:blank target behind ($before_blank -> $after_blank)"
  else
    fail_assert "instance navigate leaves no about:blank target behind ($before_blank -> $after_blank): $after"
  fi
  if jq -e --arg id "$tab" --arg url "$landed" \
      'map(select(.id == $id and .url == $url)) | length == 1' <<< "$after" >/dev/null 2>&1; then
    pass_assert "the added Chrome target is the navigated tab on its URL"
  else
    fail_assert "the added Chrome target is the navigated tab on its URL (got $after)"
  fi

  e2e_curl -s -X POST "${E2E_SERVER}/tabs/${baseline_tab}/close" >/dev/null 2>&1 || true
  e2e_curl -s -X POST "${E2E_SERVER}/tabs/${tab}/close" >/dev/null 2>&1 || true
}

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab instance navigate: default instance"

# The default instance is the one the front-door shorthand routes to.
pt_ok nav "${FIXTURES_URL}/index.html"
DEFAULT_TAB=$(tr -d '[:space:]' <<< "$PT_OUT")
DEFAULT_INST_ID=$(e2e_curl -s "${E2E_SERVER}/instances/tabs?fresh=1" \
  | jq -r --arg id "$DEFAULT_TAB" 'map(select(.id == $id)) | .[0].instanceId // empty')
if [ -z "$DEFAULT_INST_ID" ]; then
  fail_assert "find the default instance owning tab ${DEFAULT_TAB}"
else
  pass_assert "default instance: ${DEFAULT_INST_ID:0:12}..."
  check_instance_navigate "$DEFAULT_INST_ID"
fi
e2e_curl -s -X POST "${E2E_SERVER}/tabs/${DEFAULT_TAB}/close" >/dev/null 2>&1 || true

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab instance navigate: non-default instance"

pt_ok instance start
NAVIGATE_INST_ID=$(jq -r '.id // empty' <<< "$PT_OUT")
if [ -z "$NAVIGATE_INST_ID" ]; then
  fail_assert "instance start returns an id (got $PT_OUT)"
elif [ "$NAVIGATE_INST_ID" = "$DEFAULT_INST_ID" ]; then
  fail_assert "instance start returns a second instance (got the default ${DEFAULT_INST_ID})"
else
  RUNNING=false
  for ATTEMPT in $(seq 1 60); do
    STATUS=$(e2e_curl -s "${E2E_SERVER}/instances/${NAVIGATE_INST_ID}" | jq -r '.status // empty')
    if [ "$STATUS" = "running" ]; then
      RUNNING=true
      break
    fi
    sleep 1
  done
  if [ "$RUNNING" = "true" ]; then
    pass_assert "second instance ${NAVIGATE_INST_ID:0:12}... is running"
    check_instance_navigate "$NAVIGATE_INST_ID"
  else
    fail_assert "instance ${NAVIGATE_INST_ID} reached running within 60s (status: $STATUS)"
  fi
fi

end_test
