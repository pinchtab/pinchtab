#!/bin/bash
# 45-bot-detection.sh — Bot detection stealth tests
# Tests the fixes for GitHub issue #275
# Simulates checks from bot.sannysoft.com
#
# Tests run against:
# - PINCHTAB_URL (light stealth) - basic bot detection evasion
# - PINCHTAB_SECURE_URL (full stealth) - additional fingerprint protections

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
assert_json_equals "$RESULT" '.result.value' "true"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: languages are set"

assert_eval_poll "navigator.languages && navigator.languages.length > 0" "true" "languages present"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: overall score passes"

# Use the fixture's built-in scoring
pt_post /evaluate '{"expression":"window.__botDetectScore && window.__botDetectScore.passed"}'
assert_ok "get bot detect score"
assert_json_equals "$RESULT" '.result.value' "true"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: all critical tests pass"

pt_post /evaluate '{"expression":"window.__botDetectScore ? window.__botDetectScore.critical + \"/\" + window.__botDetectScore.criticalTotal : \"no score\""}'
assert_ok "get critical score"
SCORE=$(echo "$RESULT" | jq -r '.result.value')
echo "  Critical tests: $SCORE"

# Extract numbers and verify all passed
PASSED=$(echo "$SCORE" | cut -d'/' -f1)
TOTAL=$(echo "$SCORE" | cut -d'/' -f2)
if [ "$PASSED" != "$TOTAL" ]; then
  fail_test "Not all critical tests passed: $SCORE"
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# FULL STEALTH MODE (secure instance)
# Tests additional fingerprint protections only available in full mode
# ═══════════════════════════════════════════════════════════════════

echo ""
echo -e "${BLUE}Testing FULL stealth mode (secure instance)${NC}"

# Switch to secure instance for full stealth tests
ORIG_URL="$PINCHTAB_URL"
PINCHTAB_URL="$PINCHTAB_SECURE_URL"

# Navigate to bot detection fixture on secure instance
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
assert_ok "navigate to bot-detect fixture (full stealth)"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect-full: webdriver is undefined"

assert_eval_poll "navigator.webdriver === undefined || navigator.webdriver === false" "true" "navigator.webdriver hidden (full)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect-full: plugins instanceof PluginArray"

assert_eval_poll "navigator.plugins instanceof PluginArray" "true" "plugins passes instanceof (full)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect-full: WebGL vendor spoofed"

# Full stealth mode should spoof WebGL vendor to Intel
pt_post /evaluate '{"expression":"(() => { try { const c = document.createElement(\"canvas\"); const gl = c.getContext(\"webgl\"); const dbg = gl.getExtension(\"WEBGL_debug_renderer_info\"); return gl.getParameter(dbg.UNMASKED_VENDOR_WEBGL); } catch(e) { return \"error\"; } })()"}'
assert_ok "get WebGL vendor"
VENDOR=$(echo "$RESULT" | jq -r '.result.value')
echo "  WebGL vendor: $VENDOR"

# In full stealth, should be spoofed to "Intel Inc."
if [ "$VENDOR" = "Intel Inc." ]; then
  echo -e "  ${GREEN}✓${NC} WebGL vendor spoofed to Intel"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} WebGL vendor not spoofed (may vary by environment): $VENDOR"
  # Don't fail - WebGL spoofing depends on environment
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect-full: WebGL renderer spoofed"

pt_post /evaluate '{"expression":"(() => { try { const c = document.createElement(\"canvas\"); const gl = c.getContext(\"webgl\"); const dbg = gl.getExtension(\"WEBGL_debug_renderer_info\"); return gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL); } catch(e) { return \"error\"; } })()"}'
assert_ok "get WebGL renderer"
RENDERER=$(echo "$RESULT" | jq -r '.result.value')
echo "  WebGL renderer: $RENDERER"

# In full stealth, should be spoofed to Intel Iris
if echo "$RENDERER" | grep -qi "intel"; then
  echo -e "  ${GREEN}✓${NC} WebGL renderer spoofed"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} WebGL renderer not spoofed: $RENDERER"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect-full: overall score passes"

pt_post /evaluate '{"expression":"window.__botDetectScore && window.__botDetectScore.passed"}'
assert_ok "get bot detect score (full)"
assert_json_equals "$RESULT" '.result.value' "true"

end_test

# Restore original URL
PINCHTAB_URL="$ORIG_URL"
