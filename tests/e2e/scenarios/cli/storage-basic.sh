#!/bin/bash
# storage-basic.sh — CLI tests for `pinchtab storage` commands.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ═══════════════════════════════════════════════════════════════════
# Setup: Navigate to a fixture page so storage has a valid origin
# ═══════════════════════════════════════════════════════════════════

start_test "Setup: navigate to fixture page for storage tests"

pt navigate "${FIXTURES_URL}/index.html"
assert_cli_ok "navigate to fixture"
sleep 0.5

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage set writes a localStorage item"

pt_cli storage set pt_cli_key pt_cli_value --type local
assert_cli_ok "set local item"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage get reads back the item"

pt_cli storage get --type local --key pt_cli_key
assert_cli_ok "get local item"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage set writes a sessionStorage item"

pt_cli storage set pt_sess_key pt_sess_value --type session
assert_cli_ok "set session item"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage delete removes a key"

pt_cli storage delete --key pt_cli_key --type local
assert_cli_ok "delete local key"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage clear --all clears both stores"

pt_cli storage clear --all
assert_cli_ok "clear --all"

end_test

# ═══════════════════════════════════════════════════════════════════
# Tab-scoped storage CLI tests (using --tab flag)
# ═══════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage get --tab reads storage for specific tab"

# Get the actual tab ID from tabs list
pt_cli tabs list
TAB_ID=""
if [ "$PT_CODE" -eq 0 ]; then
  TAB_ID=$(echo "$PT_OUT" | safe_jq -r '.tabs[0].id // empty' 2>/dev/null)
fi

if [ -n "$TAB_ID" ] && [ "$TAB_ID" != "null" ]; then
  pt_cli storage get --tab "$TAB_ID"
  assert_cli_ok "get storage for tab $TAB_ID"
else
  echo -e "  ${YELLOW}⊘${NC} skipped (no tab found)"
  ((ASSERTIONS_SKIPPED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage set --tab writes to specific tab"

if [ -n "$TAB_ID" ] && [ "$TAB_ID" != "null" ]; then
  pt_cli storage set pt_cli_tab_key pt_cli_tab_value --type local --tab "$TAB_ID"
  assert_cli_ok "set with --tab"
else
  echo -e "  ${YELLOW}⊘${NC} skipped (no tab found)"
  ((ASSERTIONS_SKIPPED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage delete --tab removes key from specific tab"

if [ -n "$TAB_ID" ] && [ "$TAB_ID" != "null" ]; then
  pt_cli storage delete --key pt_cli_tab_key --type local --tab "$TAB_ID"
  assert_cli_ok "delete with --tab"
else
  echo -e "  ${YELLOW}⊘${NC} skipped (no tab found)"
  ((ASSERTIONS_SKIPPED++)) || true
fi

end_test
