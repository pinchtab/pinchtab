#!/bin/bash
# audit-basic.sh — POST /audit multi-page site audit runs.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "url-list run returns a versioned AuditReport"

pt_post /audit -d "{\"urls\":[\"${FIXTURES_URL}/audit-site/index.html\",\"${FIXTURES_URL}/audit-site/clean.html\"]}"
assert_ok "audit two pages"
assert_json_exists "$RESULT" '.schemaVersion' "has schemaVersion"
assert_json_exists "$RESULT" '.generatedAt' "has generatedAt"
assert_json_length "$RESULT" '.pages' 2 "two page entries"
assert_json_contains "$RESULT" '.pages[0].url' "index.html" "entry URL first"
assert_json_exists "$RESULT" '.summaryScore' "has summaryScore"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "sitemap mode discovers and audits >=10 pages"

pt_post /audit -d "{\"sitemapUrl\":\"${FIXTURES_URL}/audit-site/sitemap.xml\",\"options\":{\"screenshot\":false},\"concurrency\":3}"
assert_ok "audit from sitemap"
assert_json_length_gte "$RESULT" '.pages' 10 "at least 10 pages audited"

FAILED=$(echo "$RESULT" | jq '[.pages[] | select(.error)] | length')
if [ "$FAILED" -eq 0 ]; then
  pass_assert "no page entry has an error"
else
  fail_assert "no page entry has an error (got $FAILED: $(echo "$RESULT" | jq -c '[.pages[] | select(.error) | {url, error}]'))"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "concurrency 3 and 1 audit the same URL set"

BODY_URLS="[\"${FIXTURES_URL}/audit-site/index.html\",\"${FIXTURES_URL}/audit-site/clean.html\",\"${FIXTURES_URL}/audit-site/products/p1.html\",\"${FIXTURES_URL}/audit-site/products/p2.html\",\"${FIXTURES_URL}/audit-site/products/p3.html\"]"

pt_post /audit -d "{\"urls\":${BODY_URLS},\"options\":{\"screenshot\":false},\"concurrency\":1}"
assert_ok "audit with concurrency 1"
SET_ONE=$(echo "$RESULT" | jq -S '[.pages[].url] | sort')

pt_post /audit -d "{\"urls\":${BODY_URLS},\"options\":{\"screenshot\":false},\"concurrency\":3}"
assert_ok "audit with concurrency 3"
SET_THREE=$(echo "$RESULT" | jq -S '[.pages[].url] | sort')

if [ "$SET_ONE" = "$SET_THREE" ]; then
  pass_assert "same page URL set for concurrency 1 and 3"
else
  fail_assert "URL sets differ: $SET_ONE vs $SET_THREE"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "run with one unreachable URL completes with error entry"

pt_post /audit -d "{\"urls\":[\"${FIXTURES_URL}/audit-site/clean.html\",\"http://fixtures:9/down.html\"],\"options\":{\"screenshot\":false}}"
assert_ok "mixed run returns 200"
assert_json_length "$RESULT" '.pages' 2 "both pages present"

CLEAN_ERR=$(echo "$RESULT" | jq -r '.pages[0].error // empty')
if [ -z "$CLEAN_ERR" ]; then
  pass_assert "reachable page has no error"
else
  fail_assert "reachable page has no error (got: $CLEAN_ERR)"
fi

DOWN_ERR=$(echo "$RESULT" | jq -r '.pages[1].error // empty')
if [ -n "$DOWN_ERR" ]; then
  pass_assert "unreachable page carries error ($DOWN_ERR)"
else
  fail_assert "unreachable page carries error"
fi

pt_get /health
assert_ok "server healthy after mixed run"

end_test
