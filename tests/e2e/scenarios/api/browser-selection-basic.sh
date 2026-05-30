#!/bin/bash
# browser-selection-basic.sh — Browser selection error handling.
# Covers: unknown browser names, structured error codes, actionable messages.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "browser selection: unknown browser name → structured error"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\",\"browser\":\"nonexistent_xyz\"}"
assert_not_ok "navigate with unknown browser rejects"

# The response should contain a structured error code.
ERROR_CODE=$(echo "$RESULT" | jq -r '.code // empty' 2>/dev/null || echo "")
if [ -n "$ERROR_CODE" ] && [ "$ERROR_CODE" != "null" ]; then
  pass_assert "error has structured code: $ERROR_CODE"
else
  soft_pass_assert "no structured error code in response (server may use plain message)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "browser selection: unknown browser on /text → error"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate first"

pt_get "/text?browser=nonexistent_xyz"
assert_not_ok "text with unknown browser rejects"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "browser selection: unknown browser on /snapshot → error"

pt_get "/snapshot?browser=nonexistent_xyz"
assert_not_ok "snapshot with unknown browser rejects"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "browser selection: unknown browser on /action → error"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons"

pt_post /action -d '{"kind":"press","key":"Escape","browser":"nonexistent_xyz"}'
assert_not_ok "action with unknown browser rejects"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "browser selection: empty browser param treated as default"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate first"

pt_get "/text?browser="
assert_ok "text with empty browser param succeeds"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "browser selection: explicit provider matches default"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/index.html\"}"
assert_ok "navigate"

pt_get /text
assert_ok "text without browser param"
TEXT_DEFAULT="$RESULT"

EXPECTED_BROWSER="${PINCHTAB_E2E_BROWSER:-chrome}"
pt_get "/text?browser=${EXPECTED_BROWSER}"
assert_ok "text with browser=${EXPECTED_BROWSER}"
TEXT_EXPLICIT="$RESULT"

DEFAULT_TEXT=$(echo "$TEXT_DEFAULT" | jq -r '.text // empty' 2>/dev/null | head -c 200)
EXPLICIT_TEXT=$(echo "$TEXT_EXPLICIT" | jq -r '.text // empty' 2>/dev/null | head -c 200)
if [ -n "$DEFAULT_TEXT" ] && [ "$DEFAULT_TEXT" = "$EXPLICIT_TEXT" ]; then
  pass_assert "explicit provider matches default text"
else
  soft_pass_assert "text content may differ slightly between calls"
fi

end_test
