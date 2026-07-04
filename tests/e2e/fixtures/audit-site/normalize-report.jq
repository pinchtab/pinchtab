# normalize-report.jq — strips the volatile fields from an AuditReport so
# two runs of the same audit compare byte-identical (use with `jq -S`).
#
# Normalized away: the run timestamp, the run input (carries the caller's
# FIXTURES_URL host), all timing metrics, screenshot payloads/paths, console
# timestamps/sources, and network entry fields that vary per run (timings,
# sizes, request ids); network entries are sorted by URL because subresource
# completion order is nondeterministic.
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
    | .browser.consoleLogs = ((.browser.consoleLogs // []) | map({level: .level, message: .message}))
    | .browser.networkRequests = ((.browser.networkRequests // [])
        | map({url: .url, method: .method, status: .status, resourceType: .resourceType})
        | sort_by(.url))
  )
