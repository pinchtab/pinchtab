# Site Review Reference

Read this reference when the task asks for a site audit, comparison, or crawl. Use it only for user-authorized sites. Keep reports in a user-approved workspace or temporary path; reports and screenshots may contain sensitive page content.

## Audit

Run a browser-level audit for screenshots, console errors, broken assets, interactive elements, accessibility findings, Core Web Vitals, and rule-based security findings.

```bash
pinchtab audit https://example.com --output-dir ./audit
pinchtab audit https://example.com/sitemap.xml --sitemap --sample-size 2 --output-dir ./audit
pinchtab audit https://example.com --json
pinchtab audit https://example.com --format md --output-dir ./audit
pinchtab audit --seaportal-report results.json
```

The report includes pages that fail to load with an `error` field; the run can still exit successfully. See the product [audit guide](https://pinchtab.com/docs/audit) for report schemas and interpretation.

## Compare

Compare the same pages between two site versions. Changed pairs write annotated images under `diffs/`.

```bash
pinchtab compare https://example.com https://staging.example.com --pages /,pricing --output-dir ./comparison
pinchtab compare https://example.com https://staging.example.com --fail-on-diff
```

Use `--fail-on-diff` only when the user wants differences to fail a CI-style check.

## Scrape

Scrape a site into a page tree of Markdown. The crawler uses HTTP first and renders only thin, blocked, or JavaScript-driven pages when browser routing is available.

```bash
pinchtab scrape https://example.com --output-dir ./scrape
pinchtab scrape https://example.com --format md --output-dir ./scrape
pinchtab scrape https://example.com --json
pinchtab scrape https://example.com --preview
pinchtab scrape https://example.com --only https://example.com/pricing --only https://example.com/docs --output-dir ./scrape
pinchtab scrape https://example.com --no-browser
```

For a large site, begin with `--preview`; it returns titles, sizes, snippets, and routing verdicts without downloading page bodies. Then use `--only` to expand just the pages the user needs. Each page records whether its content came from `http` or `browser`; failed pages retain an `error` field. See the product [scrape guide](https://pinchtab.com/docs/scrape) for report schemas and interpretation.
