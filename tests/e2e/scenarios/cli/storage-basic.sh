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
pt_cli tabs list --json
TAB_ID=""
if [ "$PT_CODE" -eq 0 ]; then
  TAB_ID=$(echo "$PT_OUT" | safe_jq -r '.tabs[0].id // empty' 2>/dev/null)
fi

if [ -n "$TAB_ID" ]; then
  pt_cli storage get --tab "$TAB_ID"
  assert_cli_ok "get storage for tab $TAB_ID"
else
  skip_assert "skipped (no tab found)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage set --tab writes to specific tab"

if [ -n "$TAB_ID" ]; then
  pt_cli storage set pt_cli_tab_key pt_cli_tab_value --type local --tab "$TAB_ID"
  assert_cli_ok "set with --tab"
else
  skip_assert "skipped (no tab found)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage delete --tab removes key from specific tab"

if [ -n "$TAB_ID" ]; then
  pt_cli storage delete --key pt_cli_tab_key --type local --tab "$TAB_ID"
  assert_cli_ok "delete with --tab"
else
  skip_assert "skipped (no tab found)"
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# Positional key shapes
# ═══════════════════════════════════════════════════════════════════

assert_err_contains() {
  local needle="$1" desc="$2"
  if grep -q -- "$needle" <<<"$PT_ERR"; then
    pass_assert "$desc"
  else
    fail_assert "$desc"
    echo -e "  ${RED}  stderr was: $PT_ERR${NC}"
  fi
}

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage get <key> reads one item positionally"

pt navigate "${FIXTURES_URL}/index.html"
assert_cli_ok "navigate to fixture"
pt_cli storage set pos_k1 pos_v1 --type local
assert_cli_ok "set pos_k1"
pt_cli storage set pos_k2 pos_v2 --type local
assert_cli_ok "set pos_k2"

pt_cli storage get pos_k1 --type local
assert_cli_ok "get pos_k1 positionally"
assert_json_field '.local | length' '1' "positional get returns exactly one item"
assert_json_field '.local[0].key' 'pos_k1' "positional get returns the named key"
assert_json_field '.local[0].value' 'pos_v1' "positional get returns its value"

pt_cli storage get --key pos_k2 --type local
assert_cli_ok "get --key pos_k2 still works"
assert_json_field '.local[0].value' 'pos_v2' "--key get returns its value"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage get/delete refuse a key given as argument and --key"

pt_fail storage get pos_k1 --key pos_k2
assert_err_contains "pos_k1" "get refusal names the argument"
assert_err_contains "pos_k2" "get refusal names the flag value"

pt_fail storage delete pos_k1 --key pos_k2 --type local
assert_err_contains "twice" "delete refusal says the key was given twice"

pt_cli storage get --type local
assert_json_field '[.local[].key] | map(select(. == "pos_k1" or . == "pos_k2")) | length' '2' \
  "a refused delete removed nothing"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "bare pinchtab storage delete is refused, names storage clear, and wipes nothing"

pt_fail storage delete
assert_err_contains "storage clear" "refusal names storage clear"

pt_fail storage delete --key ""
assert_err_contains "storage clear" "empty --key refusal names storage clear"

pt_cli storage get --type local
assert_cli_ok "get after refused bare delete"
assert_json_field '[.local[].key] | map(select(. == "pos_k1" or . == "pos_k2")) | length' '2' \
  "both keys survive a bare storage delete"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab storage delete <key> removes only that key"

pt_cli storage delete pos_k1 --type local
assert_cli_ok "delete pos_k1 positionally"

pt_cli storage get --type local
assert_json_field '[.local[].key] | index("pos_k1")' 'null' "pos_k1 is gone"
assert_json_field '[.local[] | select(.key == "pos_k2") | .value][0]' 'pos_v2' "pos_k2 survives"

pt_cli storage delete --key pos_k2 --type local
assert_cli_ok "delete --key pos_k2 still works"
pt_cli storage get pos_k2 --type local
assert_json_field '.local | length' '0' "pos_k2 is gone after --key delete"

end_test
