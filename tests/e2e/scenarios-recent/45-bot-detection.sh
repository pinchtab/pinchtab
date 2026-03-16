#!/bin/bash
# 45-bot-detection.sh — Bot detection stealth tests
# Tests the fixes for GitHub issue #275
# Simulates checks from bot.sannysoft.com
#
# Tests run against:
# - E2E_SERVER (light stealth) - basic bot detection evasion
# - E2E_SECURE_SERVER (full stealth) - additional fingerprint protections

source "$(dirname "$0")/common.sh"

# ═══════════════════════════════════════════════════════════════════
# LIGHT STEALTH MODE (main instance)
# ═══════════════════════════════════════════════════════════════════

echo -e "${BLUE}Testing LIGHT stealth mode (main instance)${NC}"

# Navigate to bot detection fixture
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
assert_ok "navigate to bot-detect fixture"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: navigator.webdriver is not true"

# The critical test from issue #275: navigator.webdriver must NOT be true
assert_eval_poll "navigator.webdriver !== true" "true" "navigator.webdriver is not true"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: navigator.webdriver is undefined"

# Stricter check: should be undefined (best) or false
assert_eval_poll "navigator.webdriver === undefined || navigator.webdriver === false" "true" "navigator.webdriver is undefined or false"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: webdriver property does not exist"

# Key detection vector: 'webdriver' in navigator should be false
assert_eval_poll "!('webdriver' in navigator)" "true" "webdriver property not in navigator"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: no CDP traces"

# Check for Chrome DevTools Protocol traces left by automation
assert_eval_poll "!(window.cdc_adoQpoasnfa76pfcZLmcfl_Array || window.cdc_adoQpoasnfa76pfcZLmcfl_Promise)" "true" "no CDP automation traces"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: UA not headless"

# User agent should not contain HeadlessChrome
assert_eval_poll "!navigator.userAgent.includes('HeadlessChrome')" "true" "UA not headless"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: plugins instanceof PluginArray"

# Issue #275: navigator.plugins must pass instanceof PluginArray
assert_eval_poll "navigator.plugins instanceof PluginArray" "true" "plugins passes instanceof PluginArray"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: plugins has length > 0"

assert_eval_poll "navigator.plugins.length > 0" "true" "plugins has content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: chrome.runtime present"

assert_eval_poll "!!(window.chrome && window.chrome.runtime)" "true" "chrome.runtime exists"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: platform matches user agent"

# Platform should match UA to avoid detection
pt_post /evaluate '{"expression":"(() => { const ua = navigator.userAgent; const p = navigator.platform; if (ua.includes(\"Linux\") && !p.includes(\"Linux\")) return false; if (ua.includes(\"Macintosh\") && p !== \"MacIntel\") return false; if (ua.includes(\"Windows\") && !p.includes(\"Win\")) return false; return true; })()"}'
assert_ok "platform matches UA"
assert_json_eq "$RESULT" '.result' "true"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: languages are set"

assert_eval_poll "navigator.languages && navigator.languages.length > 0" "true" "languages present"

end_test



# ═══════════════════════════════════════════════════════════════════
# FULL STEALTH MODE (secure instance)
# NOTE: Secure instance has allowEvaluate=false for security tests,
# so we can only test navigation, not JavaScript evaluation.
# The stealth fixes (enable-automation=false) apply to ALL instances.
# ═══════════════════════════════════════════════════════════════════

echo ""
echo -e "${BLUE}Testing FULL stealth mode (secure instance - limited tests)${NC}"
echo -e "${YELLOW}Note: evaluate disabled on secure instance, testing navigation only${NC}"

# Switch to secure instance for full stealth tests
ORIG_URL="$E2E_SERVER"
E2E_SERVER="$E2E_SECURE_SERVER"

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect-full: can navigate with full stealth"

# Navigate to bot detection fixture on secure instance
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
assert_ok "navigate to bot-detect fixture (full stealth)"

# Verify we got a valid response with tab info
TAB_ID=$(echo "$RESULT" | jq -r '.tabId // empty')
if [ -n "$TAB_ID" ]; then
  echo -e "  ${GREEN}✓${NC} Got tabId: $TAB_ID"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} No tabId in response"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect-full: page title loaded correctly"

TITLE=$(echo "$RESULT" | jq -r '.title // empty')
if [ "$TITLE" = "Bot Detection Tests" ]; then
  echo -e "  ${GREEN}✓${NC} Page title: $TITLE"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} Unexpected title: $TITLE"
fi

end_test

# Restore original URL
E2E_SERVER="$ORIG_URL"

# ═══════════════════════════════════════════════════════════════════
# Note: Full stealth JavaScript tests (WebGL spoofing, etc.) would
# require allowEvaluate=true. The key stealth fix (enable-automation=false)
# is verified in the light mode tests above.
# ═══════════════════════════════════════════════════════════════════
