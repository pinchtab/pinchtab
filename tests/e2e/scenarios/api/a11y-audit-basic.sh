#!/bin/bash
# a11y-audit-basic.sh — GET /a11y/audit accessibility score and findings.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "a11y-issues page scores below 100 with expected rules"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/audit-site/a11y-issues.html\"}"
assert_ok "navigate to a11y-issues.html"

pt_get /a11y/audit
assert_ok "get a11y audit"
assert_json_exists "$RESULT" '.score' "has score"
assert_json_exists "$RESULT" '.findings' "has findings"

SCORE=$(echo "$RESULT" | jq '.score')
if [ "$SCORE" -lt 100 ] 2>/dev/null; then
  pass_assert "score < 100 (got: $SCORE)"
else
  fail_assert "score < 100 (got: $SCORE)"
fi

for rule in missing-alt missing-label empty-link; do
  if echo "$RESULT" | jq -e --arg rule "$rule" '.findings[] | select(.rule == $rule)' >/dev/null 2>&1; then
    pass_assert "findings include $rule"
  else
    fail_assert "findings include $rule (got: $(echo "$RESULT" | jq -c '[.findings[].rule]'))"
  fi
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "a11y audit is deterministic across consecutive calls"

pt_get /a11y/audit
assert_ok "first audit call"
FIRST=$(echo "$RESULT" | jq -S '{score, findings}')

pt_get /a11y/audit
assert_ok "second audit call"
SECOND=$(echo "$RESULT" | jq -S '{score, findings}')

if [ "$FIRST" = "$SECOND" ]; then
  pass_assert "consecutive audits identical"
else
  fail_assert "consecutive audits differ: $FIRST vs $SECOND"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "clean page scores 100 with zero findings"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/audit-site/clean.html\"}"
assert_ok "navigate to clean.html"

pt_get /a11y/audit
assert_ok "get a11y audit"
assert_json_eq "$RESULT" '.score' "100" "score is 100"
assert_json_length "$RESULT" '.findings' 0 "zero findings"

end_test
