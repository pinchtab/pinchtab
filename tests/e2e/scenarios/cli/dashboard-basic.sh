#!/bin/bash
# dashboard-basic.sh — Dashboard command scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab dashboard --no-open prints URL"

config_setup() {
  TMPDIR=$(mktemp -d)
  CFG="$TMPDIR/config.json"
}

config_cleanup() {
  rm -rf "$TMPDIR"
}

config_setup
cat > "$CFG" <<'EOF'
{
  "configVersion": "2",
  "server": {"port": "9870", "token": "test-dashboard-token"}
}
EOF

# Use --no-open to avoid actually launching a browser in CI
PINCHTAB_CONFIG="$CFG" pt_ok dashboard --no-open

assert_output_contains "Dashboard:" "prints dashboard heading"
assert_output_contains "127.0.0.1:9870" "prints correct URL"

# Token must NOT appear in the output (security)
assert_output_not_contains "test-dashboard-token" "token not leaked to stdout"

# Should mention clipboard (either success or unavailable)
if printf '%s\n%s\n' "$PT_OUT" "$PT_ERR" | grep -qi "clipboard\|copied"; then
  echo -e "  ${GREEN}✓${NC} clipboard feedback shown"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} no clipboard feedback in output"
  ((ASSERTIONS_FAILED++)) || true
fi

# Should suggest opening in browser since --no-open
assert_output_contains "Open this URL" "suggests opening URL manually"

config_cleanup
end_test

# ─────────────────────────────────────────────────────────────────
# Note: "no token" scenario is not testable in e2e because loadConfig()
# auto-generates a token when empty (by design).

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab dashboard --port override"

config_setup
cat > "$CFG" <<'EOF'
{
  "configVersion": "2",
  "server": {"port": "9870", "token": "abc"}
}
EOF

PINCHTAB_CONFIG="$CFG" pt_ok dashboard --no-open --port 4444

assert_output_contains "4444" "port override reflected in URL"
assert_output_not_contains "9870" "original port not shown"

config_cleanup
end_test
