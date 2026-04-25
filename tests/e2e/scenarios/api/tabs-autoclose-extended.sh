#!/bin/bash
# tabs-autoclose-extended.sh — verifies the auto-close-after-use lifecycle policy.
#
# Requires the server to be started with tests/e2e/config/pinchtab-autoclose.json
# (lifecycle=close_idle, closeDelaySec=1). The default e2e config uses the
# production 5-minute auto-close delay, which is too slow for these assertions.
#
# Behaviour under test:
#   1. /text on a tab arms an auto-close timer; the tab disappears after the delay.
#   2. A second /text within the delay window resets the timer (tab survives).
#   3. /navigate cancels a pending auto-close (tab survives until next read).
#   4. /snapshot and /action also arm the timer.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

AUTOCLOSE_ORIG_SERVER="$E2E_SERVER"
if [ -n "${E2E_AUTOCLOSE_SERVER:-}" ]; then
  E2E_SERVER="$E2E_AUTOCLOSE_SERVER"
fi

AUTOCLOSE_CONFIG=$(e2e_curl -sf "${E2E_SERVER}/api/config" 2>/dev/null || true)
if ! echo "$AUTOCLOSE_CONFIG" | jq -e '(.config // .).instanceDefaults.tabPolicy.lifecycle == "close_idle" and ((.config // .).instanceDefaults.tabPolicy.closeDelaySec | tonumber) == 1' >/dev/null 2>&1; then
  echo -e "  ${YELLOW}⚠${NC} auto-close lifecycle server not configured at ${E2E_SERVER}, skipping"
  E2E_SERVER="$AUTOCLOSE_ORIG_SERVER"
  return 0 2>/dev/null || exit 0
fi

# Auto-close delay in seconds (must match closeDelaySec in pinchtab-autoclose.json).
CLOSE_DELAY=1
# A safety margin past the delay before asserting the tab is gone.
PAST_CLOSE_WAIT=1.8
# A short interval well under the delay, used to verify reset semantics.
WITHIN_DELAY=0.4

tab_id_exists() {
  local tab_id="$1"
  local tabs_json
  tabs_json=$(e2e_curl -sf "${E2E_SERVER}/tabs" 2>/dev/null || true)
  if [ -z "$tabs_json" ]; then
    return 2
  fi

  echo "$tabs_json" | jq -e --arg id "$tab_id" 'any(.tabs[]?; .id == $id)' >/dev/null 2>&1
}

assert_tab_id_open() {
  local tab_id="$1"
  local desc="${2:-tab is open}"

  tab_id_exists "$tab_id"
  local status=$?
  if [ "$status" -eq 0 ]; then
    echo -e "  ${GREEN}✓${NC} ${desc}: ${tab_id:0:12}..."
    ((ASSERTIONS_PASSED++)) || true
  elif [ "$status" -eq 1 ]; then
    echo -e "  ${RED}✗${NC} ${desc}: tab ${tab_id:0:12}... is missing"
    ((ASSERTIONS_FAILED++)) || true
  else
    echo -e "  ${RED}✗${NC} ${desc}: unable to fetch tab list"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

wait_for_tab_id_closed() {
  local tab_id="$1"
  local desc="${2:-tab closed}"
  local attempts="${3:-20}"
  local delay="${4:-0.25}"

  for _ in $(seq 1 "$attempts"); do
    tab_id_exists "$tab_id"
    local status=$?
    if [ "$status" -eq 1 ]; then
      echo -e "  ${GREEN}✓${NC} ${desc}: ${tab_id:0:12}..."
      ((ASSERTIONS_PASSED++)) || true
      return 0
    fi
    sleep "$delay"
  done

  echo -e "  ${RED}✗${NC} ${desc}: tab ${tab_id:0:12}... is still open"
  ((ASSERTIONS_FAILED++)) || true
  return 1
}

# ─────────────────────────────────────────────────────────────────
start_test "auto-close: /text arms a timer that fires after the delay"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)
show_tab "created" "$TAB_ID"

pt_get "/text?tabId=${TAB_ID}&format=text"
assert_ok "tab text (arms auto-close)"

wait_for_tab_id_closed "$TAB_ID" "tab closed after /text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-close: a second /text within the delay window resets the timer"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/text?tabId=${TAB_ID}&format=text"
assert_ok "first /text"

# Second /text well within the delay — must reset the timer.
sleep "$WITHIN_DELAY"
pt_get "/text?tabId=${TAB_ID}&format=text"
assert_ok "second /text resets timer"

# Wait less than (CLOSE_DELAY + WITHIN_DELAY) but more than CLOSE_DELAY since
# the first /text — if reset worked, the tab is still alive here.
sleep "$WITHIN_DELAY"
assert_tab_id_open "$TAB_ID" "tab still open after reset"

# Now wait past the second timer's deadline; the tab should be gone.
wait_for_tab_id_closed "$TAB_ID" "tab closed after reset deadline"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-close: /navigate cancels a pending timer"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/text?tabId=${TAB_ID}&format=text"
assert_ok "/text arms auto-close"

# Re-navigate well within the close delay — should cancel the pending timer.
sleep "$WITHIN_DELAY"
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "/navigate cancels pending close"

# Wait past the original deadline; the tab must still be alive because
# /navigate dropped the timer and no further /text re-armed it.
sleep "$PAST_CLOSE_WAIT"
assert_tab_id_open "$TAB_ID" "tab survived after /navigate cancel"

# Clean up: explicit close so this test doesn't leak a tab into the next.
pt_post "/tabs/${TAB_ID}/close" -d '{}' >/dev/null

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-close: /snapshot also arms the timer"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
TAB_ID=$(get_tab_id)

pt_get "/snapshot?tabId=${TAB_ID}"
assert_ok "/snapshot arms auto-close"

wait_for_tab_id_closed "$TAB_ID" "tab closed after /snapshot"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "auto-close: /action also arms the timer"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
TAB_ID=$(get_tab_id)

# A read-style action: pressing a key is sufficient to mark the tab "used".
pt_post /action -d "{\"kind\":\"press\",\"key\":\"Tab\",\"tabId\":\"${TAB_ID}\"}"
assert_ok "/action arms auto-close"

wait_for_tab_id_closed "$TAB_ID" "tab closed after /action"

end_test

E2E_SERVER="$AUTOCLOSE_ORIG_SERVER"
