#!/bin/bash
# scrape-basic.sh — pinchtab scrape: HTTP-first crawl, browser-enrichment
# routing, and the preview → expand flow.
#
# Runs against the dedicated scrape-site fixture, which mixes server-rendered
# pages (index/about/guide — HTTP extraction is enough) with one JavaScript-only
# page (app.html — thin over HTTP, whole once rendered). That mix lets one run
# prove both halves of the pipeline: cheap HTTP where it suffices, browser
# rendering only where it is required.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

# scrapesite is a dedicated nginx vhost (see tests/e2e/nginx/default.conf) that
# roots at the scrape-site/ fixture dir. seaportal crawls from the host root, so
# the scrape fixture needs its own host rather than a subdirectory of `fixtures`.
SITE_URL="http://scrapesite/index.html"

# ─────────────────────────────────────────────────────────────────
start_test "scrape crawls the site with HTTP-first extraction and browser routing"

pt_ok scrape "$SITE_URL" --timeout 30 --json

if echo "$PT_OUT" | jq -e '.schemaVersion == "1.0"' >/dev/null 2>&1; then
  pass_assert "report carries schema version"
else
  fail_assert "report carries schema version"
fi

PAGE_COUNT=$(echo "$PT_OUT" | jq '.pages | length' 2>/dev/null)
if [ "${PAGE_COUNT:-0}" -ge 4 ] 2>/dev/null; then
  pass_assert "crawl discovered the linked pages (got: $PAGE_COUNT)"
else
  fail_assert "crawl discovered the linked pages (got: $PAGE_COUNT)"
fi

if echo "$PT_OUT" | jq -e '[.pages[].source] | all(. == "http" or . == "browser")' >/dev/null 2>&1; then
  pass_assert "every page records its content source"
else
  fail_assert "every page records its content source"
fi

if echo "$PT_OUT" | jq -e '[.pages[] | select(.error == null)] | all(.markdown | length > 0)' >/dev/null 2>&1; then
  pass_assert "every loaded page has markdown content"
else
  fail_assert "every loaded page has markdown content"
fi

if echo "$PT_OUT" | jq -e '.pageGroups | length > 0' >/dev/null 2>&1; then
  pass_assert "report carries the page tree"
else
  fail_assert "report carries the page tree"
fi

# The JS-only page is thin over HTTP and must be recovered by the browser.
if echo "$PT_OUT" | jq -e '.summary.browserPages >= 1' >/dev/null 2>&1; then
  pass_assert "at least one page was browser-rendered"
else
  fail_assert "at least one page was browser-rendered"
fi

if echo "$PT_OUT" | jq -e '.pages[] | select(.url | endswith("app.html")) | .source == "browser"' >/dev/null 2>&1; then
  pass_assert "the JS-only page was recovered via the browser"
else
  fail_assert "the JS-only page was recovered via the browser"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "scrape --no-browser records routing verdicts without rendering"

pt_ok scrape "$SITE_URL" --timeout 30 --no-browser --json

if echo "$PT_OUT" | jq -e '[.pages[].source] | all(. == "http")' >/dev/null 2>&1; then
  pass_assert "no page was browser-rendered"
else
  fail_assert "no page was browser-rendered"
fi

if echo "$PT_OUT" | jq -e '.summary.browserPages == 0' >/dev/null 2>&1; then
  pass_assert "summary counts zero browser pages"
else
  fail_assert "summary counts zero browser pages"
fi

# The routing verdict is still recorded even though nothing was rendered.
if echo "$PT_OUT" | jq -e '.pages[] | select(.url | endswith("app.html")) | .browserRecommended == true' >/dev/null 2>&1; then
  pass_assert "the JS-only page is still flagged as needing the browser"
else
  fail_assert "the JS-only page is still flagged as needing the browser"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "scrape --preview outlines pages without full bodies"

pt_ok scrape "$SITE_URL" --timeout 30 --preview --json

if echo "$PT_OUT" | jq -e '.pages | length >= 4' >/dev/null 2>&1; then
  pass_assert "preview discovered the linked pages"
else
  fail_assert "preview discovered the linked pages"
fi

if echo "$PT_OUT" | jq -e '[.pages[].markdown // ""] | all(. == "")' >/dev/null 2>&1; then
  pass_assert "preview withholds full page bodies"
else
  fail_assert "preview withholds full page bodies"
fi

if echo "$PT_OUT" | jq -e '[.pages[] | select(.error == null)] | any(.charCount > 0)' >/dev/null 2>&1; then
  pass_assert "preview reports content size (charCount)"
else
  fail_assert "preview reports content size (charCount)"
fi

if echo "$PT_OUT" | jq -e '.summary.browserPages == 0' >/dev/null 2>&1; then
  pass_assert "preview renders nothing in the browser"
else
  fail_assert "preview renders nothing in the browser"
fi

# The outline still tells the caller which pages would need the browser.
if echo "$PT_OUT" | jq -e '.pages[] | select(.url | endswith("app.html")) | .browserRecommended == true' >/dev/null 2>&1; then
  pass_assert "preview flags the JS-only page as needing the browser"
else
  fail_assert "preview flags the JS-only page as needing the browser"
fi

# Pick the JS-only page to expand next: it proves expand recovers via browser.
EXPAND_URL=$(echo "$PT_OUT" | jq -r '.pages[] | select(.url | endswith("app.html")) | .url' 2>/dev/null | head -1)

end_test

# ─────────────────────────────────────────────────────────────────
start_test "scrape --only expands a chosen URL at full fidelity"

pt_ok scrape "$SITE_URL" --timeout 30 --only "$EXPAND_URL" --json

if echo "$PT_OUT" | jq -e --arg u "$EXPAND_URL" '[.pages[].url] == [$u]' >/dev/null 2>&1; then
  pass_assert "expand scraped exactly the chosen URL"
else
  fail_assert "expand scraped exactly the chosen URL"
fi

if echo "$PT_OUT" | jq -e '.pages[0].source == "browser"' >/dev/null 2>&1; then
  pass_assert "expanded JS-only page was rendered in the browser"
else
  fail_assert "expanded JS-only page was rendered in the browser"
fi

if echo "$PT_OUT" | jq -e '.pages[0].markdown | contains("JavaScript")' >/dev/null 2>&1; then
  pass_assert "expanded page carries the JavaScript-rendered content"
else
  fail_assert "expanded page carries the JavaScript-rendered content"
fi

end_test
