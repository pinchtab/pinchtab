#!/bin/bash
# sessions-basic.sh — CLI agent session happy-path scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab session create"

pt_ok session create --agent-id "e2e-agent" --label "e2e-test"
assert_output_contains "sessionToken" "output contains sessionToken"
assert_output_contains "e2e-agent" "output contains agentId"
assert_output_contains "active" "output contains status active"

SESSION_ID=$(echo "$PT_OUT" | jq -r '.id')
SESSION_TOKEN=$(echo "$PT_OUT" | jq -r '.sessionToken')

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab session create without --agent-id fails"

pt_fail session create
if echo "$PT_ERR" | grep -q "agent-id"; then
  echo -e "  ${GREEN}✓${NC} error mentions --agent-id"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} error mentions --agent-id (stderr: $PT_ERR)"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab session list"

pt_ok session list
assert_output_contains "$SESSION_ID" "list includes created session"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab session info with session token"

PINCHTAB_SESSION="$SESSION_TOKEN" pt_ok session info
assert_output_contains "e2e-agent" "info shows agentId"
assert_output_contains "active" "info shows active status"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab session revoke"

pt_ok session revoke "$SESSION_ID"
assert_output_contains "ok" "revoke returns ok status"

# Subsequent info with revoked token should fail
PINCHTAB_SESSION="$SESSION_TOKEN" pt_fail session info

end_test
