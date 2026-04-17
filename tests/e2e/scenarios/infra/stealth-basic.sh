#!/bin/bash
# stealth-basic.sh — API stealth baseline scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# Migrated from: tests/integration/stealth_test.go

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "stealth: status endpoint available"

pt_get /stealth/status
assert_json_eq "$RESULT" '.level' 'light' "default level is light"
assert_json_exists "$RESULT" '.scriptHash' "status includes script hash"
assert_json_eq "$RESULT" '.capabilities.webdriverNotTrue' 'true' "status reports webdriver-not-true capability"
assert_json_eq "$RESULT" '.capabilities.webdriverNativeStrategy' 'true' "status reports native webdriver strategy"
assert_json_eq "$RESULT" '.capabilities.downlinkMax' 'true' "status reports downlinkMax capability"
assert_json_eq "$RESULT" '.flags.globalUserAgent' 'true' "status reports global launch UA"
assert_json_eq "$RESULT" '.flags.downlinkMaxFlag' 'true' "status reports downlinkMax launch flag"
assert_json_eq "$RESULT" '.capabilities.iframeIsolation' 'false' "light status keeps iframe isolation disabled"
assert_json_eq "$RESULT" '.capabilities.errorStackSanitized' 'false' "light status keeps stack sanitization disabled"
assert_json_eq "$RESULT" '.capabilities.functionToStringMasked' 'false' "light status keeps toString masking disabled"
assert_json_eq "$RESULT" '.capabilities.functionToStringNative' 'true' "light status keeps Function.prototype.toString native"
assert_json_eq "$RESULT" '.capabilities.intlLocaleCoherent' 'true' "light status keeps Intl locale coherence"
assert_json_eq "$RESULT" '.capabilities.errorPrepareStackTraceNative' 'true' "light status keeps Error.prepareStackTrace native"
assert_json_eq "$RESULT" '.capabilities.navigatorOverridesAbsent' 'true' "light status avoids navigator own-property overrides"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "stealth: webdriver is not true"

assert_eval_poll "navigator.webdriver !== true" "true" "navigator.webdriver is not true"

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

pt_post "/tabs/${TAB_ID}/evaluate" '{"expression":"JSON.stringify({userAgent:navigator.userAgent,platform:navigator.platform,language:navigator.language,viewportWidth:window.innerWidth,viewportHeight:window.innerHeight})"}'
assert_ok "capture pre-rotate tab fingerprint"
PRE_ROTATE_SURFACE=$(echo "$RESULT" | jq -r '.result // "{}"')
PRE_ROTATE_UA=$(echo "$PRE_ROTATE_SURFACE" | jq -r '.userAgent // ""')
PRE_ROTATE_PLATFORM=$(echo "$PRE_ROTATE_SURFACE" | jq -r '.platform // ""')
PRE_ROTATE_LANGUAGE=$(echo "$PRE_ROTATE_SURFACE" | jq -r '.language // ""')

pt_post /fingerprint/rotate "{\"tabId\":\"${TAB_ID}\",\"os\":\"windows\",\"screen\":\"1366x768\",\"language\":\"en-US\"}"
assert_ok "fingerprint rotate on tab"
ROTATE_FINGERPRINT_JSON=$(echo "$RESULT" | jq -c '.fingerprint // {}')
ROTATE_USER_AGENT=$(echo "$ROTATE_FINGERPRINT_JSON" | jq -r '.userAgent // empty')
ROTATE_PLATFORM=$(echo "$ROTATE_FINGERPRINT_JSON" | jq -r '.platform // empty')
ROTATE_WIDTH=$(echo "$ROTATE_FINGERPRINT_JSON" | jq -r '.screenWidth // 0')
ROTATE_HEIGHT=$(echo "$ROTATE_FINGERPRINT_JSON" | jq -r '.screenHeight // 0')
ROTATE_LANGUAGE=$(echo "$ROTATE_FINGERPRINT_JSON" | jq -r '.language // empty')

pt_get "/stealth/status?tabId=${TAB_ID}"
assert_json_eq "$RESULT" '.tabOverrides.fingerprintRotateActive' 'true' "status reports fingerprint rotate overlay on tab"

if [ -n "$ROTATE_USER_AGENT" ] && { [ "$PRE_ROTATE_UA" != "$ROTATE_USER_AGENT" ] || [ "$PRE_ROTATE_PLATFORM" != "$ROTATE_PLATFORM" ] || [ "$PRE_ROTATE_LANGUAGE" != "$ROTATE_LANGUAGE" ]; }; then
  echo -e "  ${GREEN}✓${NC} fingerprint rotate changes tab persona"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} fingerprint rotate did not change tab persona"
  echo -e "    ${MUTED}before:${NC} ua=${PRE_ROTATE_UA} platform=${PRE_ROTATE_PLATFORM} language=${PRE_ROTATE_LANGUAGE}"
  echo -e "    ${MUTED}after:${NC}  ua=${ROTATE_USER_AGENT} platform=${ROTATE_PLATFORM} language=${ROTATE_LANGUAGE}"
  ((ASSERTIONS_FAILED++)) || true
fi

assert_eval_poll "navigator.userAgent === $(jq -Rn --arg v "$ROTATE_USER_AGENT" '$v')" "true" "rotated tab userAgent matches fingerprint" 8 0.3 "$TAB_ID"
assert_eval_poll "navigator.platform === $(jq -Rn --arg v "$ROTATE_PLATFORM" '$v')" "true" "rotated tab platform matches fingerprint" 8 0.3 "$TAB_ID"
assert_eval_poll "navigator.language === $(jq -Rn --arg v "$ROTATE_LANGUAGE" '$v')" "true" "rotated tab language matches fingerprint" 8 0.3 "$TAB_ID"
assert_eval_poll "window.innerWidth === ${ROTATE_WIDTH}" "true" "rotated tab viewport width matches fingerprint" 8 0.3 "$TAB_ID"
assert_eval_poll "window.innerHeight === ${ROTATE_HEIGHT}" "true" "rotated tab viewport height matches fingerprint" 8 0.3 "$TAB_ID"

pt_post "/tabs/${TAB_ID}/navigate" "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate rotated tab"
sleep 1

assert_eval_poll "navigator.userAgent === $(jq -Rn --arg v "$ROTATE_USER_AGENT" '$v')" "true" "rotated userAgent persists across navigation" 8 0.3 "$TAB_ID"
assert_eval_poll "navigator.platform === $(jq -Rn --arg v "$ROTATE_PLATFORM" '$v')" "true" "rotated platform persists across navigation" 8 0.3 "$TAB_ID"
assert_eval_poll "navigator.language === $(jq -Rn --arg v "$ROTATE_LANGUAGE" '$v')" "true" "rotated language persists across navigation" 8 0.3 "$TAB_ID"
assert_eval_poll "window.innerWidth === ${ROTATE_WIDTH}" "true" "rotated viewport width persists across navigation" 8 0.3 "$TAB_ID"
assert_eval_poll "window.innerHeight === ${ROTATE_HEIGHT}" "true" "rotated viewport height persists across navigation" 8 0.3 "$TAB_ID"

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

start_test "bot-detect: navigator.webdriver is not true"

assert_eval_poll "navigator.webdriver !== true" "true" "navigator.webdriver is undefined or false"

end_test

start_test "bot-detect: webdriver property semantics are acceptable"

assert_eval_poll "!('webdriver' in navigator) || navigator.webdriver === false" "true" "webdriver property is hidden or native false"

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

start_test "stealth-capabilities: navigate to capability fixture"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/stealth-capabilities.html\"}"
assert_ok "navigate to stealth-capabilities fixture"
sleep 1

end_test

start_test "stealth-capabilities: plugin object semantics"

assert_eval_poll "window.__stealthCapabilities.pluginObjectSemantics" "true" "plugin object semantics are coherent"

end_test

start_test "stealth-capabilities: outer dimensions available"

assert_eval_poll "window.__stealthCapabilities.outerDimensions" "true" "outer dimensions exist"

end_test

start_test "stealth-capabilities: screen and window coherent"

assert_eval_poll "window.__stealthCapabilities.screenWindowCoherent" "true" "screen and outer window are coherent"
assert_eval_poll "window.__stealthCapabilities.screenAvailCoherent" "true" "available screen stays within screen bounds"

end_test

start_test "stealth-capabilities: devicePixelRatio positive"

assert_eval_poll "window.__stealthCapabilities.devicePixelRatioPositive" "true" "devicePixelRatio stays positive"

end_test

start_test "stealth-capabilities: battery API reported"

assert_eval_poll "window.__stealthCapabilities.batteryApi" "true" "battery API reported by capability fixture"

end_test

start_test "stealth-capabilities: downlinkMax reported"

assert_eval_poll "window.__stealthCapabilities.downlinkMaxPresent" "true" "downlinkMax reported by capability fixture"

end_test

start_test "stealth-capabilities: medium-only iframe isolation absent at light"

assert_eval_poll "window.__stealthCapabilities.iframeIsolation" "false" "iframe isolation remains disabled at light"

end_test

start_test "stealth-capabilities: native Function.prototype.toString preserved"

assert_eval_poll "window.__stealthCapabilities.functionToStringNative" "true" "Function.prototype.toString remains native-like"
assert_eval_poll "window.__stealthCapabilities.canPlayTypeNativeLike" "true" "canPlayType remains native-like"
assert_eval_poll "window.__stealthCapabilities.getComputedStyleNameNative" "true" "getComputedStyle keeps its native name"
assert_eval_poll "window.__stealthCapabilities.matchMediaNameNative" "true" "matchMedia keeps its native name"

end_test

start_test "stealth-capabilities: Intl locale stays coherent"

assert_eval_poll "window.__stealthCapabilities.timeZoneDateCoherent" "true" "resolved timezone matches local Date clock"
assert_eval_poll "window.__stealthCapabilities.timeZoneOffsetSane" "true" "timezone offset stays in a sane range"
assert_eval_poll "window.__stealthCapabilities.intlLocaleCoherent" "true" "Intl locale family stays coherent"
assert_eval_poll "window.__stealthCapabilities.intlDateTimeLocaleCoherent" "true" "Intl.DateTimeFormat locale matches navigator.language"
assert_eval_poll "window.__stealthCapabilities.intlNumberLocaleCoherent" "true" "Intl.NumberFormat locale matches navigator.language"

end_test

start_test "stealth-capabilities: Error.prepareStackTrace remains native"

assert_eval_poll "window.__stealthCapabilities.errorPrepareStackTraceNative" "true" "Error.prepareStackTrace is not trapped"

end_test

start_test "stealth-capabilities: navigator own-property overrides absent"

assert_eval_poll "window.__stealthCapabilities.navigatorOverridesAbsent" "true" "navigator keeps core properties off the instance"

end_test

start_test "stealth-capabilities: userAgentData versions coherent"

assert_eval_poll "window.__stealthCapabilities.userAgentVersionCoherent" "true" "appVersion aligns with the UA version"
assert_eval_poll "window.__stealthCapabilities.userAgentDataBrandMajorCoherent" "true" "userAgentData brand major aligns with UA major"
assert_eval_poll "window.__stealthCapabilities.userAgentDataVersionCoherent" "true" "userAgentData versions align with UA major"
assert_eval_poll "window.__stealthCapabilities.userAgentDataFullVersionCoherent" "true" "userAgentData full version aligns with the UA version"

end_test

start_test "stealth-capabilities: canvas and WebGL probes stay low-noise"

assert_eval_poll "window.__stealthCapabilities.canvasToDataURLNativeLike" "true" "canvas toDataURL remains native-like"
assert_eval_poll "window.__stealthCapabilities.canvasGetImageDataNativeLike" "true" "canvas getImageData remains native-like"
assert_eval_poll "window.__stealthCapabilities.canvasTransparentPixelPreserved" "true" "transparent canvas pixels stay untouched"
assert_eval_poll "window.__stealthCapabilities.webglGetParameterNativeLike" "true" "WebGL getParameter remains native-like"

end_test

start_test "stealth-capabilities: webdriver descriptor remains native-like"

assert_eval_poll "window.__stealthCapabilities.webdriverDescriptorNativeLike" "true" "webdriver descriptor getter stays native-like"

end_test

start_test "stealth-devtools-probe: navigate to local probe fixture"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/stealth-devtools-probe.html\"}"
assert_ok "navigate to devtools probe fixture"
sleep 2

end_test

start_test "stealth-devtools-probe: report ready"

assert_eval_poll "!!window.__stealthDevtoolsProbe && window.__stealthDevtoolsProbe.ready" "true" "devtools probe report ready"

end_test

start_test "stealth-devtools-probe: formatter probe not triggered"

assert_eval_poll "window.__stealthDevtoolsProbe.formatterCalled" "false" "console formatter probe stays untouched"
assert_eval_poll "window.__stealthDevtoolsProbe.initialFormattersLen" "0" "page starts without preinstalled devtools formatters"

end_test

start_test "stealth-devtools-probe: getter probe not triggered"

assert_eval_poll "window.__stealthDevtoolsProbe.getterCalled" "false" "console getter probe stays untouched"
assert_eval_poll "window.__stealthDevtoolsProbe.stackGetterCalls <= 1" "true" "error stack getter stays within baseline console access"

end_test

start_test "stealth-workers: navigate to worker parity fixture"

pt_post /navigate "{\"url\":\"${FIXTURES_URL}/stealth-workers.html\"}"
assert_ok "navigate to stealth-workers fixture"
sleep 1

end_test

start_test "stealth-workers: worker report ready"

assert_eval_poll "!!window.__stealthWorkerReport && window.__stealthWorkerReport.ready" "true" "worker report ready"

end_test

start_test "stealth-workers: user agent matches worker"

assert_eval_poll "window.__stealthWorkerReport.matches.userAgent" "true" "page and worker userAgent match"

end_test

start_test "stealth-workers: platform matches worker"

assert_eval_poll "window.__stealthWorkerReport.matches.platform" "true" "page and worker platform match"

end_test

start_test "stealth-workers: hardware concurrency matches worker"

assert_eval_poll "window.__stealthWorkerReport.matches.hardwareConcurrency" "true" "page and worker hardwareConcurrency match"

end_test

start_test "stealth-workers: device memory matches worker"

assert_eval_poll "window.__stealthWorkerReport.matches.deviceMemory" "true" "page and worker deviceMemory match"

end_test
