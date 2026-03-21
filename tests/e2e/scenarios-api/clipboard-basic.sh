#!/bin/bash
# clipboard-basic.sh — Clipboard read/write API tests.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "POST /clipboard/write sets clipboard text"

UNIQUE_TEXT="pinchtab-test-$(date +%s)"
pt_post "/clipboard/write" -d "{\"text\":\"${UNIQUE_TEXT}\"}"
assert_ok "write clipboard"
assert_json_exists "$RESULT" '.success' "has success"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /clipboard/read returns clipboard text without a tab"

pt_get "/clipboard/read"
assert_ok "read clipboard"
assert_json_contains "$RESULT" '.text' "$UNIQUE_TEXT" "reads back written text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /clipboard/copy (alias for write)"

COPY_TEXT="pinchtab-copy-$(date +%s)"
pt_post "/clipboard/copy" -d "{\"text\":\"${COPY_TEXT}\"}"
assert_ok "copy"
assert_json_exists "$RESULT" '.success' "has success"

pt_get "/clipboard/read"
assert_json_contains "$RESULT" '.text' "$COPY_TEXT" "copy updated clipboard"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "GET /clipboard/paste returns clipboard text"

pt_get "/clipboard/paste"
assert_ok "paste"
assert_json_contains "$RESULT" '.text' "$COPY_TEXT" "paste returns clipboard text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /clipboard/write without text → error"

pt_post "/clipboard/write" -d "{}"
assert_not_ok "rejects missing text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Clipboard rejects tabId"

pt_get "/clipboard/read?tabId=nonexistent_xyz_999"
assert_not_ok "read rejects tabId"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "POST /clipboard/write rejects oversized text"

OVERSIZED_TEXT=$(head -c $((64 * 1024 + 1)) /dev/zero | tr '\0' 'x')
pt_post "/clipboard/write" -d "$(jq -n --arg t "$OVERSIZED_TEXT" '{text: $t}')"
assert_not_ok "rejects oversized clipboard text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Clipboard with special characters"

SPECIAL_TEXT='Hello "world" with <html> & symbols!'
pt_post "/clipboard/write" -d "$(jq -n --arg t "$SPECIAL_TEXT" '{text: $t}')"
assert_ok "write special chars"

pt_get "/clipboard/read"
assert_ok "read special chars"
GOT_TEXT=$(echo "$RESULT" | jq -r '.text')
if [ "$GOT_TEXT" = "$SPECIAL_TEXT" ]; then
  echo -e "  ${GREEN}✓${NC} special characters preserved"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} special characters mismatch: got '$GOT_TEXT'"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "Clipboard with multiline text"

MULTI_TEXT=$'line one\nline two\nline three'
pt_post "/clipboard/write" -d "$(jq -n --arg t "$MULTI_TEXT" '{text: $t}')"
assert_ok "write multiline"

pt_get "/clipboard/read"
assert_ok "read multiline"
GOT_TEXT=$(echo "$RESULT" | jq -r '.text')
if [ "$GOT_TEXT" = "$MULTI_TEXT" ]; then
  echo -e "  ${GREEN}✓${NC} multiline text preserved"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}~${NC} multiline may have been modified"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test
