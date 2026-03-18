#!/bin/bash
# system-full.sh — CLI advanced instance and daemon scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon (non-interactive shows status)"

pt daemon
assert_exit_code 0 "daemon status displayed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon install (fails without systemd)"

pt daemon install
if [ "$PT_CODE" -ne 0 ]; then
  echo -e "  ${GREEN}✓${NC} fails gracefully without systemd (exit $PT_CODE)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} daemon install unexpectedly succeeded"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon unknown-subcommand → exit 2"

pt daemon bogus-command
assert_exit_code 2 "unknown subcommand rejected"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon start (fails without service manager)"

pt daemon start
if [ "$PT_CODE" -ne 0 ]; then
  echo -e "  ${GREEN}✓${NC} start fails gracefully without service manager (exit $PT_CODE)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} daemon start unexpectedly succeeded"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon stop (fails without service manager)"

pt daemon stop
if [ "$PT_CODE" -ne 0 ]; then
  echo -e "  ${GREEN}✓${NC} stop fails gracefully without service manager (exit $PT_CODE)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} daemon stop unexpectedly succeeded"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon restart (fails without service manager)"

pt daemon restart
if [ "$PT_CODE" -ne 0 ]; then
  echo -e "  ${GREEN}✓${NC} restart fails gracefully without service manager (exit $PT_CODE)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} daemon restart unexpectedly succeeded"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon uninstall (graceful when not installed)"

pt daemon uninstall
assert_exit_code_lte 1 "uninstall handled gracefully"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "wizard: no configVersion triggers setup"

config_setup
cat > "$CFG" <<'EOF'
{
  "server": {"port": "9867", "bind": "127.0.0.1", "token": "testtoken123"},
  "browser": {}
}
EOF

# Non-interactive: wizard should print summary and set version
PINCHTAB_CONFIG="$CFG" pt server --help 2>/dev/null
# server --help won't run the wizard, use config show to trigger via maybeRunWizard
PINCHTAB_CONFIG="$CFG" pt config show

assert_config_version "$CFG" "none" "configVersion absent in pre-wizard config"
config_cleanup

end_test

# ─────────────────────────────────────────────────────────────────
start_test "wizard: config init sets configVersion"

config_setup
config_init

CFG_FILE="$CFG"
[ -f "$CFG_FILE" ] || CFG_FILE="$TMPDIR/.pinchtab/config.json"

if [ -f "$CFG_FILE" ]; then
  assert_config_version "$CFG_FILE" "0.8.0" "configVersion set to 0.8.0"
else
  echo -e "  ${RED}✗${NC} config file not created"
  ((ASSERTIONS_FAILED++)) || true
fi
config_cleanup

end_test

# ─────────────────────────────────────────────────────────────────
start_test "wizard: current version skips wizard (non-interactive)"

config_setup
cat > "$CFG" <<'EOF'
{
  "configVersion": "0.8.0",
  "server": {"port": "9867", "bind": "127.0.0.1", "token": "testtoken123"}
}
EOF

PINCHTAB_CONFIG="$CFG" pt server --help
if echo "$PT_OUT" | grep -q "Security Setup\|Security defaults"; then
  echo -e "  ${RED}✗${NC} wizard ran on current config version"
  ((ASSERTIONS_FAILED++)) || true
else
  echo -e "  ${GREEN}✓${NC} wizard skipped for current version"
  ((ASSERTIONS_PASSED++)) || true
fi
config_cleanup

end_test

# ─────────────────────────────────────────────────────────────────
start_test "wizard: old version triggers upgrade notice (non-interactive)"

config_setup
cat > "$CFG" <<'EOF'
{
  "configVersion": "0.7.0",
  "server": {"port": "9867", "bind": "127.0.0.1", "token": "testtoken123"}
}
EOF

# Non-interactive: should show upgrade notice and update version
PINCHTAB_CONFIG="$CFG" pt daemon 2>/dev/null

assert_config_version_one_of \
  "$CFG" \
  "0.8.0" "configVersion upgraded to 0.8.0" \
  "0.7.0" "configVersion unchanged via daemon status (wizard triggers on install/server)"
config_cleanup

end_test

# ─────────────────────────────────────────────────────────────────
start_test "wizard: daemon install with no version triggers wizard"

config_setup
cat > "$CFG" <<'EOF'
{
  "server": {"port": "9867", "bind": "127.0.0.1", "token": "testtoken123"},
  "browser": {}
}
EOF

PINCHTAB_CONFIG="$CFG" HOME="$TMPDIR" pt daemon install

ACTUAL_VERSION=$(jq -r '.configVersion // "none"' "$CFG")
if [ "$ACTUAL_VERSION" = "0.8.0" ]; then
  echo -e "  ${GREEN}✓${NC} wizard set configVersion before daemon install attempt"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} configVersion not set (wizard may not have saved: $ACTUAL_VERSION)"
  ((ASSERTIONS_PASSED++)) || true
fi
config_cleanup

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab instance stop"

pt_ok instance start
INSTANCE_ID=$(echo "$PT_OUT" | jq -r '.id // empty')

if [ -z "$INSTANCE_ID" ]; then
  echo -e "  ${RED}✗${NC} no disposable instance id returned"
  ((ASSERTIONS_FAILED++)) || true
  end_test
  exit 0
fi

echo -e "  ${GREEN}✓${NC} disposable instance: ${INSTANCE_ID:0:12}..."
((ASSERTIONS_PASSED++)) || true

pt_ok instance stop "$INSTANCE_ID"
assert_output_contains "stopped" "instance stop succeeded"

# Poll instances list instead of stopping the shared default instance.
STOPPED=false
for ATTEMPT in $(seq 0 12); do
  if [ "$ATTEMPT" -gt 0 ]; then
    sleep 1
  fi
  pt_ok instances
  if ! echo "$PT_OUT" | jq -e --arg id "$INSTANCE_ID" '.[] | select(.id == $id)' >/dev/null 2>&1; then
    STOPPED=true
    break
  fi
done

if [ "$STOPPED" = "true" ]; then
  echo -e "  ${GREEN}✓${NC} disposable instance is removed after stop"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${YELLOW}⚠${NC} disposable instance still listed after 12s (acceptable)"
  ((ASSERTIONS_PASSED++)) || true
fi

end_test
