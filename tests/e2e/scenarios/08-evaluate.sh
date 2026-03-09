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

# Verify result
VAL=$(echo "$RESULT" | jq -r '.result')
if [ "$VAL" != "2" ]; then
  fail "expected result=2, got $VAL"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (DOM query)"

pt_post /evaluate -d '{"expression":"document.title"}'
assert_ok "evaluate DOM"

VAL=$(echo "$RESULT" | jq -r '.result')
if [ "$VAL" != "Evaluate Test Page" ]; then
  fail "expected value, got $VAL"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (call function)"

pt_post /evaluate -d '{"expression":"window.calculate.add(5, 3)"}'
assert_ok "evaluate function"

VAL=$(echo "$RESULT" | jq -r '.result')
if [ "$VAL" != "8" ]; then
  fail "expected value, got $VAL"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (get object)"

pt_post /evaluate -d '{"expression":"JSON.stringify(window.testData)"}'
assert_ok "evaluate object"

# Verify we got the test data
if ! echo "$RESULT" | jq -r '.result' | jq -e '.name == "PinchTab"' >/dev/null 2>&1; then
  fail "expected testData object"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab evaluate (modify DOM)"

pt_post /evaluate -d '{"expression":"document.getElementById(\"counter\").textContent = \"42\"; 42"}'
assert_ok "evaluate modify DOM"

# Verify the change stuck
pt_post /evaluate -d '{"expression":"document.getElementById(\"counter\").textContent"}'
VAL=$(echo "$RESULT" | jq -r '.result')
if [ "$VAL" != "42" ]; then
  fail "expected value, got $VAL"
fi

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

VAL=$(echo "$RESULT" | jq -r '.result')
if [ "$VAL" != "6" ]; then
  fail "expected value, got $VAL"
fi

end_test
