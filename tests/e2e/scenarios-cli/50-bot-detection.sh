#!/bin/bash
# 50-bot-detection.sh — Bot detection / stealth tests (CLI)
#
# Tests pinchtab's stealth capabilities using the CLI interface.

source "$(dirname "$0")/common.sh"

# ═══════════════════════════════════════════════════════════════════════════
# BOT DETECTION: Core stealth checks via CLI
# ═══════════════════════════════════════════════════════════════════════════

start_test "bot-detect-cli: navigate to test page"

pt nav "${FIXTURES_URL}/bot-detect.html"
assert_ok "navigate"
sleep 1

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: navigator.webdriver check"

pt eval "navigator.webdriver === true"
assert_json_field '.result' 'false' "webdriver !== true"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: no HeadlessChrome in user agent"

pt eval "navigator.userAgent.includes('HeadlessChrome')"
assert_json_field '.result' 'false' "UA clean"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: plugins array present"

pt eval "navigator.plugins.length > 0"
assert_json_field '.result' 'true' "plugins exist"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: chrome.runtime exists"

pt eval "!!(window.chrome && window.chrome.runtime)"
assert_json_field '.result' 'true' "chrome.runtime"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: full test suite score"

pt eval "JSON.stringify(window.__botDetectScore || {})"
local score
score=$(echo "$PT_OUT" | jq -r '.result // "{}"')

local passed
passed=$(echo "$score" | jq -r '.passed // false')

if [ "$passed" = "true" ]; then
  echo -e "  ${GREEN}✓${NC} all critical tests passed"
  ((ASSERTIONS_PASSED++)) || true
else
  local crit=$(echo "$score" | jq -r '.critical // 0')
  local total=$(echo "$score" | jq -r '.criticalTotal // 0')
  echo -e "  ${RED}✗${NC} some critical tests failed (${crit}/${total})"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ═══════════════════════════════════════════════════════════════════════════
print_summary
