#!/bin/bash
# stealth-basic.sh — CLI stealth baseline scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/cli.sh"

# Tests pinchtab's stealth capabilities using the CLI interface.

# BOT DETECTION: Core stealth checks via CLI

start_test "bot-detect-cli: navigate to test page"

pt_ok nav "${FIXTURES_URL}/bot-detect.html"
sleep 1

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: navigator.webdriver check"

pt_ok eval "navigator.webdriver === true"
assert_json_field '.result' 'false' "webdriver !== true"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: no HeadlessChrome in user agent"

pt_ok eval "navigator.userAgent.includes('HeadlessChrome')"
assert_json_field '.result' 'false' "UA clean"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: plugins array present"

pt_ok eval "navigator.plugins.length > 0"
assert_json_field '.result' 'true' "plugins exist"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: chrome.runtime exists"

pt_ok eval "!!(window.chrome && window.chrome.runtime)"
assert_json_field '.result' 'true' "chrome.runtime"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "stealth-cli: capability fixture reports native webdriver contract"

pt_ok nav "${FIXTURES_URL}/stealth-capabilities.html"
sleep 1

pt_ok eval "window.__stealthCapabilities.webdriverDescriptorNativeLike"
assert_json_field '.result' 'true' "webdriver descriptor stays native-like"

pt_ok eval "window.__stealthCapabilities.userAgentVersionCoherent"
assert_json_field '.result' 'true' "user agent version remains coherent"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "stealth-cli: created tab keeps stealth capability contract"

pt_ok tab new
assert_output_json "tab new returns JSON"
TAB_ID=$(echo "$PT_OUT" | jq -r '.tabId // empty')

if [ -n "$TAB_ID" ]; then
  echo -e "  ${GREEN}✓${NC} created tab returned id"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} created tab did not return id"
  ((ASSERTIONS_FAILED++)) || true
fi

pt_ok nav "${FIXTURES_URL}/stealth-capabilities.html" --tab "$TAB_ID"
sleep 1

pt_ok eval "window.__stealthCapabilities.webdriverDescriptorNativeLike" --tab "$TAB_ID"
assert_json_field '.result' 'true' "created tab keeps native webdriver descriptor"

pt_ok eval "window.__stealthCapabilities.intlLocaleCoherent" --tab "$TAB_ID"
assert_json_field '.result' 'true' "created tab keeps locale coherence"

pt_ok tab close "$TAB_ID"

end_test

# ─────────────────────────────────────────────────────────────────────────────
start_test "bot-detect-cli: full test suite score"

pt_ok eval "JSON.stringify(window.__botDetectScore || {})"
score=$(echo "$PT_OUT" | jq -r '.result // "{}"')

pt_ok eval "window.__pinchtab_stealth_level || 'light'"
stealth_level=$(echo "$PT_OUT" | jq -r '.result // "light"')

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
  print_summary
fi
