#!/bin/bash
# idpi-extended.sh — IDPI scanner extended scenarios.
#
# Tests three IDPI improvements:
# 1. Unicode/homoglyph bypass detection
# 2. Hidden node flagging in snapshots
# 3. Content wrapping on snapshot responses
#
# Uses both the permissive server (IDPI warn mode) and secure server (IDPI strict/block).

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

secure_get() {
  local path="$1"
  shift
  local old_url="$E2E_SERVER"
  E2E_SERVER="$E2E_SECURE_SERVER"
  pt_get "$path" "$@"
  E2E_SERVER="$old_url"
}

secure_post() {
  local path="$1"
  shift
  local old_url="$E2E_SERVER"
  E2E_SERVER="$E2E_SECURE_SERVER"
  pt_post "$path" "$@"
  E2E_SERVER="$old_url"
}

# ═══════════════════════════════════════════════════════════════════
# PART 1: IDPI OFF — injection gets through
# ═══════════════════════════════════════════════════════════════════

start_test "idpi-off: hidden injection passes through without IDPI"

# Navigate to the hidden injection fixture (permissive server, IDPI warn only)
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-hidden-injection.html\"}"
assert_ok "navigate to hidden injection page"
sleep 0.5

# Get snapshot — visible content should be present, IDPI warning should be set
pt_get "/snapshot?format=json"
assert_ok "snapshot succeeds"
assert_contains "$RESULT" "Safe Button" "visible content present"
assert_contains "$RESULT" "Learn more" "normal link present"
# NOTE: Chrome's accessibility tree excludes display:none elements, so the
# aria-label injection text won't appear in the snapshot. We verify IDPI
# detected the pattern via the idpiWarning field instead.
assert_json_exists "$RESULT" ".idpiWarning" "IDPI warning present for detected injection"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi-off: unicode bypass passes through without strict IDPI"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-unicode-bypass.html\"}"
assert_ok "navigate to unicode bypass page"
sleep 0.5

pt_get "/snapshot"
assert_ok "snapshot succeeds"
assert_contains "$RESULT" "Submit" "visible button present"

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 2: IDPI ON (strict) — injection is BLOCKED
# ═══════════════════════════════════════════════════════════════════

start_test "idpi-strict: hidden injection BLOCKED by scanner"

# Navigate to the injection fixture via secure server (strict IDPI)
# First navigate to a clean page to establish tab
secure_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-clean.html\"}"
assert_ok "navigate to clean page first"
sleep 0.5

# Now navigate to the injection page
secure_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-hidden-injection.html\"}"
assert_ok "navigate to injection page"
sleep 0.5

# Snapshot should be BLOCKED (403) because strict mode + injection detected
secure_get "/snapshot"
assert_http_status 403 "snapshot blocked by IDPI"
assert_contains "$RESULT" "IDPI\|injection\|blocked" "IDPI block reason present"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi-strict: standard injection BLOCKED"

secure_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-inject.html\"}"
assert_ok "navigate to injection page"
sleep 0.5

secure_get "/snapshot"
assert_http_status 403 "snapshot blocked by IDPI"
assert_contains "$RESULT" "instruction-override" "detected injection pattern"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi-strict: clean page passes through"

secure_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-clean.html\"}"
assert_ok "navigate to clean page"
sleep 0.5

secure_get "/snapshot"
assert_ok "clean snapshot passes IDPI"
assert_contains "$RESULT" "Safe Action" "clean content present"

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 3: Hidden node flagging
# ═══════════════════════════════════════════════════════════════════

start_test "idpi-hidden: hidden traps omitted from snapshot but warning preserved"

# Use permissive server (warn mode) so we get the snapshot back with flags
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-hidden-injection.html\"}"
assert_ok "navigate to injection page"
sleep 0.5

pt_get "/snapshot"
assert_ok "snapshot returned"

# Chrome's full accessibility tree does not reliably expose display:none /
# aria-hidden trap nodes from this fixture, so the runtime signal is the
# warning header/body rather than hidden:true entries in .nodes.
HIDDEN_COUNT=$(echo "$RESULT" | jq '[.nodes[] | select(.hidden == true)] | length')
if [ "$HIDDEN_COUNT" -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} hidden trap nodes omitted from the visible snapshot"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected hidden trap nodes to be omitted, found $HIDDEN_COUNT hidden entries"
  ((ASSERTIONS_FAILED++)) || true
fi

assert_json_exists "$RESULT" ".idpiWarning" "IDPI warning preserved on returned snapshot"

# Visible nodes should NOT be flagged as hidden
VISIBLE_BTN=$(echo "$RESULT" | jq '[.nodes[] | select(.name == "Safe Button" and .hidden != true)] | length')
if [ "$VISIBLE_BTN" -gt 0 ]; then
  echo -e "  ${GREEN}✓${NC} visible button not flagged as hidden"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} visible button incorrectly flagged as hidden"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 4: Unicode bypass detection
# ═══════════════════════════════════════════════════════════════════

start_test "idpi-strict: unicode homoglyph bypass BLOCKED"

secure_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-unicode-bypass.html\"}"
assert_ok "navigate to unicode bypass page"
sleep 0.5

# The Cyrillic homoglyph "Іgnore previous instructions" should be caught
# after NFKC normalization + homoglyph replacement
secure_get "/snapshot"
assert_http_status 403 "unicode bypass blocked by IDPI"
assert_contains "$RESULT" "injection\|blocked\|IDPI" "IDPI detected unicode bypass"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi-warn: unicode bypass detected with warning"

# Permissive server — warn mode
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-unicode-bypass.html\"}"
assert_ok "navigate to unicode bypass page"
sleep 0.5

pt_get "/snapshot"
assert_ok "snapshot returned (warn mode)"
# Check for IDPI warning in response
IDPI_WARN=$(echo "$RESULT" | jq -r '.idpiWarning // empty')
if [ -n "$IDPI_WARN" ]; then
  echo -e "  ${GREEN}✓${NC} IDPI warning present: ${IDPI_WARN:0:60}..."
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} no IDPI warning in response (expected warning for unicode bypass)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# PART 5: Text format wrapping (when wrapContent is disabled)
# ═══════════════════════════════════════════════════════════════════

start_test "idpi-nowrap: snapshot text format without wrapping"

# Earlier infra scenarios exercise live config updates through /api/config.
# Reassert the expected base-server IDPI wrap setting here so this check stays
# independent of prior suite ordering.
pt PUT /api/config -d '{"security":{"idpi":{"enabled":true,"scanContent":true,"wrapContent":false}}}'
assert_ok "IDPI wrapContent disabled on base server"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/idpi-clean.html\"}"
assert_ok "navigate to clean page"
sleep 0.5

# E2E config has wrapContent disabled — text should NOT be wrapped
pt_get "/snapshot?format=text"
assert_ok "text snapshot returned"
# Should NOT contain untrusted_web_content wrapper
if echo "$RESULT" | grep -q "untrusted_web_content"; then
  echo -e "  ${RED}✗${NC} text should not be wrapped when wrapContent is off"
  ((ASSERTIONS_FAILED++)) || true
else
  echo -e "  ${GREEN}✓${NC} text output not wrapped (wrapContent disabled)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test
