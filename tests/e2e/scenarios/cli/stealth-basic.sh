#!/bin/bash
# stealth-basic.sh — CLI stealth baseline scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# Tests pinchtab's stealth capabilities using the CLI interface.

# BOT DETECTION: Core stealth checks via CLI

start_test "bot-detect-cli: navigate to test page"

pt_ok nav "${FIXTURES_URL}/bot-detect.html"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: navigator.webdriver check"

pt_ok eval "navigator.webdriver === true"
assert_output_contains "false" "webdriver !== true"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: no HeadlessChrome in user agent"

pt_ok eval "navigator.userAgent.includes('HeadlessChrome')"
assert_output_contains "false" "UA clean"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: plugins array present"

pt_ok eval "navigator.plugins.length > 0"
assert_output_contains "true" "plugins exist"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: chrome.runtime exists"

pt_ok eval "!!(window.chrome && window.chrome.runtime)"
if [ "${PINCHTAB_E2E_BROWSER:-chrome}" = "cloak" ]; then
  assert_output_contains "false" "chrome.runtime stripped by cloak native stealth"
else
  assert_output_contains "true" "chrome.runtime"
fi

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "stealth-cli: capability fixture reports native webdriver contract"

pt_ok nav "${FIXTURES_URL}/stealth-capabilities.html"

pt_ok eval "window.__stealthCapabilities.webdriverDescriptorNativeLike"
assert_output_contains "true" "webdriver descriptor stays native-like"

pt_ok eval "window.__stealthCapabilities.userAgentVersionCoherent"
assert_output_contains "true" "user agent version remains coherent"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "stealth-cli: new navigation keeps stealth capability contract"

pt_ok nav "${FIXTURES_URL}/bot-detect.html" --new-tab --json
assert_output_json "nav --new-tab returns JSON"
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId // empty')

if [ -n "$TAB_ID" ]; then
  echo -e "  ${GREEN}✓${NC} created tab returned id"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} created tab did not return id"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_ok nav "${FIXTURES_URL}/stealth-capabilities.html" --tab "$TAB_ID"

pt_ok eval "window.__stealthCapabilities.webdriverDescriptorNativeLike" --tab "$TAB_ID"
assert_output_contains "true" "created tab keeps native webdriver descriptor"

pt_ok eval "window.__stealthCapabilities.intlLocaleCoherent" --tab "$TAB_ID"
assert_output_contains "true" "created tab keeps locale coherence"

pt_ok tab close "$TAB_ID"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: full test suite score"

# Navigate to bot-detect page in a new tab (previous test closed its tab)
pt_ok nav --new-tab "${FIXTURES_URL}/bot-detect.html"

pt_ok eval "JSON.stringify(window.__botDetectScore || {})"
score="$PT_OUT"

pt_ok eval "window.__pinchtab_stealth_level || 'light'"
stealth_level="$PT_OUT"

crit=$(echo "$score" | jq -r '.critical // 0')
total=$(echo "$score" | jq -r '.criticalTotal // 0')

case "$stealth_level" in
  medium|full)
    min_crit="$total"
    ;;
  *)
    min_crit=$((total - 3))
    ;;
esac

if [ "$crit" -ge "$min_crit" ]; then
  echo -e "  ${GREEN}✓${NC} score meets ${stealth_level} expectations"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} score below ${stealth_level} expectations (${crit}/${total}, need ${min_crit})"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  finish_suite
fi
