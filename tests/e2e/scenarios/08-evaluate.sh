#!/bin/bash
# 08-evaluate.sh — JavaScript evaluation

source "$(dirname "$0")/common.sh"

# Navigate to evaluate test page
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/evaluate.html\"}"
sleep 1

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (simple expression)"

pt_post /evaluate -d '{"expression":"1 + 1"}'
assert_ok "evaluate simple"
assert_result_eq ".result" "2" "1+1=2"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (DOM query)"

pt_post /evaluate -d '{"expression":"document.title"}'
assert_ok "evaluate DOM"
assert_result_eq ".result" "Evaluate Test Page" "got title"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (call function)"

pt_post /evaluate -d '{"expression":"window.calculate.add(5, 3)"}'
assert_ok "evaluate function"
assert_result_eq ".result" "8" "5+3=8"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (get object)"

pt_post /evaluate -d '{"expression":"JSON.stringify(window.testData)"}'
assert_ok "evaluate object"
assert_json_contains "$RESULT" ".result" "PinchTab" "has testData"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (modify DOM)"

pt_post /evaluate -d '{"expression":"document.getElementById(\"counter\").textContent = \"42\"; 42"}'
assert_ok "evaluate modify DOM"

# Verify the change stuck
pt_post /evaluate -d '{"expression":"document.getElementById(\"counter\").textContent"}'
assert_result_eq ".result" "42" "counter=42"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate --tab <id>"

# Open evaluate page in new tab - capture tabId from response
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/evaluate.html\",\"newTab\":true}"
assert_ok "navigate for evaluate"
TAB_ID=$(echo "$RESULT" | jq -r '.tabId')
sleep 1

pt_post "/tabs/${TAB_ID}/evaluate" -d '{"expression":"1 + 2 + 3"}'
assert_ok "tab evaluate"
assert_result_eq ".result" "6" "1+2+3=6"

end_test
