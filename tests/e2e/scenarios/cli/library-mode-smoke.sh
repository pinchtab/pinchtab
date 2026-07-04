#!/bin/bash
# library-mode-smoke.sh — pkg/pinchtabaudit library client end-to-end.
#
# The example program (docs/examples/enrich) is compiled into the runner-cli
# image as `enrich-example`; this smoke drives it against the e2e service.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# ─────────────────────────────────────────────────────────────────
start_test "library example enriches a fixture page"

if ! command -v enrich-example >/dev/null 2>&1; then
  fail_assert "enrich-example binary present in runner image"
  end_test
  exit 0
fi
pass_assert "enrich-example binary present"

OUT=$(enrich-example --server "$E2E_SERVER" --token "$E2E_SERVER_TOKEN" "http://fixtures/audit-site/clean.html" 2>/tmp/enrich-err)
STATUS=$?
if [ "$STATUS" -eq 0 ]; then
  pass_assert "example exits 0"
else
  fail_assert "example exits 0 (got $STATUS, stderr: $(cat /tmp/enrich-err))"
fi

if echo "$OUT" | jq -e '.url == "http://fixtures/audit-site/clean.html"' >/dev/null 2>&1; then
  pass_assert "output is valid JSON with the page url"
else
  fail_assert "output is valid JSON with the page url"
fi

if echo "$OUT" | jq -e '.accessibilityScore == 100' >/dev/null 2>&1; then
  pass_assert "accessibilityScore present (100 for clean.html)"
else
  fail_assert "accessibilityScore present (got: $(echo "$OUT" | jq '.accessibilityScore'))"
fi

if echo "$OUT" | jq -e '.timingMetrics.loadMs > 0' >/dev/null 2>&1; then
  pass_assert "timingMetrics.loadMs > 0"
else
  fail_assert "timingMetrics.loadMs > 0"
fi

if echo "$OUT" | jq -e '.interactiveElements | length >= 1' >/dev/null 2>&1; then
  pass_assert "interactiveElements populated"
else
  fail_assert "interactiveElements populated"
fi

end_test
