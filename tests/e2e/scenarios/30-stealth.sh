#!/bin/bash
# 30-stealth.sh — Stealth and fingerprint tests
# Migrated from: tests/integration/stealth_test.go

source "$(dirname "$0")/common.sh"

# Navigate first
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


