#!/bin/bash
# 45-bot-detection.sh — Bot detection stealth tests
# Tests the fixes for GitHub issue #275
# Simulates checks from bot.sannysoft.com

source "$(dirname "$0")/common.sh"

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
