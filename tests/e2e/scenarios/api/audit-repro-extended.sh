#!/bin/bash
# audit-repro-extended.sh — the audit reproducibility guarantee: same input,
# same report. Uses the checked-in fixtures/audit-site/normalize-report.jq
# to strip volatile fields; the golden report is
# fixtures/audit-site/golden-report.json (regeneration instructions live in
# the jq filter's header comment).

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

NORMALIZE="${GROUP_DIR}/../../fixtures/audit-site/normalize-report.jq"
GOLDEN="${GROUP_DIR}/../../fixtures/audit-site/golden-report.json"
RUN_BODY="{\"sitemapUrl\":\"${FIXTURES_URL}/audit-site/sitemap.xml\",\"options\":{\"screenshot\":false},\"concurrency\":1}"

# ─────────────────────────────────────────────────────────────────
start_test "two identical audit runs normalize byte-identical"

pt_post /audit -d "$RUN_BODY"
assert_ok "first audit run"
RUN_ONE=$(echo "$RESULT" | jq -S -f "$NORMALIZE")

pt_post /audit -d "$RUN_BODY"
assert_ok "second audit run"
RUN_TWO=$(echo "$RESULT" | jq -S -f "$NORMALIZE")

if [ -n "$RUN_ONE" ] && [ "$RUN_ONE" = "$RUN_TWO" ]; then
  pass_assert "normalized reports are byte-identical"
else
  fail_assert "normalized reports differ"
  diff <(echo "$RUN_ONE") <(echo "$RUN_TWO") | head -20
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "normalized report matches the checked-in golden"

if [ ! -f "$GOLDEN" ]; then
  fail_assert "golden report exists at $GOLDEN"
elif diff -q <(echo "$RUN_ONE") "$GOLDEN" >/dev/null 2>&1; then
  pass_assert "live report matches golden-report.json"
else
  fail_assert "live report diverges from golden-report.json (schema/content drift; see normalize-report.jq header for how to regenerate)"
  diff <(echo "$RUN_ONE") "$GOLDEN" | head -30
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "concurrency 4 audits the same page set as serial"

pt_post /audit -d "{\"sitemapUrl\":\"${FIXTURES_URL}/audit-site/sitemap.xml\",\"options\":{\"screenshot\":false},\"concurrency\":4}"
assert_ok "concurrent audit run"
SET_FOUR=$(echo "$RESULT" | jq -S '[.pages[].url] | sort')
SET_ONE=$(echo "$RUN_ONE" | jq -S '[.pages[].url] | sort')

if [ "$SET_ONE" = "$SET_FOUR" ]; then
  pass_assert "identical page URL set at concurrency 1 and 4"
else
  fail_assert "page sets differ: $SET_ONE vs $SET_FOUR"
fi

end_test
