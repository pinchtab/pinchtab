#!/bin/bash
# audit-security-extended.sh — rule-based security findings in audit runs.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "forms.html yields a password-form-over-http finding"

pt_post /audit -d "{\"urls\":[\"${FIXTURES_URL}/audit-site/forms.html\"],\"options\":{\"screenshot\":false}}"
assert_ok "audit forms.html"

FINDING=$(echo "$RESULT" | jq '.pages[0].securityFindings[]? | select(.ruleId == "insecure-password-form")')
if [ -n "$FINDING" ]; then
  pass_assert "page carries insecure-password-form finding"
else
  fail_assert "page carries insecure-password-form finding (got: $(echo "$RESULT" | jq -c '.pages[0].securityFindings'))"
fi

if echo "$FINDING" | jq -e '.severity == "high"' >/dev/null 2>&1; then
  pass_assert "finding has severity high"
else
  fail_assert "finding has severity high (got: $(echo "$FINDING" | jq -c '.severity'))"
fi

if echo "$RESULT" | jq -e '.securityFindings[]? | select(.ruleId == "insecure-password-form")' >/dev/null 2>&1; then
  pass_assert "finding aggregated into the report security section"
else
  fail_assert "finding aggregated into the report security section"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "clean.html yields zero security findings"

pt_post /audit -d "{\"urls\":[\"${FIXTURES_URL}/audit-site/clean.html\"],\"options\":{\"screenshot\":false}}"
assert_ok "audit clean.html"

COUNT=$(echo "$RESULT" | jq '[.pages[0].securityFindings[]?] | length')
if [ "$COUNT" -eq 0 ]; then
  pass_assert "zero page security findings"
else
  fail_assert "zero page security findings (got: $(echo "$RESULT" | jq -c '.pages[0].securityFindings'))"
fi

SITE_COUNT=$(echo "$RESULT" | jq '[.securityFindings[]?] | length')
if [ "$SITE_COUNT" -eq 0 ]; then
  pass_assert "zero report-level security findings"
else
  fail_assert "zero report-level security findings (got: $SITE_COUNT)"
fi

end_test
