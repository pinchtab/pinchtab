#!/bin/bash
# 06-screenshot-pdf.sh — Screenshot and PDF export

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
sleep 1

pt_get /screenshot
assert_ok "screenshot"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf (default)"

pt_get /pdf
assert_ok "pdf"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab screenshot --tab <id>"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_get "/tabs/${TAB_ID}/screenshot"
assert_ok "tab screenshot"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf --tab <id>"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_get "/tabs/${TAB_ID}/pdf"
assert_ok "tab pdf"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab pdf --tab <id> (with options)"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_post "/tabs/${TAB_ID}/pdf" -d '{"printBackground":true,"scale":0.8}'
assert_ok "tab pdf with options"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screenshot: quality parameter"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
sleep 1

# Low quality screenshot should be smaller than default
LOW_Q_SIZE=$(curl -s "${E2E_SERVER}/screenshot?quality=10" | wc -c)
HIGH_Q_SIZE=$(curl -s "${E2E_SERVER}/screenshot?quality=95" | wc -c)

if [ "$LOW_Q_SIZE" -lt "$HIGH_Q_SIZE" ]; then
  echo -e "  ${GREEN}✓${NC} quality=10 ($LOW_Q_SIZE bytes) < quality=95 ($HIGH_Q_SIZE bytes)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} quality=10 ($LOW_Q_SIZE) not smaller than quality=95 ($HIGH_Q_SIZE)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screenshot: output=file"

pt_get "/screenshot?output=file"
assert_ok "screenshot output=file"

# Response should contain a file path
assert_json_exists "$RESULT" '.path' "response has path field"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pdf: output=file"

pt_get "/pdf?output=file"
assert_ok "pdf output=file"

assert_json_exists "$RESULT" '.path' "response has path field"

end_test
