#!/bin/bash
# clipboard-basic.sh — CLI clipboard commands.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab clipboard write <text>"

UNIQUE_TEXT="pinchtab-cli-$(date +%s)"
pt_ok clipboard write "$UNIQUE_TEXT"
assert_output_contains "updated\|Clipboard" "confirms update"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab clipboard read"

pt_ok clipboard read
assert_output_contains "$UNIQUE_TEXT" "reads back written text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab clipboard copy <text>"

COPY_TEXT="pinchtab-copy-$(date +%s)"
pt_ok clipboard copy "$COPY_TEXT"
assert_output_contains "updated\|Clipboard" "confirms copy"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab clipboard paste"

pt_ok clipboard paste
assert_output_contains "$COPY_TEXT" "paste returns clipboard text"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab clipboard help"

pt_ok clipboard --help
assert_output_contains "read\|write\|clipboard" "shows usage"

end_test
