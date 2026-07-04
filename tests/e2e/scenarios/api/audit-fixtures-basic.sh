#!/bin/bash
# audit-fixtures-basic.sh — Integrity checks for the deterministic audit
# fixture site (audit-site + audit-site-staging), served by the fixtures
# nginx container. Pure HTTP checks against the fixtures server; no
# pinchtab API calls.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

FIXTURES_DIR="${GROUP_DIR}/../../fixtures"

http_status() {
  curl -s -o /dev/null -w '%{http_code}' "$1"
}

assert_status() {
  local url="$1"
  local expected="$2"
  local desc="${3:-GET ${url#"${FIXTURES_URL}"} → ${expected}}"
  local actual
  actual=$(http_status "$url")
  if [ "$actual" = "$expected" ]; then
    pass_assert "$desc"
  else
    fail_assert "$desc (got: $actual)"
  fi
}

AUDIT_PAGES=(
  index.html
  broken-assets.html
  console-errors.html
  a11y-issues.html
  clean.html
  forms.html
  cookie-echo.html
  products/p1.html
  products/p2.html
  products/p3.html
  products/p4.html
  products/p5.html
  sitemap.xml
)

# ─────────────────────────────────────────────────────────────────
start_test "every audit-site page and sitemap.xml return 200"

for page in "${AUDIT_PAGES[@]}"; do
  assert_status "${FIXTURES_URL}/audit-site/${page}" 200
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "broken-assets.html missing references all 404"

for asset in missing.css missing.js missing.png missing.json; do
  assert_status "${FIXTURES_URL}/audit-site/assets/${asset}" 404
done

# Quoted occurrences only: the actual href/src/fetch references, not the
# file's documentation comment.
MISSING_REFS=$(grep -cE "['\"]assets/missing\." "${FIXTURES_DIR}/audit-site/broken-assets.html")
if [ "$MISSING_REFS" -eq 4 ]; then
  pass_assert "broken-assets.html references exactly 3 assets + 1 fetch target"
else
  fail_assert "broken-assets.html missing-reference count (got: $MISSING_REFS, want 4)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "staging clean.html is byte-identical, index.html differs"

LIVE_TMP=$(mktemp /tmp/audit-fixture-live.XXXXXX)
STAGING_TMP=$(mktemp /tmp/audit-fixture-staging.XXXXXX)

curl -s -o "$LIVE_TMP" "${FIXTURES_URL}/audit-site/clean.html"
curl -s -o "$STAGING_TMP" "${FIXTURES_URL}/audit-site-staging/clean.html"
if cmp -s "$LIVE_TMP" "$STAGING_TMP"; then
  pass_assert "clean.html byte-identical between live and staging"
else
  fail_assert "clean.html differs between live and staging"
fi

curl -s -o "$LIVE_TMP" "${FIXTURES_URL}/audit-site/index.html"
curl -s -o "$STAGING_TMP" "${FIXTURES_URL}/audit-site-staging/index.html"
if cmp -s "$LIVE_TMP" "$STAGING_TMP"; then
  fail_assert "index.html should differ between live and staging"
else
  pass_assert "index.html differs between live and staging"
fi

rm -f "$LIVE_TMP" "$STAGING_TMP"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "sitemap lists >=10 URLs incl products, all resolve 200"

SITEMAP=$(curl -s "${FIXTURES_URL}/audit-site/sitemap.xml")
LOCS=$(echo "$SITEMAP" | grep -o '<loc>[^<]*</loc>' | sed -e 's|<loc>||' -e 's|</loc>||')

LOC_COUNT=$(echo "$LOCS" | grep -c .)
if [ "$LOC_COUNT" -ge 10 ]; then
  pass_assert "sitemap lists $LOC_COUNT URLs (>= 10)"
else
  fail_assert "sitemap lists $LOC_COUNT URLs (want >= 10)"
fi

PRODUCT_COUNT=$(echo "$LOCS" | grep -c 'products/p')
if [ "$PRODUCT_COUNT" -eq 5 ]; then
  pass_assert "sitemap lists all 5 products/* pages"
else
  fail_assert "sitemap products/* count (got: $PRODUCT_COUNT, want 5)"
fi

while IFS= read -r loc; do
  [ -z "$loc" ] && continue
  path=$(echo "$loc" | sed -E 's|^https?://[^/]+||')
  assert_status "${FIXTURES_URL}${path}" 200 "sitemap URL ${path} → 200"
done <<< "$LOCS"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "no fixture references an external domain"

# Only the compose-internal fixtures host and the standard (never fetched)
# sitemap namespace identifier are allowed to appear as absolute URLs.
EXTERNAL_REFS=$(grep -rEoh 'https?://[^"'"'"'<> ]+' \
  "${FIXTURES_DIR}/audit-site" "${FIXTURES_DIR}/audit-site-staging" \
  | grep -vE '^http://fixtures([/:]|$)' \
  | grep -vF 'http://www.sitemaps.org/schemas/sitemap/0.9' || true)

if [ -z "$EXTERNAL_REFS" ]; then
  pass_assert "no external domain references in audit fixtures"
else
  fail_assert "external references found: $(echo "$EXTERNAL_REFS" | tr '\n' ' ')"
fi

end_test
