#!/bin/bash
# stealth-basic.sh — API stealth baseline scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

# Migrated from: tests/integration/stealth_test.go

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "stealth: webdriver is undefined"

assert_eval_poll "navigator.webdriver === undefined" "true" "navigator.webdriver is undefined"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "stealth: plugins present"

assert_eval_poll "navigator.plugins.length > 0" "true" "navigator.plugins spoofed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "stealth: chrome.runtime present"

assert_eval_poll "!!window.chrome && !!window.chrome.runtime" "true" "window.chrome.runtime present"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "stealth: fingerprint rotate"

pt_post /fingerprint/rotate '{"os":"windows"}'
assert_ok "fingerprint rotate (windows)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "stealth: fingerprint rotate (random)"

pt_post /fingerprint/rotate '{}'
assert_ok "fingerprint rotate (random)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "stealth: fingerprint rotate (specific tab)"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
TAB_ID=$(get_tab_id)

pt_post /fingerprint/rotate "{\"tabId\":\"${TAB_ID}\",\"os\":\"mac\"}"
assert_ok "fingerprint rotate on tab"

end_test

# Tests the core stealth invariants from GitHub issue #275.
# This is the API fast-suite bot scenario.

start_test "bot-detect: navigate to test page"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/bot-detect.html\"}"
assert_ok "navigate to bot-detect fixture"
sleep 1

end_test

start_test "bot-detect: navigator.webdriver is not true"

assert_eval_poll "navigator.webdriver !== true" "true" "navigator.webdriver is not true"

end_test

start_test "bot-detect: navigator.webdriver is undefined"

assert_eval_poll "navigator.webdriver === undefined || navigator.webdriver === false" "true" "navigator.webdriver is undefined or false"

end_test

start_test "bot-detect: webdriver property does not exist"

assert_eval_poll "!('webdriver' in navigator)" "true" "webdriver property not in navigator"

end_test

start_test "bot-detect: iframe navigator matches parent"

pt_post /evaluate '{"expression":"(() => { const iframe = document.createElement(\"iframe\"); document.body.appendChild(iframe); try { const parent = navigator; const child = iframe.contentWindow.navigator; return child.webdriver === parent.webdriver && JSON.stringify(child.languages) === JSON.stringify(parent.languages) && child.platform === parent.platform; } finally { iframe.remove(); } })()"}'
assert_ok "iframe consistency evaluate"
assert_json_eq "$RESULT" '.result' 'true' "iframe navigator matches parent"

end_test

start_test "bot-detect: no CDP traces"

assert_eval_poll "!(window.cdc_adoQpoasnfa76pfcZLmcfl_Array || window.cdc_adoQpoasnfa76pfcZLmcfl_Promise)" "true" "no CDP automation traces"

end_test

start_test "bot-detect: UA not headless"

assert_eval_poll "!navigator.userAgent.includes('HeadlessChrome')" "true" "UA not headless"

end_test

start_test "bot-detect: plugins instanceof PluginArray"

assert_eval_poll "navigator.plugins instanceof PluginArray" "true" "plugins passes instanceof PluginArray"

end_test

start_test "bot-detect: plugins has length > 0"

assert_eval_poll "navigator.plugins.length > 0" "true" "plugins has content"

end_test

start_test "bot-detect: chrome.runtime present"

assert_eval_poll "!!(window.chrome && window.chrome.runtime)" "true" "chrome.runtime exists"

end_test

start_test "bot-detect: platform matches user agent"

pt_post /evaluate '{"expression":"(() => { const ua = navigator.userAgent; const p = navigator.platform; if (ua.includes(\"Linux\") && !p.includes(\"Linux\")) return false; if (ua.includes(\"Macintosh\") && p !== \"MacIntel\") return false; if (ua.includes(\"Windows\") && !p.includes(\"Win\")) return false; return true; })()"}'
assert_ok "platform matches UA"
assert_json_eq "$RESULT" '.result' "true"

end_test

start_test "bot-detect: permissions API functional"

assert_eval_poll "navigator.permissions && typeof navigator.permissions.query === 'function'" "true" "permissions.query exists"

end_test

start_test "bot-detect: languages are set"

assert_eval_poll "navigator.languages && navigator.languages.length > 0" "true" "languages present"

end_test

start_test "bot-detect: screen dimensions realistic"

assert_eval_poll "screen.width >= 1024 && screen.height >= 768" "true" "screen dimensions realistic"

end_test

start_test "bot-detect: screen colorDepth is 24"

assert_eval_poll "screen.colorDepth === 24" "true" "colorDepth is 24"

end_test

start_test "bot-detect: battery API exists"

assert_eval_poll "typeof navigator.getBattery === 'function'" "true" "getBattery exists"

end_test
