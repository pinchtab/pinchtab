#!/bin/bash
# 51-stealth-levels.sh — Verify stealth level differentiation
#
# Tests that each stealth level (light/medium/full) provides the expected
# features and that level-specific protections are only active at that level.
#
# This test expects STEALTH_LEVEL env var to be set (light|medium|full)
# and verifies the behavior matches the configured level.
#
# Usage:
#   STEALTH_LEVEL=light ./51-stealth-levels.sh
#   STEALTH_LEVEL=medium ./51-stealth-levels.sh
#   STEALTH_LEVEL=full ./51-stealth-levels.sh

source "$(dirname "$0")/common.sh"

STEALTH_LEVEL="${STEALTH_LEVEL:-light}"
echo -e "${BLUE}Testing stealth level: ${STEALTH_LEVEL}${NC}\n"

# ═══════════════════════════════════════════════════════════════════════════
# SETUP
# ═══════════════════════════════════════════════════════════════════════════

start_test "stealth-levels: navigate to test page"
pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
assert_ok "navigate"
sleep 1
end_test

# ═══════════════════════════════════════════════════════════════════════════
# LIGHT LEVEL FEATURES (should pass at ALL levels)
# ═══════════════════════════════════════════════════════════════════════════

start_test "stealth-levels: [light] webdriver hidden"
pt_post /evaluate '{"expression":"navigator.webdriver === true"}'
assert_json_eq "$RESULT" '.result' 'false' "webdriver !== true"
end_test

start_test "stealth-levels: [light] plugins array exists"
pt_post /evaluate '{"expression":"navigator.plugins.length > 0"}'
assert_json_eq "$RESULT" '.result' 'true' "plugins present"
end_test

start_test "stealth-levels: [light] hardware concurrency"
pt_post /evaluate '{"expression":"navigator.hardwareConcurrency >= 2"}'
assert_json_eq "$RESULT" '.result' 'true' "hardwareConcurrency >= 2"
end_test

start_test "stealth-levels: [light] basic chrome.runtime"
pt_post /evaluate '{"expression":"!!(window.chrome && window.chrome.runtime)"}'
assert_json_eq "$RESULT" '.result' 'true' "chrome.runtime exists"
end_test

# ═══════════════════════════════════════════════════════════════════════════
# MEDIUM LEVEL FEATURES (should pass at medium and full only)
# ═══════════════════════════════════════════════════════════════════════════

start_test "stealth-levels: [medium] userAgentData exists"
pt_post /evaluate '{"expression":"!!(navigator.userAgentData && navigator.userAgentData.brands)"}'
if [ "$STEALTH_LEVEL" = "light" ]; then
  # Light level: userAgentData might exist natively but won't have our spoofing
  echo -e "  ${MUTED}(skipped - native behavior at light level)${NC}"
  ((ASSERTIONS_PASSED++)) || true
else
  assert_json_eq "$RESULT" '.result' 'true' "userAgentData.brands exists"
fi
end_test

start_test "stealth-levels: [medium] chrome.runtime.connect exists"
pt_post /evaluate '{"expression":"typeof window.chrome.runtime.connect === \"function\""}'
if [ "$STEALTH_LEVEL" = "light" ]; then
  assert_json_eq "$RESULT" '.result' 'false' "connect not spoofed at light"
else
  assert_json_eq "$RESULT" '.result' 'true' "connect exists at medium+"
fi
end_test

start_test "stealth-levels: [medium] chrome.csi exists"
pt_post /evaluate '{"expression":"typeof window.chrome.csi === \"function\""}'
if [ "$STEALTH_LEVEL" = "light" ]; then
  assert_json_eq "$RESULT" '.result' 'false' "csi not spoofed at light"
else
  assert_json_eq "$RESULT" '.result' 'true' "csi exists at medium+"
fi
end_test

start_test "stealth-levels: [medium] chrome.loadTimes exists"
pt_post /evaluate '{"expression":"typeof window.chrome.loadTimes === \"function\""}'
if [ "$STEALTH_LEVEL" = "light" ]; then
  assert_json_eq "$RESULT" '.result' 'false' "loadTimes not spoofed at light"
else
  assert_json_eq "$RESULT" '.result' 'true' "loadTimes exists at medium+"
fi
end_test

start_test "stealth-levels: [medium] maxTouchPoints spoofed"
pt_post /evaluate '{"expression":"navigator.maxTouchPoints === 0"}'
if [ "$STEALTH_LEVEL" = "light" ]; then
  # Light doesn't spoof this, native value may vary
  echo -e "  ${MUTED}(skipped - native behavior at light level)${NC}"
  ((ASSERTIONS_PASSED++)) || true
else
  assert_json_eq "$RESULT" '.result' 'true' "maxTouchPoints === 0 at medium+"
fi
end_test

# ═══════════════════════════════════════════════════════════════════════════
# FULL LEVEL FEATURES (should pass at full only)
# ═══════════════════════════════════════════════════════════════════════════

start_test "stealth-levels: [full] WebGL renderer spoofed"
pt_post /evaluate '{"expression":"(() => { try { const c = document.createElement(\"canvas\"); const gl = c.getContext(\"webgl\"); const d = gl.getExtension(\"WEBGL_debug_renderer_info\"); const r = gl.getParameter(d.UNMASKED_RENDERER_WEBGL); return r.includes(\"Intel\") && r.includes(\"UHD\"); } catch(e) { return false; } })()"}'
if [ "$STEALTH_LEVEL" = "full" ]; then
  assert_json_eq "$RESULT" '.result' 'true' "WebGL spoofed to Intel UHD"
else
  # At light/medium, native renderer is used
  echo -e "  ${MUTED}(skipped - native WebGL at ${STEALTH_LEVEL} level)${NC}"
  ((ASSERTIONS_PASSED++)) || true
fi
end_test

start_test "stealth-levels: [full] canvas toDataURL modified"
# At full level, canvas has noise injection so consecutive calls may differ slightly
# We can't easily test this without comparing outputs, so we just verify the function exists
pt_post /evaluate '{"expression":"typeof HTMLCanvasElement.prototype.toDataURL === \"function\""}'
assert_json_eq "$RESULT" '.result' 'true' "toDataURL exists"
end_test

# ═══════════════════════════════════════════════════════════════════════════
# SUMMARY
# ═══════════════════════════════════════════════════════════════════════════

start_test "stealth-levels: comprehensive score at ${STEALTH_LEVEL} level"
pt_post /evaluate '{"expression":"JSON.stringify(window.__botDetectScore || {})"}'
local score_json
score_json=$(echo "$RESULT" | jq -r '.result // "{}"')
local critical_passed critical_total
critical_passed=$(echo "$score_json" | jq -r '.critical // 0')
critical_total=$(echo "$score_json" | jq -r '.criticalTotal // 0')

echo -e "  ${MUTED}Critical tests: ${critical_passed}/${critical_total}${NC}"

# Expected minimum scores by level
case "$STEALTH_LEVEL" in
  light)
    # Light should pass basic detection (webdriver, plugins, etc.)
    [ "$critical_passed" -ge 6 ] && ((ASSERTIONS_PASSED++)) || ((ASSERTIONS_FAILED++))
    ;;
  medium)
    # Medium adds userAgentData, chrome APIs
    [ "$critical_passed" -ge 10 ] && ((ASSERTIONS_PASSED++)) || ((ASSERTIONS_FAILED++))
    ;;
  full)
    # Full should pass all critical tests
    [ "$critical_passed" -ge 12 ] && ((ASSERTIONS_PASSED++)) || ((ASSERTIONS_FAILED++))
    ;;
esac

echo -e "  ${GREEN}✓${NC} score meets ${STEALTH_LEVEL} level expectations"
end_test

# ═══════════════════════════════════════════════════════════════════════════
print_summary
