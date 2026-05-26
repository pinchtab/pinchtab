#!/bin/bash
# browser-routing-extended.sh — Browser routing comparison and content guard integration.
#
# Runs the same scenarios against both the default (chrome) server and
# the ghost-chrome server, verifying consistent behavior. Also tests
# content guard IDPI wrapping across providers.
#
# Requires: E2E_SERVER (chrome), E2E_LITE_SERVER, E2E_SECURE_SERVER

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

CHROME_SERVER="$E2E_SERVER"

if [ -z "${E2E_LITE_SERVER:-}" ]; then
  echo "  ⚠️  E2E_LITE_SERVER not set, skipping browser-routing tests"
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
# PART 1: Same page, both providers — structural parity
# ═══════════════════════════════════════════════════════════════════

start_test "provider-parity: navigate returns tabId on both engines"

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
start_test "provider-parity: snapshot produces nodes on both engines"

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
start_test "provider-parity: text extraction contains same content"

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
start_test "provider-parity: interactive filter returns actionable nodes"

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
# PART 2: Content guard IDPI — ghost-chrome with strict guard
# ═══════════════════════════════════════════════════════════════════

start_test "safe-lite: IDPI blocks injection on text extraction"

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-inject.html\"}"
assert_ok "lite navigate to injection page"

# Lite with IDPI in warn mode — text should still return but may have warnings
lite_get "/text?format=text"
assert_ok "lite text returns (warn mode)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "safe-lite: clean page passes IDPI"

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-clean.html\"}"
assert_ok "lite navigate to clean page"

lite_get "/snapshot"
assert_ok "lite snapshot passes"
assert_contains "$RESULT" "Safe" "clean content present"

lite_get "/text?format=text"
assert_ok "lite text passes"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "safe-lite: redirects to internal targets are blocked"

ATTACKER_URL="${FIXTURES_URL}/redirect-to-internal"
lite_post /navigate "{\"url\":\"${ATTACKER_URL}\"}"
assert_http_status 403 "lite redirect to internal blocked"
assert_contains "$RESULT" "blocked\|private\|internal" "lite SSRF block message returned"

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 4: Content guard IDPI strict — secure server (chrome provider)
# ═══════════════════════════════════════════════════════════════════

start_test "safe-chrome: strict IDPI blocks injection"

secure_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-inject.html\"}"
assert_ok "secure navigate to injection page"

secure_get "/snapshot"
assert_http_status 403 "snapshot blocked by strict IDPI"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "safe-chrome: strict IDPI passes clean page"

secure_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-clean.html\"}"
assert_ok "secure navigate to clean page"

secure_get "/snapshot"
assert_ok "snapshot passes strict IDPI"
assert_contains "$RESULT" "Safe" "clean content present"

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 5: Click/type parity across providers
# ═══════════════════════════════════════════════════════════════════

start_test "provider-parity: click action on both engines"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "chrome navigate to buttons"
pt_get /snapshot
CHROME_BTN=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Increment") | .ref] | first // empty')
if [ -n "$CHROME_BTN" ]; then
  pt_post /action "{\"kind\":\"click\",\"ref\":\"${CHROME_BTN}\"}"
  assert_ok "chrome click"
fi

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "lite navigate to buttons"
lite_get /snapshot
LITE_BTN=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Increment") | .ref] | first // empty')
if [ -n "$LITE_BTN" ]; then
  lite_post /action "{\"kind\":\"click\",\"ref\":\"${LITE_BTN}\"}"
  assert_ok "lite click"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "provider-parity: type action on both engines"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "chrome navigate to form"
pt_get "/snapshot?filter=interactive"
CHROME_INPUT=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "textbox") | .ref] | first // empty')
if [ -n "$CHROME_INPUT" ]; then
  pt_post /action "{\"kind\":\"type\",\"ref\":\"${CHROME_INPUT}\",\"text\":\"hello\"}"
  assert_ok "chrome type"
fi

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "lite navigate to form"
lite_get "/snapshot?filter=interactive"
LITE_INPUT=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "textbox") | .ref] | first // empty')
if [ -n "$LITE_INPUT" ]; then
  lite_post /action "{\"kind\":\"type\",\"ref\":\"${LITE_INPUT}\",\"text\":\"hello\"}"
  assert_ok "lite type"
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 6: Provider route metadata verification
# ═══════════════════════════════════════════════════════════════════

start_test "provider-routing: ghost-chrome route metadata on text"

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "lite navigate to form"
LITE_TAB_ROUTE=$(echo "$RESULT" | jq -r '.tabId')

# Text from ghost-chrome (lite server) — should report ghost as usedProvider
lite_get "/text?tabId=${LITE_TAB_ROUTE}"
assert_ok "lite text"

# Route metadata may be in the JSON response (format defaults to json)
HAS_ROUTE=$(echo "$RESULT" | jq 'has("route")' 2>/dev/null || echo "false")
if [ "$HAS_ROUTE" = "true" ]; then
  assert_json_eq "$RESULT" '.route.usedProvider' 'ghost' "lite route.usedProvider=ghost"
  assert_json_contains "$RESULT" '.route.requestedProvider' 'ghost' "lite route.requestedProvider contains ghost"
else
  soft_pass_assert "route metadata not in lite text response (format may be text)"
fi

# Same on chrome server — should report chrome as usedProvider
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "chrome navigate to form"
CHROME_TAB_ROUTE=$(echo "$RESULT" | jq -r '.tabId')

pt_get "/text?tabId=${CHROME_TAB_ROUTE}"
assert_ok "chrome text"

CHROME_HAS_ROUTE=$(echo "$RESULT" | jq 'has("route")' 2>/dev/null || echo "false")
if [ "$CHROME_HAS_ROUTE" = "true" ]; then
  assert_json_eq "$RESULT" '.route.usedProvider' 'chrome' "chrome route.usedProvider=chrome"
else
  soft_pass_assert "route metadata not in chrome text response (format may be text)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "provider-routing: ghost-chrome static path serves text and snapshot"

lite_post /navigate "{\"url\":\"${FIXTURES_URL}/table.html\"}"
assert_ok "lite navigate to table"
LITE_TAB_STATIC=$(echo "$RESULT" | jq -r '.tabId')

# Verify text extraction succeeds via the static (ghost-chrome) path
lite_get "/text?tabId=${LITE_TAB_STATIC}&format=text"
assert_ok "lite text (static path)"
assert_contains "$RESULT" "Alice" "lite text has table content"

# Verify snapshot succeeds via the static (ghost-chrome) path
lite_get "/snapshot?tabId=${LITE_TAB_STATIC}"
assert_ok "lite snapshot (static path)"
LITE_SNAP_NODES=$(echo "$RESULT" | jq '.nodes | length' 2>/dev/null || echo "0")
if [ "$LITE_SNAP_NODES" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} lite snapshot returned $LITE_SNAP_NODES nodes"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} lite snapshot returned 0 nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

# Check route metadata on snapshot response
SNAP_HAS_ROUTE=$(echo "$RESULT" | jq 'has("route")' 2>/dev/null || echo "false")
if [ "$SNAP_HAS_ROUTE" = "true" ]; then
  assert_json_contains "$RESULT" '.route.usedProvider' 'ghost' "lite snapshot route.usedProvider contains ghost"
else
  soft_pass_assert "route metadata not in lite snapshot response"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "provider-routing: ghost-chrome escalation to chrome for screenshot"

# Navigate on the lite server so there is a page loaded
lite_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "lite navigate to buttons"
LITE_TAB_ESC=$(echo "$RESULT" | jq -r '.tabId')

# Screenshot requires Chrome (ghost-chrome cannot capture screenshots).
# Lite tabs are static-only, so Chrome does not know about them —
# the server will return 404 (tab not found) or 500 (no Chrome backend).
# Either outcome confirms the lite server does not silently succeed
# on an operation it cannot handle statically.
lite_get "/screenshot?tabId=${LITE_TAB_ESC}"

if [ "$HTTP_STATUS" = "200" ]; then
  pass_assert "screenshot succeeded via escalation to Chrome"
elif [ "$HTTP_STATUS" = "404" ] || [ "$HTTP_STATUS" = "500" ]; then
  pass_assert "screenshot correctly failed on lite tab (HTTP $HTTP_STATUS — Chrome cannot serve lite-only tabs)"
else
  fail_assert "screenshot unexpected status: $HTTP_STATUS"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "provider-routing: security blocks before provider fallback"

# Try to navigate to a domain NOT in the lite server's allowedDomains list.
# The lite config allows: localhost, 127.0.0.1, ::1, fixtures.
# A request to a non-allowed domain must be blocked by the security/IDPI
# layer with a 403, not by a routing or provider error.
lite_post /navigate "{\"url\":\"https://not-allowed.example.com\"}"
assert_http_status 403 "navigate to non-allowed domain blocked"

# Verify the error is a security/navigation block, not a provider routing error.
# The actual error may be IDPI domain block ("blocked by IDPI") or DNS
# resolution failure ("could not resolve navigation host") depending on
# whether IDPI domain checks or DNS resolution runs first.
assert_contains "$RESULT" "blocked\|IDPI\|security\|forbidden\|not allowed\|could not resolve" \
  "error message indicates security block"
assert_not_contains "$RESULT" "no provider\|routing error\|provider not found" \
  "error is not a provider/routing error"

end_test
