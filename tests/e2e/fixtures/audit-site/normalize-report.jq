# normalize-report.jq — strips the volatile fields from an AuditReport so
# two runs of the same audit compare byte-identical (use with `jq -S`).
#
# Normalized away: the run timestamp, the run input (carries the caller's
# FIXTURES_URL host), all timing metrics, screenshot payloads/paths, console
# timestamps/sources, and network entry fields that vary per run (timings,
# sizes, request ids); network entries and broken assets are sorted by URL
# because subresource completion order is nondeterministic (more so under load).
# Console messages are trimmed of trailing whitespace and interactive-element
# refs are dropped: both vary by capture path / accessibility tree across
# browser providers (chrome vs cloak) while the content itself is stable.
#
# Regenerate the golden report (from a runner shell against the e2e stack):
#   curl -s -H "Authorization: Bearer $E2E_SERVER_TOKEN" -X POST "$E2E_SERVER/audit" \
#     -d '{"sitemapUrl":"'"$FIXTURES_URL"'/audit-site/sitemap.xml","options":{"screenshot":false},"concurrency":1}' \
#     | jq -S -f tests/e2e/fixtures/audit-site/normalize-report.jq \
#     > tests/e2e/fixtures/audit-site/golden-report.json
.generatedAt = "0000-00-00T00:00:00Z"
| .input = {}
| .pages |= map(
    del(.screenshot)
    | .browser.screenshotPath = ""
    | .browser.timingMetrics = {}
    | .browser.consoleLogs = ((.browser.consoleLogs // []) | map({level: .level, message: (.message | sub("\\s+$"; ""))}))
    | .browser.interactiveElements = ((.browser.interactiveElements // []) | map(del(.ref)))
    | .browser.networkRequests = ((.browser.networkRequests // [])
        | map({url: .url, method: .method, status: .status, resourceType: .resourceType})
        | sort_by(.url))
    | .browser.brokenAssets = ((.browser.brokenAssets // []) | sort_by(.url))
  )
