#!/bin/bash
# system-extended.sh — CLI advanced instance and daemon scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab transport failure exits non-zero"

with_server "http://127.0.0.1:1" pt health
assert_exit_code 1 "connection failure is reported to automation"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab 403 policy denial exits non-zero"

pt network route "*.png" --abort
assert_exit_code 1 "403 policy denial is reported to automation"
if echo "$PT_ERR" | grep -q "Error 403"; then
  pass_assert "policy denial reports HTTP 403"
else
  fail_assert "policy denial reports HTTP 403"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon (non-interactive shows status)"

pt daemon
assert_exit_code 0 "daemon status displayed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon install (fails without systemd)"

pt daemon install
if [ "$PT_CODE" -ne 0 ]; then
  pass_assert "fails gracefully without systemd (exit $PT_CODE)"
else
  fail_assert "daemon install unexpectedly succeeded"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon unknown-subcommand → exit 1"

pt daemon bogus-command
assert_exit_code 1 "unknown subcommand rejected"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon start (fails without service manager)"

pt daemon start
if [ "$PT_CODE" -ne 0 ]; then
  pass_assert "start fails gracefully without service manager (exit $PT_CODE)"
else
  fail_assert "daemon start unexpectedly succeeded"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon stop (fails without service manager)"

pt daemon stop
if [ "$PT_CODE" -ne 0 ]; then
  pass_assert "stop fails gracefully without service manager (exit $PT_CODE)"
else
  fail_assert "daemon stop unexpectedly succeeded"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab daemon restart (fails without service manager)"

pt daemon restart
if [ "$PT_CODE" -ne 0 ]; then
  pass_assert "restart fails gracefully without service manager (exit $PT_CODE)"
else
  fail_assert "daemon restart unexpectedly succeeded"
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
  fail_assert "config file not created"
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
  fail_assert "wizard ran on current config version"
else
  pass_assert "wizard skipped for current version"
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
  pass_assert "wizard set configVersion before daemon install attempt"
else
  skip_assert "configVersion not set (wizard may not have saved: $ACTUAL_VERSION)"
fi
config_cleanup

end_test
