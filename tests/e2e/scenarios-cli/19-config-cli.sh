#!/bin/bash
# 19-config-cli.sh — Config CLI subcommands (init, show, path, set, patch, validate, get)

source "$(dirname "$0")/common.sh"

# Helper: create temp config dir, set CFG and TMPDIR
config_setup() {
  TMPDIR=$(mktemp -d)
  CFG="$TMPDIR/config.json"
}

# Helper: clean up temp dir
config_cleanup() {
  rm -rf "$TMPDIR"
}

# Helper: init config in temp dir
config_init() {
  PINCHTAB_CONFIG="$CFG" HOME="$TMPDIR" pt_ok config init
}

# Helper: assert config file JSON field
assert_config_field() {
  local path="$1" expected="$2" desc="$3"
  local actual
  actual=$(jq -r "$path" "$CFG" 2>/dev/null)
  if [ "$actual" = "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (expected $expected, got $actual)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

# ═══════════════════════════════════════════════════════════════════
start_test "config init creates valid config"

config_setup
config_init

# Check which path was created
CFG_FILE="$CFG"
[ -f "$CFG_FILE" ] || CFG_FILE="$TMPDIR/.pinchtab/config.json"
assert_file_exists "$CFG_FILE" "config file created"
CFG="$CFG_FILE"

# Verify structure has expected sections
if jq -e '.server' "$CFG" >/dev/null 2>&1; then
  echo -e "  ${GREEN}✓${NC} has server section"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} missing server section"
  ((ASSERTIONS_FAILED++)) || true
fi
if jq -e '.browser' "$CFG" >/dev/null 2>&1; then
  echo -e "  ${GREEN}✓${NC} has browser section"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} missing browser section"
  ((ASSERTIONS_FAILED++)) || true
fi
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config show displays config"

config_setup
PINCHTAB_CONFIG="$CFG" pt_ok config show
assert_output_contains "Server" "has Server section header"
assert_output_contains "Browser" "has Browser section header"
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config path outputs config file path"

config_setup
EXPECTED_PATH="$TMPDIR/custom-config.json"
PINCHTAB_CONFIG="$EXPECTED_PATH" pt_ok config path
assert_output_contains "$EXPECTED_PATH" "path matches expected"
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config set updates a value"

config_setup
config_init
PINCHTAB_CONFIG="$CFG" pt_ok config set server.port 8080
assert_output_contains "Set server.port = 8080" "success message"
assert_config_field ".server.port" "8080" "file contains port 8080"
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config patch merges JSON"

config_setup
config_init
PINCHTAB_CONFIG="$CFG" pt_ok config patch '{"server":{"port":"7777"},"instanceDefaults":{"maxTabs":100}}'
assert_config_field ".server.port" "7777" "port set to 7777"
assert_config_field ".instanceDefaults.maxTabs" "100" "maxTabs set to 100"
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config validate accepts valid config"

config_setup
cat > "$CFG" <<'EOF'
{
  "server": {"port": "9867"},
  "instanceDefaults": {"stealthLevel": "light", "tabEvictionPolicy": "reject"},
  "multiInstance": {"strategy": "simple", "allocationPolicy": "fcfs"}
}
EOF
PINCHTAB_CONFIG="$CFG" pt_ok config validate
assert_output_contains "valid" "reports valid"
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config validate rejects invalid config"

config_setup
cat > "$CFG" <<'EOF'
{
  "server": {"port": "99999"},
  "instanceDefaults": {"stealthLevel": "superstealth"},
  "multiInstance": {"strategy": "magical"}
}
EOF
PINCHTAB_CONFIG="$CFG" pt_fail config validate
assert_output_contains "error" "reports error"
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config get retrieves a value"

config_setup
config_init
PINCHTAB_CONFIG="$CFG" pt_ok config set server.port 7654
PINCHTAB_CONFIG="$CFG" pt_ok config get server.port
assert_output_contains "7654" "got value 7654"
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config get fails for unknown path"

config_setup
PINCHTAB_CONFIG="$CFG" pt_fail config get unknown.field
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config get returns slice as comma-separated"

config_setup
config_init
PINCHTAB_CONFIG="$CFG" pt_ok config set security.attach.allowHosts "127.0.0.1,localhost"
PINCHTAB_CONFIG="$CFG" pt_ok config get security.attach.allowHosts
assert_output_contains "127.0.0.1,localhost" "got comma-separated value"
config_cleanup
end_test

# ═══════════════════════════════════════════════════════════════════
start_test "config show loads legacy flat config"

config_setup
cat > "$CFG" <<'EOF'
{
  "port": "8765",
  "headless": true,
  "maxTabs": 30
}
EOF
PINCHTAB_CONFIG="$CFG" pt_ok config show
assert_output_contains "8765" "shows port from legacy config"
config_cleanup
end_test
