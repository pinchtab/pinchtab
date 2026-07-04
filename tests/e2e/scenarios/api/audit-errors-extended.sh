#!/bin/bash
# audit-errors-extended.sh — audit error-path contract:
#   - unreachable host  → HTTP 200, page entry carries `error`, run continues
#   - 404 entry page    → HTTP 200, page audited normally (Chrome renders the
#                         error document; the 404 shows in networkRequests)
#   - empty sitemap     → HTTP 400 "no URLs to audit"
# After every case the server stays healthy and the next audit succeeds.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

assert_healthy_and_recovers() {
  pt_get /health
  assert_ok "/health ok after the case"
  pt_post /audit -d "{\"urls\":[\"${FIXTURES_URL}/audit-site/clean.html\"],\"options\":{\"screenshot\":false}}"
  assert_ok "follow-up audit succeeds"
  assert_json_eq "$RESULT" '.pages[0].error // "none"' "none" "follow-up page has no error"
}

# ─────────────────────────────────────────────────────────────────
start_test "unreachable host produces a structured page error"

pt_post /audit -d "{\"urls\":[\"http://fixtures:9/down.html\"],\"options\":{\"screenshot\":false}}"
assert_ok "audit run returns 200"
ERR=$(echo "$RESULT" | jq -r '.pages[0].error // empty')
if [ -n "$ERR" ]; then
  pass_assert "page entry carries error ($ERR)"
else
  fail_assert "page entry carries error"
fi

assert_healthy_and_recovers

end_test

# ─────────────────────────────────────────────────────────────────
start_test "404 entry page is audited as a page, not a failure"

pt_post /audit -d "{\"urls\":[\"${FIXTURES_URL}/audit-site/no-such-page.html\"],\"options\":{\"screenshot\":false}}"
assert_ok "audit run returns 200"
assert_json_length "$RESULT" '.pages' 1 "one page entry"

ERR=$(echo "$RESULT" | jq -r '.pages[0].error // empty')
if [ -z "$ERR" ]; then
  pass_assert "no error field (error page rendered and audited)"
else
  fail_assert "no error field (got: $ERR)"
fi

if echo "$RESULT" | jq -e '.pages[0].browser.networkRequests[]? | select(.url | endswith("no-such-page.html")) | select(.status == 404)' >/dev/null 2>&1; then
  pass_assert "document 404 visible in networkRequests"
else
  fail_assert "document 404 visible in networkRequests (got: $(echo "$RESULT" | jq -c '[.pages[0].browser.networkRequests[]? | {url, status}]'))"
fi

assert_healthy_and_recovers

end_test

# ─────────────────────────────────────────────────────────────────
start_test "empty sitemap fails cleanly with no URLs to audit"

STATUS=$(e2e_curl -s -o /tmp/audit-empty-sitemap.json -w '%{http_code}' -X POST "${E2E_SERVER}/audit" \
  -d "{\"sitemapUrl\":\"${FIXTURES_URL}/audit-site/empty-sitemap.xml\"}")
if [ "$STATUS" = "400" ]; then
  pass_assert "empty sitemap returns HTTP 400"
else
  fail_assert "empty sitemap returns HTTP 400 (got: $STATUS)"
fi

if grep -q "no URLs to audit" /tmp/audit-empty-sitemap.json; then
  pass_assert "error explains there is nothing to audit"
else
  fail_assert "error explains there is nothing to audit (got: $(cat /tmp/audit-empty-sitemap.json))"
fi

assert_healthy_and_recovers

end_test
