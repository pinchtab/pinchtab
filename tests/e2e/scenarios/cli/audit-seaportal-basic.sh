#!/bin/bash
# audit-seaportal-basic.sh — pinchtab audit --seaportal-report ingestion.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

SEAPORTAL_REPORT="${GROUP_DIR}/../../fixtures/audit-site/seaportal-report.json"
CLEAN_URL="http://fixtures/audit-site/clean.html"

# ─────────────────────────────────────────────────────────────────
start_test "seaportal report drives routing via browserRecommended"

pt_ok audit --seaportal-report "$SEAPORTAL_REPORT" --json

PAGE_COUNT=$(echo "$PT_OUT" | jq '.pages | length' 2>/dev/null)
if [ "${PAGE_COUNT:-0}" -eq 3 ] 2>/dev/null; then
  pass_assert "all 3 report pages present"
else
  fail_assert "all 3 report pages present (got: $PAGE_COUNT)"
fi

if echo "$PT_OUT" | jq -e '.input.seaportalFormat == "seaportal-results/v0"' >/dev/null 2>&1; then
  pass_assert "input carries seaportal format version"
else
  fail_assert "input carries seaportal format version"
fi

CLEAN_PAGE=$(echo "$PT_OUT" | jq --arg url "$CLEAN_URL" '.pages[] | select(.url == $url)')

if echo "$CLEAN_PAGE" | jq -e 'has("screenshot") | not' >/dev/null 2>&1; then
  pass_assert "browserRecommended=false page has no screenshot"
else
  fail_assert "browserRecommended=false page has no screenshot"
fi

if echo "$CLEAN_PAGE" | jq -e '.seaportal.title == "Clean Page — Audit Fixture Site"' >/dev/null 2>&1; then
  pass_assert "non-enriched page keeps seaportal title"
else
  fail_assert "non-enriched page keeps seaportal title (got: $(echo "$CLEAN_PAGE" | jq -c '.seaportal'))"
fi

if echo "$CLEAN_PAGE" | jq -e '.seaportal.description == "Defect-free control page for audit e2e scenarios."' >/dev/null 2>&1; then
  pass_assert "non-enriched page keeps seaportal description"
else
  fail_assert "non-enriched page keeps seaportal description"
fi

ENRICHED=$(echo "$PT_OUT" | jq '.pages[] | select(.url | endswith("index.html"))')
if echo "$ENRICHED" | jq -e '.screenshot | length > 0' >/dev/null 2>&1; then
  pass_assert "browserRecommended=true page is enriched (screenshot)"
else
  fail_assert "browserRecommended=true page is enriched (screenshot)"
fi
if echo "$ENRICHED" | jq -e '.seaportal.description | length > 0' >/dev/null 2>&1; then
  pass_assert "enriched page also carries seaportal fields"
else
  fail_assert "enriched page also carries seaportal fields"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "seaportal-only audit accepts cookie authentication"

pt_ok audit --seaportal-report "$SEAPORTAL_REPORT" --cookie session=e2evalue --screenshot=false --json

PAGE_COUNT=$(echo "$PT_OUT" | jq '.pages | length' 2>/dev/null)
if [ "${PAGE_COUNT:-0}" -eq 3 ] 2>/dev/null; then
  pass_assert "cookie-authenticated seaportal report ran without a positional URL"
else
  fail_assert "cookie-authenticated seaportal report ran without a positional URL (pages: $PAGE_COUNT)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "--enrich-all overrides browserRecommended routing"

pt_ok audit --seaportal-report "$SEAPORTAL_REPORT" --enrich-all --json

CLEAN_PAGE=$(echo "$PT_OUT" | jq --arg url "$CLEAN_URL" '.pages[] | select(.url == $url)')
if echo "$CLEAN_PAGE" | jq -e '.screenshot | length > 0' >/dev/null 2>&1; then
  pass_assert "clean.html enriched under --enrich-all"
else
  fail_assert "clean.html enriched under --enrich-all"
fi

end_test
