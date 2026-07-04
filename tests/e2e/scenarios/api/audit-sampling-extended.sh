#!/bin/bash
# audit-sampling-extended.sh — POST /audit template-group sampling.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

SAMPLING_BODY="{\"sitemapUrl\":\"${FIXTURES_URL}/audit-site/sitemap.xml\",\"sampleSize\":2,\"options\":{\"screenshot\":false}}"

# ─────────────────────────────────────────────────────────────────
start_test "sampleSize=2 keeps all non-group pages plus 2 products"

pt_post /audit -d "$SAMPLING_BODY"
assert_ok "sampled sitemap audit"

assert_json_length "$RESULT" '.pages' 9 "9 pages (7 non-group + 2 sampled products)"

PRODUCT_COUNT=$(echo "$RESULT" | jq '[.pages[].url | select(contains("products/"))] | length')
if [ "$PRODUCT_COUNT" -eq 2 ]; then
  pass_assert "exactly 2 products/* pages"
else
  fail_assert "exactly 2 products/* pages (got: $PRODUCT_COUNT)"
fi

for page in index.html broken-assets.html console-errors.html a11y-issues.html clean.html forms.html cookie-echo.html; do
  if echo "$RESULT" | jq -e --arg page "$page" '.pages[] | select(.url | endswith($page))' >/dev/null 2>&1; then
    pass_assert "non-group page $page present"
  else
    fail_assert "non-group page $page present"
  fi
done

assert_json_contains "$RESULT" '.pages[0].url' "index.html" "homepage first in page order"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "sampling is reproducible across runs"

FIRST_SET=$(echo "$RESULT" | jq -S '[.pages[].url] | sort')

pt_post /audit -d "$SAMPLING_BODY"
assert_ok "second sampled audit"
SECOND_SET=$(echo "$RESULT" | jq -S '[.pages[].url] | sort')

if [ "$FIRST_SET" = "$SECOND_SET" ]; then
  pass_assert "identical page URL set across runs"
else
  fail_assert "URL sets differ: $FIRST_SET vs $SECOND_SET"
fi

end_test
