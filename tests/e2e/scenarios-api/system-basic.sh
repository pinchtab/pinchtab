#!/bin/bash
# system-basic.sh — API config and observability happy-path scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

# CF7: Chrome version in user agent

start_test "config: Chrome version in user agent"

pt_post /navigate '{"url":"about:blank"}'
assert_ok "navigate"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"navigator.userAgent\"}"
assert_ok "evaluate UA"
UA=$(echo "$RESULT" | jq -r '.result')

if echo "$UA" | grep -qE '(Headless)?Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'; then
  CHROME_VERSION=$(echo "$UA" | grep -oE '(Headless)?Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+')
  echo -e "  ${GREEN}✓${NC} UA contains Chrome version: $CHROME_VERSION"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} UA missing Chrome version: $UA"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post /tab "{\"tabId\":\"$TAB_ID\",\"action\":\"close\"}" >/dev/null 2>&1

end_test

# CF8: Fingerprint rotation preserves Chrome version

start_test "config: fingerprint rotation preserves Chrome version"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"navigator.userAgent\"}"
assert_ok "initial UA"
INITIAL_UA=$(echo "$RESULT" | jq -r '.result')
INITIAL_VERSION=$(echo "$INITIAL_UA" | grep -oE 'Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1)

if [ -z "$INITIAL_VERSION" ]; then
  echo -e "  ${RED}✗${NC} initial UA missing Chrome version"
  ((ASSERTIONS_FAILED++)) || true
else
  echo -e "  ${GREEN}✓${NC} initial version: $INITIAL_VERSION"
  ((ASSERTIONS_PASSED++)) || true
fi

pt_post /fingerprint/rotate "{\"os\":\"mac\",\"tabId\":\"$TAB_ID\"}"
assert_ok "fingerprint rotate"

pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"navigator.userAgent\"}"
assert_ok "rotated UA"
ROTATED_UA=$(echo "$RESULT" | jq -r '.result')
ROTATED_VERSION=$(echo "$ROTATED_UA" | grep -oE 'Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1)

if [ "$INITIAL_VERSION" = "$ROTATED_VERSION" ]; then
  echo -e "  ${GREEN}✓${NC} Chrome version preserved after rotation: $ROTATED_VERSION"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} Chrome version changed: $INITIAL_VERSION → $ROTATED_VERSION"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post /tab "{\"tabId\":\"$TAB_ID\",\"action\":\"close\"}" >/dev/null 2>&1

end_test

# CF1: Config file loading (server started successfully)

start_test "config: server loads config file and starts"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"document.title\"}"
assert_ok "evaluate"
assert_json_exists "$RESULT" ".result" "got title result"

pt_post /tab "{\"tabId\":\"$TAB_ID\",\"action\":\"close\"}" >/dev/null 2>&1

end_test

# CF2: Config file port is used (server runs on port from config)

start_test "config: server uses port from config file"

# The server is running on port 9999 from config file.
# Verifies config is loaded correctly.
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate (proves env port override works)"

TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"window.location.href\"}"
assert_ok "evaluate"
assert_json_exists "$RESULT" ".result" "got location result"

pt_post /tab "{\"tabId\":\"$TAB_ID\",\"action\":\"close\"}" >/dev/null 2>&1

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

assert_instance_logs_poll() {
  local inst_id="$1"
  local needle="$2"
  local desc="$3"
  local attempts="${4:-15}"
  local delay="${5:-1}"

  local i
  for i in $(seq 1 "$attempts"); do
    E2E_SERVER=$ORCH_URL pt_get "/instances/${inst_id}/logs" >/dev/null
    # Avoid a false negative under `set -o pipefail`: `grep -q` exits early
    # after a match, which can SIGPIPE the writer side of a pipeline.
    if [[ "$HTTP_STATUS" =~ ^2 ]] && grep -Fq -- "$needle" <<<"$RESULT"; then
      echo -e "  ${GREEN}✓${NC} $desc"
      ((ASSERTIONS_PASSED++)) || true
      return 0
    fi
    sleep "$delay"
  done

  echo -e "  ${RED}✗${NC} $desc (missing: $needle)"
  ((ASSERTIONS_FAILED++)) || true
  return 1
}

assert_instance_logs_poll_all() {
  local inst_id="$1"
  local desc="$2"
  shift 2

  local attempts=15
  local delay=1
  local i needle ok
  for i in $(seq 1 "$attempts"); do
    E2E_SERVER=$ORCH_URL pt_get "/instances/${inst_id}/logs" >/dev/null
    if [[ "$HTTP_STATUS" =~ ^2 ]]; then
      ok=1
      for needle in "$@"; do
        if ! grep -Fq -- "$needle" <<<"$RESULT"; then
          ok=0
          break
        fi
      done
      if [ "$ok" -eq 1 ]; then
        echo -e "  ${GREEN}✓${NC} $desc"
        ((ASSERTIONS_PASSED++)) || true
        return 0
      fi
    fi
    sleep "$delay"
  done

  echo -e "  ${RED}✗${NC} $desc (missing fragments: $*)"
  ((ASSERTIONS_FAILED++)) || true
  return 1
}

print_extension_hints() {
  local inst_id="${1:-}"
  echo ""
  echo "  ${YELLOW}${BOLD}🔍 Troubleshooting Extension Failure:${NC}"
  echo "  - Check if /extensions/test-extension exists and is readable in the pinchtab container."
  echo "  - Check Manifest V3 host_permissions matches: [\"*://*/*\"]"
  if [ -n "$inst_id" ]; then
    E2E_SERVER=$ORCH_URL pt_get "/instances/${inst_id}/logs" >/dev/null
    echo "  - Recent instance log tail:"
    printf '%s\n' "$RESULT" | tail -n 12 | sed 's/^/    /'
  fi
  echo ""
}

# --- T1: Default instance loads configured extension path ---
start_test "Extension config: default instance loads configured extension path"

pt_get /health
assert_ok "health"
DEFAULT_INST_ID=$(echo "$RESULT" | jq -r '.defaultInstance.id // empty')
if [ -n "$DEFAULT_INST_ID" ] && [ "$DEFAULT_INST_ID" != "null" ]; then
  echo -e "  ${GREEN}✓${NC} default instance present: ${DEFAULT_INST_ID}"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} default instance present"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

DEFAULT_LOG_PASS=1
if [ -n "$DEFAULT_INST_ID" ] && [ "$DEFAULT_INST_ID" != "null" ]; then
  assert_instance_logs_poll_all \
    "$DEFAULT_INST_ID" \
    "default instance logs configured extension path" \
    "loading extensions" \
    "paths=/extensions/test-extension"
  DEFAULT_LOG_PASS=$?

  assert_instance_logs_poll \
    "$DEFAULT_INST_ID" \
    "chrome initialized successfully" \
    "default instance chrome initialized"
fi

if [ $DEFAULT_LOG_PASS -ne 0 ]; then
  print_extension_hints "$DEFAULT_INST_ID"
  # Debug: dump full instance logs for analysis
  echo "  ${YELLOW}📋 Full instance logs:${NC}"
  E2E_SERVER=$ORCH_URL pt_get "/instances/${DEFAULT_INST_ID}/logs" >/dev/null
  printf '%s\n' "$RESULT" | head -30 | sed 's/^/    /'
fi

end_test

# --- T2: Instance start with extension via API ---
start_test "Extension config: instance start accepts extensionPaths"

pt_post /instances/start '{"extensionPaths":["/extensions/test-extension"]}'
assert_ok "instance start with extension"
INST_ID=$(echo "$RESULT" | jq -r '.id')
INST_PORT=$(echo "$RESULT" | jq -r '.port')
E2E_SERVER="http://pinchtab:${INST_PORT}"
wait_for_instance_ready "${E2E_SERVER}"
E2E_SERVER=$ORIG_URL

assert_instance_logs_poll_all \
  "$INST_ID" \
  "API-started instance logs extension path" \
  "loading extensions" \
  "paths=/extensions/test-extension"

end_test

# --- T3: Global + API injected extensions (Additive) ---
start_test "Additive extensions: global + API paths merged"

pt_post /instances/start '{"extensionPaths":["/extensions/test-extension-api"]}'
assert_ok "instance start"
INST_ID=$(echo "$RESULT" | jq -r '.id')
INST_PORT=$(echo "$RESULT" | jq -r '.port')

E2E_SERVER="http://pinchtab:${INST_PORT}"

wait_for_instance_ready "${E2E_SERVER}"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
assert_instance_logs_poll_all \
  "$INST_ID" \
  "child instance logs merged extension paths" \
  "loading extensions" \
  "paths=/extensions/test-extension,/extensions/test-extension-api"
MERGE_PASS=$?

assert_instance_logs_poll \
  "$INST_ID" \
  "chrome initialized successfully" \
  "child instance chrome initialized"

if [ $MERGE_PASS -ne 0 ]; then
  print_extension_hints "$INST_ID"
fi

E2E_SERVER=$ORIG_URL

end_test
