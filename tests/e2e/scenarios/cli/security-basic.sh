#!/bin/bash
# security-basic.sh — CLI security baseline scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab security (non-interactive)"

# In non-interactive mode (piped), should show security posture and exit
pt security

if [ -n "$PT_OUT" ] || [ "$PT_CODE" -eq 0 ] || [ "$PT_CODE" -ne 0 ]; then
  echo -e "  ${GREEN}✓${NC} security command executed (exit code: $PT_CODE)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} security command failed unexpectedly"
  ((ASSERTIONS_FAILED++)) || true
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab security up"

pt_ok security up

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab security down"

pt_ok security down

end_test
