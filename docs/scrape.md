# Site Scrape

`pinchtab scrape` turns a whole site into a page tree of markdown content.
Pages are discovered and extracted over plain HTTP first (via SeaPortal: sitemap
or link crawl, URL-pattern sampling). Only pages whose HTTP extraction comes back
thin, blocked, or failed are re-rendered in the real browser and re-extracted
from the rendered DOM — so JavaScript-only content still lands in the report,
while static pages never pay for a browser.

The pipeline:

```
seaportal (HTTP crawl & extraction)  →  routing  →  pinchtab (browser)  →  report
   sitemap/link crawl, URL-pattern       thin /        render + re-extract   json / md
   sampling, per-page markdown           blocked?       JS-only pages         page tree
```

SeaPortal does the cheap HTTP work; the browser — the expensive resource — runs
only where HTTP was not enough. Every page records where its content came from
(`http` or `browser`) and the routing verdict that put it there.

## Quick start

```bash
pinchtab scrape https://example.com --output-dir ./scrape
# → ./scrape/report.json
```

Also write a human-readable digest and print a summary:

```bash
pinchtab scrape https://example.com --format md --output-dir ./scrape
# → ./scrape/report.json + ./scrape/report.md
```

## Preview → Expand (large sites)

Scraping a large site at full fidelity is expensive twice over: shipping every
page's markdown dominates the token cost, and browser-rendering every routed
page dominates the wall-clock cost. To survey first and drill down second, run
the scrape in two cheap-then-deep passes.

**1. Preview** — the HTTP crawl and per-page routing verdict, but no browser
rendering and no full page bodies. Each page's markdown is withheld and replaced
by a `charCount` (how heavy a full expand would be) and a leading `snippet`:

```bash
pinchtab scrape https://example.com --preview
#   https://example.com/            · 4229 chars
#       Browser control for AI agents. 12MB Go binary…
#   https://example.com/docs/app    · 87 chars · needs browser: thin-content
#       Loading…
```

**2. Expand** — scrape exactly the URLs you picked, at full fidelity (HTTP
extract + browser render per the normal routing), instead of re-crawling:

```bash
pinchtab scrape https://example.com \
  --only https://example.com/docs/app \
  --only https://example.com/pricing \
  --output-dir ./scrape
```

Expand is stateless: hand back the exact URLs the preview showed. No server-side
session or cache is kept.

## `pinchtab scrape`

```
pinchtab scrape <url> [flags]
```

The argument is the site URL. Discovery seeds from that host's root: SeaPortal
looks for `robots.txt` and `sitemap.xml` at the host root and, failing that,
link-crawls from the root page.

| Flag | Default | Meaning |
|---|---|---|
| `--preview` | false | Outline only: page tree, `charCount`, and `snippet` per page — no browser rendering or full bodies |
| `--only <url>` | | Expand exactly these URLs at full fidelity instead of crawling (repeatable) |
| `--max-pages <n>` | 50 | Maximum pages sampled across the whole site |
| `--max-per-pattern <n>` | 8 | Maximum pages sampled per URL-pattern group (e.g. `/blog/*`) |
| `--include <regex>` | | Only crawl URLs matching this regex (repeatable) |
| `--exclude <regex>` | | Skip URLs matching this regex (repeatable) |
| `--enrich-all` | false | Browser-render every reachable page, ignoring routing |
| `--no-browser` | false | HTTP crawl only; record routing verdicts without browser rendering |
| `--concurrency <n>` | 2 | Pages browser-rendered in parallel (max 8) |
| `--timeout <s>` | 60 | Overall HTTP crawl timeout in seconds |
| `--output-dir <dir>` | | Write `report.json` (and `report.md` with `--format md`) to this directory |
| `--format <f>` | json | Report format: `json` or `md` |
| `--json` | false | Print the full report JSON to stdout |
| `--cookie name=value` | | Inject a cookie into an isolated temporary browser instance before the run (repeatable) |
| `--cookies-file <file>` | | Inject cookies into an isolated temporary browser instance from a JSON array of `{name, value, domain, ...}` objects |
| `--profile <name>` | | Run against the instance of this browser profile (cannot be combined with `--cookie` or `--cookies-file`) |

Failure contract: a page that fails in **both** engines does not fail the run —
the command exits 0 and that page's report entry carries an `error` field. A
failed browser enrichment keeps the HTTP extraction and records `browserError`.
The run itself only errors when the crawl fails outright or discovers no pages.

### Routing

Each HTTP-extracted page is scored for whether the browser should re-render it:

- **not-found** (404 / 410) never routes — re-rendering cannot recover content.
- **fetch-error** or a **blocked status** (401, 403, 407, 429, 503) routes: a
  real browser with stealth and challenge handling may succeed where a plain
  HTTP client was refused.
- **thin-content** (extraction shorter than the static-ok threshold) routes: a
  probable JavaScript shell.

`--enrich-all` forces every non-404 page to the browser; `--no-browser` records
the verdict on each page but renders nothing.

## Report anatomy

`report.json` is a versioned scrape report (`schemaVersion`):

```
schemaVersion, generatedAt, input
site:      baseUrl, title, sitemapFound, totalDiscovered, sampledPages
pageGroups[]:  pattern, total, sampled, urls[]        # site tree by URL pattern
pages[]:
  url, title, statusCode, contentType
  markdown                     # withheld in preview mode
  charCount, snippet           # set in preview mode instead of markdown
  meta, schema, internalLinks, externalLinks
  source                       # "http" or "browser"
  browserRecommended, browserReasons   # the routing verdict
  browserError?                # browser enrichment failed; HTTP content kept
  error?                       # page failed in both engines
summary:   contentTypes, httpPages, browserPages, failedPages, recommendations
```

`--format md` writes `report.md` next to `report.json` (or prints to stdout
without `--output-dir`): a single digest with the site overview, the page tree
by URL pattern, and each page's content (or its snippet in preview mode).

## HTTP API

The CLI is a thin client over one endpoint:

- `POST /scrape {"url", "preview", "only", "maxPages", "maxPerPattern",
	"includePatterns", "excludePatterns", "concurrency", "enrichAll",
	"noBrowser", "timeoutSeconds", "browser"}` → the scrape report

Every crawl fetch and every expand fetch goes through the same SSRF/redirect
navigation guard as browser navigation.

## MCP

Agents can drive the same flow over MCP with the `pinchtab_scrape` tool. Prefer
`preview=true` for a cheap outline, then expand with `only` (comma-separated
URLs) — the full report can be large. See [MCP Server](mcp.md).

```
pinchtab_scrape { "url": "https://example.com", "preview": true }
pinchtab_scrape { "url": "https://example.com", "only": "https://example.com/a, https://example.com/b" }
```

## Docker / CI

The scrape CLI is a thin client over a running pinchtab server, so in Docker run
the server container first and exec the scrape against it (see
[guides/docker.md](guides/docker.md) for the server setup):

```bash
docker run -d --name pinchtab -v pinchtab-data:/data --shm-size=2g pinchtab/pinchtab
docker exec pinchtab pinchtab scrape https://example.com --output-dir /data/scrape
docker cp pinchtab:/data/scrape ./scrape
```
