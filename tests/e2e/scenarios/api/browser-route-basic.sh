#!/bin/bash
# browser-route-basic.sh — Browser route metadata assertions.
# Covers: /text?browser=, /snapshot?browser= with the configured provider.
#
# Cross-browser routing (e.g. ?browser=chrome on a cloak stack) lives in
# the browser-routing smoke test which sets up all 3 browsers.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# The PINCHTAB_E2E_BROWSER env tells scenarios which browser the
# stack was launched with. Default to chrome when unset.
EXPECTED_BROWSER="${PINCHTAB_E2E_BROWSER:-chrome}"

# ─────────────────────────────────────────────────────────────────
start_test "browser route: /text response matches expected provider"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate to form"

# Request text with the currently configured provider.
pt_get "/text?browser=${EXPECTED_BROWSER}"
assert_ok "text with browser=${EXPECTED_BROWSER}"
assert_contains "$RESULT" "Form\|Name\|Email\|Submit" "text has form content"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "browser route: /snapshot response matches expected provider"

pt_get "/snapshot?browser=${EXPECTED_BROWSER}"
assert_ok "snapshot with browser=${EXPECTED_BROWSER}"
assert_json_length_gte "$RESULT" '.nodes' 1 "snapshot has nodes"

end_test
