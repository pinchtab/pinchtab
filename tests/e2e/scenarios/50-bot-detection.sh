#!/bin/bash
# 50-bot-detection.sh — Bot detection / stealth tests
#
# Tests pinchtab's stealth capabilities against common bot detection methods.
# Uses fixtures/bot-detect.html which runs comprehensive client-side checks.
#
# Based on research from ~/dev/research/stealth/:
# - navigator.webdriver detection
# - HeadlessChrome user agent
# - CDP traces (cdc_* variables)
# - Plugins array
# - chrome.runtime
# - Permissions API
# - WebGL renderer

source "$(dirname "$0")/common.sh"

# ═══════════════════════════════════════════════════════════════════════════
# BOT DETECTION: Core stealth checks
# ═══════════════════════════════════════════════════════════════════════════

start_test "bot-detect: navigate to test page"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
assert_ok "navigate"
sleep 1  # Allow stealth.js injection

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: navigator.webdriver = false/undefined"

# Critical: navigator.webdriver must NOT be true
assert_eval_poll "navigator.webdriver === true" "false" "webdriver !== true"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: webdriver property hidden"

# The property should either not exist or return undefined/false
assert_eval_poll "'webdriver' in navigator && navigator.webdriver === true" "false" "webdriver property not exposed as true"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: no HeadlessChrome in user agent"

pt_post /evaluate '{"expression":"navigator.userAgent.includes(\"HeadlessChrome\")"}'
assert_json_eq "$RESULT" '.result' 'false' "UA does not contain HeadlessChrome"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: no CDP traces (cdc_* variables)"

pt_post /evaluate '{"expression":"Object.keys(window).some(k => k.startsWith(\"cdc_\"))"}'
assert_json_eq "$RESULT" '.result' 'false' "no cdc_* variables"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: plugins array exists"

pt_post /evaluate '{"expression":"navigator.plugins instanceof PluginArray && navigator.plugins.length > 0"}'
assert_json_eq "$RESULT" '.result' 'true' "plugins array populated"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: chrome.runtime exists"

pt_post /evaluate '{"expression":"!!(window.chrome && window.chrome.runtime)"}'
assert_json_eq "$RESULT" '.result' 'true' "chrome.runtime present"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: permissions API functional"

pt_post /evaluate '{"expression":"navigator.permissions && typeof navigator.permissions.query === \"function\""}'
assert_json_eq "$RESULT" '.result' 'true' "permissions.query exists"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: languages defined"

pt_post /evaluate '{"expression":"Array.isArray(navigator.languages) && navigator.languages.length > 0"}'
assert_json_eq "$RESULT" '.result' 'true' "navigator.languages populated"

end_test

# ═══════════════════════════════════════════════════════════════════════════
# BOT DETECTION: Warning-level checks (nice to pass)
# ═══════════════════════════════════════════════════════════════════════════

start_test "bot-detect: WebGL not SwiftShader (headless indicator)"

# SwiftShader is the software renderer used in headless mode
pt_post /evaluate '{"expression":"(() => { try { const c = document.createElement(\"canvas\"); const gl = c.getContext(\"webgl\"); const d = gl.getExtension(\"WEBGL_debug_renderer_info\"); return gl.getParameter(d.UNMASKED_RENDERER_WEBGL).toLowerCase().includes(\"swiftshader\"); } catch(e) { return false; } })()"}'
assert_json_eq "$RESULT" '.result' 'false' "WebGL renderer not SwiftShader"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: outer window dimensions exist"

# Headless browsers may have 0 outer dimensions
pt_post /evaluate '{"expression":"window.outerWidth > 0 && window.outerHeight > 0"}'
assert_json_eq "$RESULT" '.result' 'true' "outerWidth/outerHeight > 0"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: hardware concurrency reasonable"

pt_post /evaluate '{"expression":"navigator.hardwareConcurrency >= 2"}'
assert_json_eq "$RESULT" '.result' 'true' "hardwareConcurrency >= 2"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect: device memory exists"

pt_post /evaluate '{"expression":"navigator.deviceMemory > 0"}'
assert_json_eq "$RESULT" '.result' 'true' "deviceMemory > 0"

end_test

# ═══════════════════════════════════════════════════════════════════════════
# BOT DETECTION: Full suite validation
# ═══════════════════════════════════════════════════════════════════════════

start_test "bot-detect: comprehensive score check"

# The bot-detect.html page computes a score object
pt_post /evaluate '{"expression":"JSON.stringify(window.__botDetectScore || {})"}'
local score_json
score_json=$(echo "$RESULT" | jq -r '.result // "{}"')

# Parse the score
local critical_passed critical_total passed
critical_passed=$(echo "$score_json" | jq -r '.critical // 0')
critical_total=$(echo "$score_json" | jq -r '.criticalTotal // 0')
passed=$(echo "$score_json" | jq -r '.passed // false')

echo -e "  ${MUTED}Score: critical ${critical_passed}/${critical_total}${NC}"

if [ "$passed" = "true" ]; then
  echo -e "  ${GREEN}✓${NC} all critical tests passed"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} some critical tests failed (${critical_passed}/${critical_total})"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ═══════════════════════════════════════════════════════════════════════════
print_summary
