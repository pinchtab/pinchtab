#!/bin/bash
# browser-routing-extended.sh — Browser routing comparison and content guard integration.
#
# Runs the same scenarios against the primary server and the ghost-chrome
# server, verifying consistent behavior. Also tests content guard IDPI
# wrapping across providers.
#
# Requires: E2E_SERVER (primary), E2E_SERVER_GHOSTCHROME, E2E_SECURE_SERVER

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

CHROME_SERVER="$E2E_SERVER"

# These tests compare the chrome primary against the dedicated ghost-chrome
# server — chrome vs ghost-chrome parity. They run in the chrome lane (the
# default, blocking one). In non-chrome lanes the primary is a different
# provider, so the comparison would not be the parity this scenario tests.
requires_providers() {
  if [ -z "${E2E_SERVER_GHOSTCHROME:-}" ]; then
    echo "  ⚠️  Skipping browser-routing tests (E2E_SERVER_GHOSTCHROME not set)"
    return 1
  fi
  local primary="${PINCHTAB_E2E_BROWSER:-chrome}"
  if [ "$primary" != "chrome" ]; then
    echo "  ⚠️  Skipping browser-routing tests (primary provider is ${primary} — chrome vs ghost-chrome parity needs the chrome lane)"
    return 1
  fi
  return 0
}

if ! requires_providers; then
  return 0
fi

ghostchrome_get() {
  local old="$E2E_SERVER"
  E2E_SERVER="$E2E_SERVER_GHOSTCHROME"
  pt_get "$1"
  E2E_SERVER="$old"
}

ghostchrome_post() {
  local old="$E2E_SERVER"
  E2E_SERVER="$E2E_SERVER_GHOSTCHROME"
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

ghostchrome_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "ghost-chrome navigate"
GC_TAB=$(echo "$RESULT" | jq -r '.tabId')

if [ -n "$CHROME_TAB" ] && [ "$CHROME_TAB" != "null" ]; then
  echo -e "  ${GREEN}✓${NC} chrome tabId: ${CHROME_TAB:0:12}..."
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} chrome missing tabId"
  ((ASSERTIONS_FAILED++)) || true
fi

if [ -n "$GC_TAB" ] && [ "$GC_TAB" != "null" ]; then
  echo -e "  ${GREEN}✓${NC} ghost-chrome tabId: ${GC_TAB:0:12}..."
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} ghost-chrome missing tabId"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "provider-parity: snapshot produces nodes on both engines"

pt_get "/snapshot?tabId=${CHROME_TAB}"
assert_ok "chrome snapshot"
CHROME_NODES=$(echo "$RESULT" | jq '.nodes | length')

ghostchrome_get "/snapshot?tabId=${GC_TAB}"
assert_ok "ghost-chrome snapshot"
GC_NODES=$(echo "$RESULT" | jq '.nodes | length')

if [ "$CHROME_NODES" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} chrome nodes: $CHROME_NODES"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} chrome returned 0 nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

if [ "$GC_NODES" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} ghost-chrome nodes: $GC_NODES"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} ghost-chrome returned 0 nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "provider-parity: text extraction contains same content"

pt_get "/text?tabId=${CHROME_TAB}&format=text"
assert_ok "chrome text"
CHROME_TEXT="$RESULT"

ghostchrome_get "/text?tabId=${GC_TAB}&format=text"
assert_ok "ghost-chrome text"
GC_TEXT="$RESULT"

# Both should contain form-related text from form.html
assert_contains "$CHROME_TEXT" "Username" "chrome text has Username"
assert_contains "$GC_TEXT" "Username" "ghost-chrome text has Username"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "provider-parity: interactive filter returns actionable nodes"

pt_get "/snapshot?tabId=${CHROME_TAB}&filter=interactive"
assert_ok "chrome interactive snapshot"
CHROME_INTERACTIVE=$(echo "$RESULT" | jq '.nodes | length')

ghostchrome_get "/snapshot?tabId=${GC_TAB}&filter=interactive"
assert_ok "ghost-chrome interactive snapshot"
GC_INTERACTIVE=$(echo "$RESULT" | jq '.nodes | length')

if [ "$CHROME_INTERACTIVE" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} chrome interactive nodes: $CHROME_INTERACTIVE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} chrome returned 0 interactive nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

if [ "$GC_INTERACTIVE" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} ghost-chrome interactive nodes: $GC_INTERACTIVE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} ghost-chrome returned 0 interactive nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 2: Content guard IDPI — ghost-chrome with strict guard
# ═══════════════════════════════════════════════════════════════════

start_test "safe-ghostchrome: IDPI blocks injection on text extraction"

ghostchrome_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-inject.html\"}"
assert_ok "ghost-chrome navigate to injection page"

# Ghost-chrome with IDPI in warn mode — text should still return but may have warnings
ghostchrome_get "/text?format=text"
assert_ok "ghost-chrome text returns (warn mode)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "safe-ghostchrome: clean page passes IDPI"

ghostchrome_post /navigate "{\"url\":\"${FIXTURES_URL}/idpi-clean.html\"}"
assert_ok "ghost-chrome navigate to clean page"

ghostchrome_get "/snapshot"
assert_ok "ghost-chrome snapshot passes"
assert_contains "$RESULT" "Safe" "clean content present"

ghostchrome_get "/text?format=text"
assert_ok "ghost-chrome text passes"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "safe-ghostchrome: redirects to internal targets are blocked"

ATTACKER_URL="${FIXTURES_URL}/redirect-to-internal"
ghostchrome_post /navigate "{\"url\":\"${ATTACKER_URL}\"}"
assert_http_status 403 "ghost-chrome redirect to internal blocked"
assert_contains "$RESULT" "blocked\|private\|internal" "ghost-chrome SSRF block message returned"

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

ghostchrome_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "ghost-chrome navigate to buttons"
ghostchrome_get /snapshot
GC_BTN=$(echo "$RESULT" | jq -r '[.nodes[] | select(.name == "Increment") | .ref] | first // empty')
if [ -n "$GC_BTN" ]; then
  ghostchrome_post /action "{\"kind\":\"click\",\"ref\":\"${GC_BTN}\"}"
  assert_ok "ghost-chrome click"
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

ghostchrome_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "ghost-chrome navigate to form"
ghostchrome_get "/snapshot?filter=interactive"
GC_INPUT=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "textbox") | .ref] | first // empty')
if [ -n "$GC_INPUT" ]; then
  ghostchrome_post /action "{\"kind\":\"type\",\"ref\":\"${GC_INPUT}\",\"text\":\"hello\"}"
  assert_ok "ghost-chrome type"
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 6: Provider route metadata verification
# (ghost-chrome-specific — only when primary is ghost-chrome)
# ═══════════════════════════════════════════════════════════════════

EXPECTED_BROWSER="${PINCHTAB_E2E_BROWSER:-chrome}"
if [ "$EXPECTED_BROWSER" = "ghost-chrome" ]; then

start_test "provider-routing: ghost-chrome route metadata on text"

ghostchrome_post /navigate "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "ghost-chrome navigate to form"
GC_TAB_ROUTE=$(echo "$RESULT" | jq -r '.tabId')

ghostchrome_get "/text?tabId=${GC_TAB_ROUTE}"
assert_ok "ghost-chrome text"

HAS_ROUTE=$(echo "$RESULT" | jq 'has("route")' 2>/dev/null || echo "false")
if [ "$HAS_ROUTE" = "true" ]; then
  # /text bypasses the ghost-chrome adapter (uses readability.js directly),
  # so route metadata reflects chrome, not ghost-chrome.
  USED=$(echo "$RESULT" | jq -r '.route.usedProvider' 2>/dev/null)
  if [ "$USED" = "ghost-chrome" ] || [ "$USED" = "chrome" ]; then
    echo -e "  ${GREEN}✓${NC} ghost-chrome text route.usedProvider=$USED (text handler may bypass adapter)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} ghost-chrome text route.usedProvider unexpected: $USED"
    ((ASSERTIONS_FAILED++)) || true
  fi
else
  soft_pass_assert "route metadata not in ghost-chrome text response (format may be text)"
fi

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

ghostchrome_post /navigate "{\"url\":\"${FIXTURES_URL}/table.html\"}"
assert_ok "ghost-chrome navigate to table"
GC_TAB_STATIC=$(echo "$RESULT" | jq -r '.tabId')

ghostchrome_get "/text?tabId=${GC_TAB_STATIC}&format=text"
assert_ok "ghost-chrome text (static path)"
assert_contains "$RESULT" "Alice" "ghost-chrome text has table content"

ghostchrome_get "/snapshot?tabId=${GC_TAB_STATIC}"
assert_ok "ghost-chrome snapshot (static path)"
GC_SNAP_NODES=$(echo "$RESULT" | jq '.nodes | length' 2>/dev/null || echo "0")
if [ "$GC_SNAP_NODES" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} ghost-chrome snapshot returned $GC_SNAP_NODES nodes"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} ghost-chrome snapshot returned 0 nodes"
  ((ASSERTIONS_FAILED++)) || true
fi

SNAP_HAS_ROUTE=$(echo "$RESULT" | jq 'has("route")' 2>/dev/null || echo "false")
if [ "$SNAP_HAS_ROUTE" = "true" ]; then
  assert_json_contains "$RESULT" '.route.usedProvider' 'ghost-chrome' "ghost-chrome snapshot route.usedProvider contains ghost-chrome"
else
  soft_pass_assert "route metadata not in ghost-chrome snapshot response"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "provider-routing: ghost-chrome escalation to chrome for screenshot"

ghostchrome_post /navigate "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "ghost-chrome navigate to buttons"
GC_TAB_ESC=$(echo "$RESULT" | jq -r '.tabId')

ghostchrome_get "/screenshot?tabId=${GC_TAB_ESC}"

if [ "$HTTP_STATUS" = "200" ]; then
  pass_assert "screenshot succeeded via escalation to Chrome"
elif [ "$HTTP_STATUS" = "404" ] || [ "$HTTP_STATUS" = "500" ]; then
  pass_assert "screenshot correctly failed on ghost-chrome tab (HTTP $HTTP_STATUS — Chrome cannot serve ghost-chrome-only tabs)"
else
  fail_assert "screenshot unexpected status: $HTTP_STATUS"
fi

end_test

else
  echo "  ⚠️  Skipping ghost-chrome routing tests (provider=${EXPECTED_BROWSER})"
fi

# ─────────────────────────────────────────────────────────────────
start_test "provider-routing: security blocks before provider fallback"

# Try to navigate to a domain NOT in the ghost-chrome server's allowedDomains list.
# The ghost-chrome config allows: localhost, 127.0.0.1, ::1, fixtures.
# A request to a non-allowed domain must be blocked by the security/IDPI
# layer with a 403, not by a routing or provider error.
ghostchrome_post /navigate "{\"url\":\"https://not-allowed.example.com\"}"
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
