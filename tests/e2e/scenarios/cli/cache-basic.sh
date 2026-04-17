#!/bin/bash
# cache-basic.sh — CLI cache management scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab cache clear"

pt_ok nav "${FIXTURES_URL}/form.html"
pt_ok cache clear
assert_output_contains "cleared" "cache clear response"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab cache status"

pt_ok cache status
assert_json_field ".canClear" "true" "cache can be cleared"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab cache clear is idempotent"

pt_ok cache clear
pt_ok cache clear
assert_output_contains "cleared" "cache clear works multiple times"

end_test
