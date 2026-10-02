#!/bin/bash
# state-basic.sh — CLI tests for `pinchtab state` commands.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

STATE_NAME="cli-e2e-state-$(date +%s)"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state list shows saved states"

pt_cli state list
assert_cli_ok "list states"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state save captures current browser state"

pt_cli state save --name "$STATE_NAME"
assert_cli_ok "save state"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state show displays state details"

pt_cli state show --name "$STATE_NAME"
assert_cli_ok "show state"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state load restores saved state (exact name)"

pt_cli state load --name "$STATE_NAME"
assert_cli_ok "load exact name"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state load restores saved state (prefix)"

PREFIX=$(echo "$STATE_NAME" | cut -c1-8)
pt_cli state load --name "$PREFIX"
assert_cli_ok "load by prefix"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state delete removes the saved state"

pt_cli state delete --name "$STATE_NAME"
assert_cli_ok "delete state"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state clean runs without error"

pt_cli state clean --older-than 8760
assert_cli_ok "clean old states"

end_test

# ═══════════════════════════════════════════════════════════════════
# Encryption CLI tests (require security.stateEncryptionKey in config)
# ═══════════════════════════════════════════════════════════════════

ENCRYPTED_CLI_STATE="cli-encrypted-$(date +%s)"
ENCRYPTED_STATE_CREATED=0

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state save --encrypt creates encrypted state"

# This test requires stateEncryptionKey to be configured.
pt_cli state save --name "$ENCRYPTED_CLI_STATE" --encrypt

if [ "$PT_CODE" -eq 0 ]; then
  pass_assert "save encrypted state"
  ENCRYPTED_STATE_CREATED=1
elif echo "$PT_ERR" | grep -q "encryption key"; then
  skip_assert "skipped (stateEncryptionKey not configured)"
else
  fail_assert "save encrypted state (exit $PT_CODE)"
  echo "  stderr: $PT_ERR"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state load loads encrypted state"

if [ "${ENCRYPTED_STATE_CREATED}" -eq 1 ]; then
  pt_cli state load --name "$ENCRYPTED_CLI_STATE"
  assert_cli_ok "load encrypted state"
else
  skip_assert "skipped (encrypted state not created)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state delete cleans up encrypted state"

if [ "${ENCRYPTED_STATE_CREATED}" -eq 1 ]; then
  pt_cli state delete --name "$ENCRYPTED_CLI_STATE"
  assert_cli_ok "delete encrypted state"
else
  skip_assert "skipped"
fi

end_test

# ═══════════════════════════════════════════════════════════════════
# Positional name shapes
# ═══════════════════════════════════════════════════════════════════

POS_STATE="cli-pos-state-$(date +%s)"

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
start_test "pinchtab state save/show/load/delete <name> round trip positionally"

pt navigate "${FIXTURES_URL}/index.html"
assert_cli_ok "navigate to fixture"

pt_cli state save "$POS_STATE"
assert_cli_ok "save positional"

pt_cli state list
assert_output_contains "$POS_STATE" "list shows the positionally saved state"

pt_cli state show "$POS_STATE"
assert_cli_ok "show positional"
assert_output_contains "$POS_STATE" "show prints the named state"

pt_cli state load "$POS_STATE"
assert_cli_ok "load positional"

pt_cli state delete "$POS_STATE"
assert_cli_ok "delete positional"

pt_fail state show "$POS_STATE"

pt_cli state list
assert_output_not_contains "$POS_STATE" "list no longer shows the deleted state"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state verbs refuse a name given as argument and --name"

pt_cli state save "$POS_STATE"
assert_cli_ok "save for refusal checks"

for verb in save load show delete; do
  pt_fail state "$verb" "$POS_STATE" --name other-name
  assert_err_contains "twice" "state $verb refusal says the name was given twice"
done

pt_cli state show --name "$POS_STATE"
assert_cli_ok "state survives the refused delete (--name show)"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state show/delete/load with no name are refused"

for verb in show delete load; do
  pt_fail state "$verb"
done

pt_cli state show "$POS_STATE"
assert_cli_ok "state survives the bare delete"

pt_cli state delete "$POS_STATE"
assert_cli_ok "cleanup positional state"

end_test
