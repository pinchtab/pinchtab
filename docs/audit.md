# Site Audit & Compare

`pinchtab audit` runs a browser-level audit of one or many pages and produces a
versioned report: screenshots, console logs, network requests and broken
assets, interactive elements, an accessibility score with findings, Core Web
Vitals timing, and rule-based security findings. `pinchtab compare` audits the
same pages on two site versions (live vs staging) and reports per-page visual
and data differences — usable as a CI gate.

The pipeline behind both commands:

```
seaportal (HTTP discovery & extraction)  →  pinchtab (browser enrichment)  →  report
   sitemap flattening, page profiles,        screenshots, console, network,     json / md /
   browserRecommended routing                a11y, timing, security rules       html / pdf
```

SeaPortal does the cheap HTTP work; the browser — the expensive resource —
runs only where it adds signal. Reports are deterministic: the same input
yields the same report (the e2e suite enforces this against a golden file).

## Quick start

```bash
pinchtab audit https://example.com --output-dir ./audit
# → ./audit/report.json + ./audit/screenshots/*.png
```

Audit a whole site from its sitemap, bounded per page template:

```bash
pinchtab audit https://example.com/sitemap.xml --sitemap --sample-size 2 --output-dir ./audit
```

## `pinchtab audit`

```
pinchtab audit [url] [flags]
```

The argument is a page URL, or a sitemap URL with `--sitemap`. With
`--seaportal-report` the pages come from a SeaPortal results file instead and
no URL argument is needed.

| Flag | Default | Meaning |
|---|---|---|
| `--sitemap` | false | Treat the URL as a sitemap.xml and audit the discovered pages |
| `--seaportal-report <file>` | | Audit pages from a SeaPortal results JSON file (array of `Result` objects) |
| `--enrich-all` | false | Browser-enrich every seaportal page, ignoring `browserRecommended` routing |
| `--sample-size <n>` | 0 | Pages audited per template group, e.g. `/products/p1..pN` (0 = all pages; deterministic picks) |
| `--screenshot` | true | Capture page screenshots |
| `--network-monitor` | true | Collect network requests and broken assets |
| `--concurrency <n>` | 2 | Pages audited in parallel (max 8) |
| `--output-dir <dir>` | | Write `report.json` and `screenshots/` to this directory |
| `--format <f>` | json | Report format: `json`, `md`, `html`, or `pdf` |
| `--json` | false | Print the full report JSON to stdout |
| `--cookie name=value` | | Inject a cookie before the run (repeatable; the cookie jar is cleared afterwards) |
| `--cookies-file <file>` | | Inject cookies from a JSON array of `{name, value, domain, ...}` objects |
| `--profile <name>` | | Run against the instance of this browser profile |

Failure contract: a page that fails to load does **not** fail the run — the
command exits 0 and that page's report entry carries an `error` field. The run
itself only errors when there is nothing to audit (for example an empty
sitemap).

### Inputs and routing

- **URL** — audits that one page.
- **`--sitemap`** — pages are discovered through seaportal's sitemap
  flattening (recursive sitemap indexes supported), then deduplicated,
  grouped by URL template, and sampled (`--sample-size`). The entry URL is
  always audited first; template pages are audited after ungrouped pages.
- **`--seaportal-report`** — the file is a JSON array of seaportal `Result`
  objects (the interim `seaportal-results/v0` format, versioned in the
  report's `input.seaportalFormat`). Only pages seaportal marked
  `profile.browserRecommended` are browser-enriched; the rest keep their
  HTTP-extraction summary in the report. `--enrich-all` overrides.

### Report anatomy

`report.json` is a versioned `AuditReport` (`schemaVersion`):

```
schemaVersion, generatedAt, input, options
summaryScore                     # mean accessibility score of enriched pages
pages[]:
  url, title, error?             # error set when the page failed to load
  seaportal?                     # HTTP-extraction summary when ingested
  securityFindings[]?            # ruleId, severity, detail, url
  browser:
    screenshotPath               # relative path under the output dir
    consoleLogs[], networkRequests[], brokenAssets[]
    interactiveElements[], accessibilityScore
    timingMetrics: ttfbMs, fcpMs, lcpMs, cls, domContentLoadedMs, loadMs
securityFindings[]               # page findings aggregated site-level
recommendations[]
```

`--format md` / `--format html` write `report.md` / `report.html` next to
`report.json` (or print to stdout without `--output-dir`). The HTML is
self-contained — inline CSS, relative screenshot links. `--format pdf` prints
the HTML report through the browser; it needs `--output-dir` and the
`evaluate` capability, and on a print failure `report.json` is still written,
a warning is surfaced, and the exit code is non-zero.

Security findings are rule-based and computed offline from collected data:
mixed content, insecure form actions, password forms posting over http,
exposed sensitive paths (`.env`, `.git`, …), and directory-listing pages.

## `pinchtab compare`

```
pinchtab compare <live-url> <staging-url> [flags]
```

Audits the same pages on both base URLs, pairs them by path, pixel-diffs the
screenshot pairs, and diffs the data (console error count, broken assets,
accessibility score, load time with a noise threshold). Pages present on only
one side are reported as `added`/`removed`.

| Flag | Default | Meaning |
|---|---|---|
| `--pages <p1,p2>` | the base URLs | Comma-separated relative paths to compare |
| `--visual-diff` | true | Capture screenshots and compute visual diffs |
| `--concurrency <n>` | 2 | Pages audited in parallel per side (max 8) |
| `--output-dir <dir>` | | Write `report.json` and `diffs/` to this directory |
| `--format <f>` | json | Report format: `json`, `md`, or `html` |
| `--json` | false | Print the comparison report JSON to stdout |
| `--fail-on-diff` | false | Exit non-zero when any visual or data diff exists |
| `--cookie`, `--cookies-file`, `--profile` | | Same auth flags as `audit` |

Differences are data, not failure: without `--fail-on-diff` the command exits
0 even when pages differ. Changed pairs get an annotated diff image under
`diffs/`, referenced by the page's `diffImagePath`.

## Authentication

```bash
pinchtab audit https://example.com/account --cookie session=abc123
pinchtab audit https://example.com/account --cookies-file cookies.json
pinchtab audit https://example.com --profile work
```

Cookies are injected before the first navigation and scoped to the run: the
browser cookie jar is cleared when the run finishes, so a later audit does
not inherit them. `--profile` routes the run at the instance owning that
browser profile (`pinchtab instance start --profile <name>` to launch one).

## Library mode (Go)

`pkg/pinchtabaudit` is the public client for embedding enrichment in Go
programs — see the runnable example in `docs/examples/enrich`:

```go
client := pinchtabaudit.New("http://localhost:9867", token)
page, err := client.EnrichPage(ctx, "https://example.com", nil)
report, err := client.EnrichWithBrowser(ctx,
    pinchtabaudit.AuditInput{SitemapURL: "https://example.com/sitemap.xml"}, nil)
```

## HTTP API

The CLI is a thin client over two endpoints:

- `POST /audit/page {"url", "options"}` → single-page `BrowserPageData`
- `POST /audit {"urls" | "sitemapUrl" | "seaportalResults", "options",
  "concurrency", "sampleSize", "enrichAll"}` → `AuditReport`

## Docker / CI

The audit CLI is a thin client over a running pinchtab server, so in Docker
run the server container first and exec the audit against it (see
[guides/docker.md](guides/docker.md) for the server setup):

```bash
docker run -d --name pinchtab -v pinchtab-data:/data --shm-size=2g pinchtab/pinchtab
docker exec pinchtab pinchtab audit https://example.com --output-dir /data/audit
docker cp pinchtab:/data/audit ./audit
```

Gate a deploy on staging matching production — `--fail-on-diff` exits 0 when
the sites match and non-zero when any visual or data diff exists (both exit
codes are exercised by the e2e suite in the compose stack):

```bash
pinchtab compare https://example.com https://staging.example.com \
  --pages /,pricing,docs --fail-on-diff --output-dir ./compare-artifacts
# exit 0 → ship; exit 1 → inspect ./compare-artifacts/diffs/
```

## Not yet shipped

- `--llm-refine` (LLM post-processing of reports) — pending decision.
- External scanner integration (e.g. Nuclei) for deeper security checks —
  pending decision; today's security findings are the built-in rules above.
