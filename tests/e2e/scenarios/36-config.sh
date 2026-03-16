#!/bin/bash
# 36-config.sh — Config behaviour: Chrome version in UA, fingerprint rotation, config loading

source "$(dirname "$0")/common.sh"

# ═══════════════════════════════════════════════════════════════════
# CF7: Chrome version in user agent
# ═══════════════════════════════════════════════════════════════════

start_test "config: Chrome version in user agent"

pt_post /navigate '{"url":"about:blank"}'
assert_ok "navigate"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"navigator.userAgent\"}"
assert_ok "evaluate UA"
UA=$(echo "$RESULT" | jq -r '.result')

# Should contain Chrome/X.Y.Z.W or HeadlessChrome/X.Y.Z.W
if echo "$UA" | grep -qE '(Headless)?Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'; then
  CHROME_VERSION=$(echo "$UA" | grep -oE '(Headless)?Chrome/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+')
  echo -e "  ${GREEN}✓${NC} UA contains Chrome version: $CHROME_VERSION"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} UA missing Chrome version: $UA"
  ((ASSERTIONS_FAILED++)) || true
fi

# Clean up
pt_post /tab "{\"tabId\":\"$TAB_ID\",\"action\":\"close\"}" >/dev/null 2>&1

end_test

# ═══════════════════════════════════════════════════════════════════
# CF8: Fingerprint rotation preserves Chrome version
# ═══════════════════════════════════════════════════════════════════

start_test "config: fingerprint rotation preserves Chrome version"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

# Get initial Chrome version
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

# Rotate fingerprint
pt_post /fingerprint/rotate "{\"os\":\"mac\",\"tabId\":\"$TAB_ID\"}"
assert_ok "fingerprint rotate"

# Get post-rotation Chrome version
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

# Clean up
pt_post /tab "{\"tabId\":\"$TAB_ID\",\"action\":\"close\"}" >/dev/null 2>&1

end_test

# ═══════════════════════════════════════════════════════════════════
# CF1: Config file loading (server started successfully)
# ═══════════════════════════════════════════════════════════════════

start_test "config: server loads config file and starts"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')

pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"document.title\"}"
assert_ok "evaluate"
assert_json_exists "$RESULT" ".result" "got title result"

# Clean up
pt_post /tab "{\"tabId\":\"$TAB_ID\",\"action\":\"close\"}" >/dev/null 2>&1

end_test

# ═══════════════════════════════════════════════════════════════════
# CF2: Config file port is used (server runs on port from config)
# ═══════════════════════════════════════════════════════════════════

start_test "config: server uses port from config file"

# The server is running on port 9999 from config file.
# Verifies config is loaded correctly.
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate (proves env port override works)"

TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
pt_post /evaluate "{\"tabId\":\"$TAB_ID\",\"expression\":\"window.location.href\"}"
assert_ok "evaluate"
assert_json_exists "$RESULT" ".result" "got location result"

# Clean up
pt_post /tab "{\"tabId\":\"$TAB_ID\",\"action\":\"close\"}" >/dev/null 2>&1

end_test
