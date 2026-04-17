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

pt_cli state clean --older-than 9999
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
  echo -e "  ${GREEN}✓${NC} save encrypted state"
  ((ASSERTIONS_PASSED++)) || true
  ENCRYPTED_STATE_CREATED=1
elif echo "$PT_ERR" | grep -q "encryption key"; then
  echo -e "  ${YELLOW}⊘${NC} skipped (stateEncryptionKey not configured)"
  ((ASSERTIONS_SKIPPED++)) || true
else
  echo -e "  ${RED}✗${NC} save encrypted state (exit $PT_CODE)"
  echo "  stderr: $PT_ERR"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state load loads encrypted state"

if [ "${ENCRYPTED_STATE_CREATED}" -eq 1 ]; then
  pt_cli state load --name "$ENCRYPTED_CLI_STATE"
  assert_cli_ok "load encrypted state"
else
  echo -e "  ${YELLOW}⊘${NC} skipped (encrypted state not created)"
  ((ASSERTIONS_SKIPPED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab state delete cleans up encrypted state"

if [ "${ENCRYPTED_STATE_CREATED}" -eq 1 ]; then
  pt_cli state delete --name "$ENCRYPTED_CLI_STATE"
  assert_cli_ok "delete encrypted state"
else
  echo -e "  ${YELLOW}⊘${NC} skipped"
  ((ASSERTIONS_SKIPPED++)) || true
fi

end_test
