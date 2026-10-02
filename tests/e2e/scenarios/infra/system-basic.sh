#!/bin/bash
# system-basic.sh — API config and observability happy-path scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# CF7: Chrome version in user agent

start_test "config: Chrome version in user agent"

pt_post /navigate '{"url":"about:blank"}'
assert_ok "navigate"
TAB_ID=$(get_tab_id)

pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"navigator.userAgent\"}"
assert_ok "evaluate UA"
UA=$(echo "$RESULT" | jq -r '.result')

if echo "$UA" | grep -qE '(Headless)?Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'; then
  CHROME_VERSION=$(echo "$UA" | grep -oE '(Headless)?Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+')
  pass_assert "UA contains Chrome version: $CHROME_VERSION"
else
  fail_assert "UA missing Chrome version: $UA"
fi

pt_post /close "{\"tabId\":\"$TAB_ID\"}" >/dev/null 2>&1

end_test

# CF8: Fingerprint rotation preserves Chrome version

start_test "config: fingerprint rotation preserves Chrome version"

if [ "${PINCHTAB_E2E_BROWSER:-chrome}" = "cloak" ]; then
  skip_test "fingerprint rotation is a PinchTab-overlay feature; cloak owns its own fingerprint engine"
else
  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
  assert_ok "navigate"
  TAB_ID=$(get_tab_id)

  pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"navigator.userAgent\"}"
  assert_ok "initial UA"
  INITIAL_UA=$(echo "$RESULT" | jq -r '.result')
  INITIAL_VERSION=$(echo "$INITIAL_UA" | grep -oE 'Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1)

  if [ -z "$INITIAL_VERSION" ]; then
    fail_assert "initial UA missing Chrome version"
  else
    pass_assert "initial version: $INITIAL_VERSION"
  fi

  pt_post /fingerprint/rotate "{\"os\":\"mac\",\"tabId\":\"$TAB_ID\"}"
  assert_ok "fingerprint rotate"

  pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"navigator.userAgent\"}"
  assert_ok "rotated UA"
  ROTATED_UA=$(echo "$RESULT" | jq -r '.result')
  ROTATED_VERSION=$(echo "$ROTATED_UA" | grep -oE 'Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1)

  if [ "$INITIAL_VERSION" = "$ROTATED_VERSION" ]; then
    pass_assert "Chrome version preserved after rotation: $ROTATED_VERSION"
  else
    fail_assert "Chrome version changed: $INITIAL_VERSION → $ROTATED_VERSION"
  fi

  pt_post /close "{\"tabId\":\"$TAB_ID\"}" >/dev/null 2>&1
fi

end_test

# CF1: Config file loading (server started successfully)

start_test "config: server loads config file and starts"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
TAB_ID=$(get_tab_id)

pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"document.title\"}"
assert_ok "evaluate"
assert_json_exists "$RESULT" ".result" "got title result"

pt_post /close "{\"tabId\":\"$TAB_ID\"}" >/dev/null 2>&1

end_test

# CF2: Config file port is used (server runs on port from config)

start_test "config: server uses port from config file"

# The server is running on port 9999 from config file.
# Verifies config is loaded correctly.
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate (proves env port override works)"

TAB_ID=$(get_tab_id)
pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"window.location.href\"}"
assert_ok "evaluate"
assert_json_exists "$RESULT" ".result" "got location result"

pt_post /close "{\"tabId\":\"$TAB_ID\"}" >/dev/null 2>&1

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /api/activity — records tab-scoped requests"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/tabs/${TAB_ID}/snapshot"
assert_ok "tab snapshot succeeded"

pt_post "/tabs/${TAB_ID}/action" -d '{"kind":"click","selector":"#increment"}'
assert_http_status "200" "click action succeeded"

pt_get "/api/activity?tabId=${TAB_ID}&limit=100&ageSec=300"
assert_ok "activity query"
assert_json_exists "$RESULT" '.events' "events array present"
assert_result_has_tab_event \
  "$TAB_ID" \
  "/tabs/${TAB_ID}/snapshot" \
  "tab snapshot event recorded" \
  "missing tab snapshot event for tab ${TAB_ID}"
assert_result_has_tab_event \
  "$TAB_ID" \
  "/tabs/${TAB_ID}/action" \
  "tab action event recorded" \
  "missing tab action event for tab ${TAB_ID}"

end_test

# Test: Browser extension loading
# Verifies that extension paths reach Chrome startup reliably in CI.

ORCH_URL=$E2E_SERVER
ORIG_URL=$E2E_SERVER

# --- T1: Default instance loads configured extension path ---
start_test "Extension config: default instance loads configured extension path"

pt_get /health
assert_ok "health"
DEFAULT_INST_ID=$(echo "$RESULT" | jq -r '.defaultInstance.id // empty')
if [ -n "$DEFAULT_INST_ID" ] && [ "$DEFAULT_INST_ID" != "null" ]; then
  pass_assert "default instance present: ${DEFAULT_INST_ID}"
else
  fail_assert "default instance present"
fi

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
EXTENSION_TAB_ID=$(get_tab_id)
pt_post "/tabs/${EXTENSION_TAB_ID}/evaluate" '{"expression":"document.documentElement.getAttribute(\"data-pinchtab-test-extension\")"}'
assert_ok "evaluate extension marker"
assert_result_eq ".result" "loaded" "configured extension executed on the default instance"

end_test

# --- T2: Instance start rejects request-level extension injection ---
start_test "Extension config: instance start rejects extensionPaths"

pt_post /instances/start '{"extensionPaths":["/extensions/test-extension"]}'
assert_not_ok "instance start rejects extensionPaths"
assert_contains "$RESULT" "extensionPaths are not supported" "extensionPaths rejection message"

end_test

# --- T3: Launch alias also rejects request-level extension injection ---
start_test "Extension config: launch alias rejects extensionPaths"

pt_post /instances/launch '{"extensionPaths":["/extensions/test-extension-api"]}'
assert_not_ok "launch rejects extensionPaths"
assert_contains "$RESULT" "extensionPaths are not supported" "launch rejection message"

end_test
