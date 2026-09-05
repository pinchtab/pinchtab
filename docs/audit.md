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
| `--cookie name=value` | | Inject a cookie into an isolated temporary browser instance before the run (repeatable) |
| `--cookies-file <file>` | | Inject cookies into an isolated temporary browser instance from a JSON array of `{name, value, domain, ...}` objects |
| `--profile <name>` | | Run against the instance of this browser profile (cannot be combined with `--cookie` or `--cookies-file`) |

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
summaryScore                     # mean accessibility score of enriched pages;
                                 # broken assets, failed requests and uncaught
                                 # JS errors do not move it
pages[]:
  url, title, statusCode?, error?   # error set when the page could not be
                                 # collected; statusCode is the page's own
                                 # document response
  seaportal?                     # HTTP-extraction summary when ingested
  securityFindings[]?            # ruleId, severity, detail, url
  browser:
    screenshotPath               # relative path under the output dir
    consoleLogs[], jsErrors[], networkRequests[], brokenAssets[]
    interactiveElements[], accessibilityScore
    timingMetrics: ttfbMs, fcpMs, lcpMs, cls, domContentLoadedMs, loadMs
securityFindings[]               # page findings aggregated site-level
recommendations[]
```

A page is **failed** when it could not be collected (`error`) or when its own
document answered 4xx/5xx (`statusCode`). The headline's `failed page(s)` counts
both and the page's status line reads `http 404` rather than `ok`. The page's own
failed document also appears in its `brokenAssets` (resource type `document`), and
stays there for consumers, but the headline's `broken asset(s)` count leaves it out so
one failure is not reported twice. `summaryScore` is unchanged by any of this: it
measures accessibility and deliberately ignores failures, which is exactly why they
are printed beside it.

`jsErrors[]` are uncaught JavaScript exceptions (message, stack, line,
column) — the `GET /errors` channel, separate from `consoleLogs[]`, so a
`console.error` call and a thrown exception never merge. A page that raised
one is never reported `ok`.

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
screenshot pairs, and diffs the data (uncaught JS errors, console error count,
broken assets, accessibility score, load time with a noise threshold). Pages
present on only one side are reported as `added`/`removed`.

Uncaught JS errors (`jsErrors`) and broken assets (`brokenAssets`) are compared
by identity rather than by count, both with the page's own `scheme://host`
masked so the same failure on two base URLs matches: an exception's first
message line, and an asset's `url` plus its HTTP status. A failure on one side
only — or one swapped for another at the same count — is drift; the same
failure on both sides is not, since this gate compares two deploys and does not
judge absolute page health. The other fields, `consoleErrors` included, compare
counts, so substituting one console message for another stays invisible.

A broken asset with no HTTP status — a transport error such as a connection
reset, or a request that never completed inside the collection window — is not
compared. `--fail-on-diff` is a gate on the deployment, and which request a run
happened to lose is a property of the run: counting those made two identical
sites fail roughly one run in five. They are still reported in full by the
audit itself and by `/network`, where a failed load is a real finding; the
exclusion applies only to this comparison.

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

Cookies are injected before the first navigation into an isolated temporary
browser instance. The instance and its cookie jar are discarded when the run
finishes, including if the audit request fails, so no persistent profile is
altered. `--cookie` and `--cookies-file` cannot be combined with `--profile`.
Without injected cookies, `--profile` routes the run at the instance owning
that browser profile (`pinchtab instance start --profile <name>` to launch one).

## Library mode (Go)

`pkg/pinchtabaudit` is the public client for embedding enrichment in Go
programs — see the runnable example in `docs/examples/enrich`:

```go
client := pinchtabaudit.New("http://localhost:9867", token)
page, err := client.EnrichPage(ctx, "https://example.com", nil)
report, err := client.EnrichWithBrowser(ctx,
    pinchtabaudit.AuditInput{SitemapURL: "https://example.com/sitemap.xml"}, nil)
```

The module is pre-1.0. The exported Go types in `pkg/pinchtabaudit` may change name,
shape or JSON tag in any release without notice, and no deprecation window is offered.
Pin a module version if that matters to you. The HTTP API below is the stable contract:
build against it if you need one.

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
