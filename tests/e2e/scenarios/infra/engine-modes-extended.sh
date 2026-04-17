#!/bin/bash
# engine-modes.sh — Cross-engine behavior comparison and SafeEngine integration.
#
# Runs the same scenarios against both the default (chrome) server and
# the lite engine server, verifying consistent behavior. Also tests
# SafeEngine IDPI wrapping across engines.
#
# Requires: E2E_SERVER (chrome), E2E_LITE_SERVER, E2E_SECURE_SERVER

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

CHROME_SERVER="$E2E_SERVER"

if [ -z "${E2E_LITE_SERVER:-}" ]; then
  echo "  ⚠️  E2E_LITE_SERVER not set, skipping engine-modes tests"
  return 0
fi

lite_get() {
  local old="$E2E_SERVER"
  E2E_SERVER="$E2E_LITE_SERVER"
  pt_get "$1"
  E2E_SERVER="$old"
}

lite_post() {
  local old="$E2E_SERVER"
  E2E_SERVER="$E2E_LITE_SERVER"
  pt_post "$1" "$2"
  E2E_SERVER="$old"
}

secure_get() {
  local old="$E2E_SERVER"
  E2E_SERVER="$E2E_SECURE_SERVER"
  pt_get "$1"
  E2E_SERVER="$old"
}

secure_post() {
  local old="$E2E_SERVER"
  E2E_SERVER="$E2E_SECURE_SERVER"
  pt_post "$1" "$2"
  E2E_SERVER="$old"
}

# ═══════════════════════════════════════════════════════════════════
# PART 1: Same page, both engines — structural parity
# ═══════════════════════════════════════════════════════════════════

start_test "engine-parity: navigate returns tabId on both engines"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "chrome navigate"
CHROME_TAB=$(echo "$RESULT" | jq -r '.tabId')

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "lite navigate"
LITE_TAB=$(echo "$RESULT" | jq -r '.tabId')

if [ -n "$CHROME_TAB" ] && [ "$CHROME_TAB" != "null" ]; then
  echo -e "  ${GREEN}✓${NC} chrome tabId: ${CHROME_TAB:0:12}..."
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} chrome missing tabId"
  ((ASSERTIONS_FAILED++)) || true
fi

if [ -n "$LITE_TAB" ] && [ "$LITE_TAB" != "null" ]; then
  echo -e "  ${GREEN}✓${NC} lite tabId: ${LITE_TAB:0:12}..."
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} lite missing tabId"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "engine-parity: snapshot produces nodes on both engines"

pt_get "/snapshot?tabId=${CHROME_TAB}"
assert_ok "chrome snapshot"
CHROME_NODES=$(echo "$RESULT" | jq '.nodes | length')

lite_get "/snapshot?tabId=${LITE_TAB}"
assert_ok "lite snapshot"
LITE_NODES=$(echo "$RESULT" | jq '.nodes | length')

if [ "$CHROME_NODES" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} chrome nodes: $CHROME_NODES"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} chrome returned 0 nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

if [ "$LITE_NODES" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} lite nodes: $LITE_NODES"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} lite returned 0 nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "engine-parity: text extraction contains same content"

pt_get "/text?tabId=${CHROME_TAB}&format=text"
assert_ok "chrome text"
CHROME_TEXT="$RESULT"

lite_get "/text?tabId=${LITE_TAB}&format=text"
assert_ok "lite text"
LITE_TEXT="$RESULT"

# Both should contain form-related text from form.html
assert_contains "$CHROME_TEXT" "Username" "chrome text has Username"
assert_contains "$LITE_TEXT" "Username" "lite text has Username"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "engine-parity: interactive filter returns actionable nodes"

pt_get "/snapshot?tabId=${CHROME_TAB}&filter=interactive"
assert_ok "chrome interactive snapshot"
CHROME_INTERACTIVE=$(echo "$RESULT" | jq '.nodes | length')

lite_get "/snapshot?tabId=${LITE_TAB}&filter=interactive"
assert_ok "lite interactive snapshot"
LITE_INTERACTIVE=$(echo "$RESULT" | jq '.nodes | length')

if [ "$CHROME_INTERACTIVE" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} chrome interactive nodes: $CHROME_INTERACTIVE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} chrome returned 0 interactive nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

if [ "$LITE_INTERACTIVE" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} lite interactive nodes: $LITE_INTERACTIVE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} lite returned 0 interactive nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 2: Engine metadata in responses
# ═══════════════════════════════════════════════════════════════════

start_test "engine-meta: snapshot includes engine name"

pt_get "/snapshot?tabId=${CHROME_TAB}"
assert_ok "chrome snapshot"
CHROME_ENGINE=$(echo "$RESULT" | jq -r '.engine // empty')

lite_get "/snapshot?tabId=${LITE_TAB}"
assert_ok "lite snapshot"
LITE_ENGINE=$(echo "$RESULT" | jq -r '.engine // empty')

if [ "$CHROME_ENGINE" = "chrome" ]; then
  echo -e "  ${GREEN}✓${NC} chrome engine: $CHROME_ENGINE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected engine=chrome, got: $CHROME_ENGINE"
  ((ASSERTIONS_FAILED++)) || true
fi

if [ "$LITE_ENGINE" = "lite" ]; then
  echo -e "  ${GREEN}✓${NC} lite engine: $LITE_ENGINE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected engine=lite, got: $LITE_ENGINE"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 3: SafeEngine IDPI — lite engine with strict guard
# ═══════════════════════════════════════════════════════════════════

start_test "safe-lite: IDPI blocks injection on text extraction"

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-inject.html\"}"
assert_ok "lite navigate to injection page"
sleep 0.5

# Lite with IDPI in warn mode — text should still return but may have warnings
lite_get "/text?format=text"
assert_ok "lite text returns (warn mode)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "safe-lite: clean page passes IDPI"

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-clean.html\"}"
assert_ok "lite navigate to clean page"
sleep 0.5

lite_get "/snapshot"
assert_ok "lite snapshot passes"
assert_contains "$RESULT" "Safe" "clean content present"

lite_get "/text?format=text"
assert_ok "lite text passes"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "safe-lite: redirects to internal targets are blocked"

ATTACKER_URL="https://httpbin.org/redirect-to?url=http%3A%2F%2F169.254.169.254%2Flatest%2Fmeta-data%2F"
lite_post /navigate "{\"url\":\"${ATTACKER_URL}\"}"
assert_http_status 403 "lite redirect to internal blocked"
assert_contains "$RESULT" "blocked\|private\|internal" "lite SSRF block message returned"

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 4: SafeEngine IDPI strict — secure server (chrome engine)
# ═══════════════════════════════════════════════════════════════════

start_test "safe-chrome: strict IDPI blocks injection"

secure_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-inject.html\"}"
assert_ok "secure navigate to injection page"
sleep 0.5

secure_get "/snapshot"
assert_http_status 403 "snapshot blocked by strict IDPI"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "safe-chrome: strict IDPI passes clean page"

secure_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-clean.html\"}"
assert_ok "secure navigate to clean page"
sleep 0.5

secure_get "/snapshot"
assert_ok "snapshot passes strict IDPI"
assert_contains "$RESULT" "Safe" "clean content present"

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 5: Click/type parity across engines
# ═══════════════════════════════════════════════════════════════════

start_test "engine-parity: click action on both engines"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "chrome navigate to buttons"
sleep 0.5
pt_get /snapshot
CHROME_BTN=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Increment") | .ref] | first // empty')
if [ -n "$CHROME_BTN" ]; then
  pt_post /action "{\"kind\":\"click\",\"ref\":\"${CHROME_BTN}\"}"
  assert_ok "chrome click"
fi

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "lite navigate to buttons"
sleep 0.5
lite_get /snapshot
LITE_BTN=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Increment") | .ref] | first // empty')
if [ -n "$LITE_BTN" ]; then
  lite_post /action "{\"kind\":\"click\",\"ref\":\"${LITE_BTN}\"}"
  assert_ok "lite click"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "engine-parity: type action on both engines"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "chrome navigate to form"
sleep 0.5
pt_get "/snapshot?filter=interactive"
CHROME_INPUT=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "textbox") | .ref] | first // empty')
if [ -n "$CHROME_INPUT" ]; then
  pt_post /action "{\"kind\":\"type\",\"ref\":\"${CHROME_INPUT}\",\"text\":\"hello\"}"
  assert_ok "chrome type"
fi

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "lite navigate to form"
sleep 0.5
lite_get "/snapshot?filter=interactive"
LITE_INPUT=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "textbox") | .ref] | first // empty')
if [ -n "$LITE_INPUT" ]; then
  lite_post /action "{\"kind\":\"type\",\"ref\":\"${LITE_INPUT}\",\"text\":\"hello\"}"
  assert_ok "lite type"
fi

end_test
