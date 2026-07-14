# PinchTab Scraping & Audit Spec

## Overview

PinchTab serves as the **deep browser enhancement layer** on top of SeaPortal's discovery and HTTP extraction. It adds visual, interactive, performance, and security capabilities that pure HTTP scraping cannot provide.

## Core Command

```bash
pinchtab audit <url> [flags]
pinchtab compare <live-url> <staging-url> [flags]
```

## Responsibilities

1. **Consume SeaPortal output** as base data
2. **Enrich sampled pages** with browser-specific data
3. **Run visual regression** (when comparing versions)
4. **Generate final rich report** (or send structured data to LLM)

## Input

- SeaPortal `SiteReport` JSON (or direct URL + sitemap data)
- Optional: authentication details (cookies, login flow)
- Comparison baseline (for diff mode)

## Output

Enhanced `SiteReport` with browser-enriched fields per page.

## Page Enhancement Fields (PinchTab)

```go
type BrowserPageData struct {
    ScreenshotPath      string
    FullPageScreenshot  bool
    ConsoleLogs         []ConsoleLogEntry
    NetworkRequests     []NetworkRequest
    BrokenAssets        []BrokenAsset      // especially 404 images
    InteractiveElements []InteractiveElement
    AccessibilityScore  int
    VisualDiff          *VisualDiffResult   // only in compare mode
    TimingMetrics       BrowserTimingMetrics
}
```

## Key Features

### 1. Visual Analysis
- Full-page screenshots
- Image diffing (for compare mode)
- Highlighted diff images with annotations

### 2. Console & JS Monitoring
- Capture all console logs, errors, warnings
- Filter critical JS errors

### 3. Network Monitoring
- Track all resource requests
- Detect broken images, scripts, stylesheets (404, 500, etc.)
- Monitor API failures

### 4. Performance (Browser-level)
- Core Web Vitals (via CDP)
- Navigation timing
- Resource loading breakdown

### 5. Usability & Accessibility
- Basic a11y checks
- Missing form labels, alt texts
- Navigation flow validation

### 6. Security Surface (optional)
- Integration point for Nuclei or similar
- Exposed endpoints detection
- Mixed content warnings

## Sampling & Scalability

- Respect SeaPortal's page groups and samples
- Allow overriding sample size
- Parallel processing with configurable concurrency
- Smart prioritization (homepage, key flows first)

### Preview → Expand (large sites)

Full-fidelity scraping of a large site is expensive twice over: shipping every
page's markdown body dominates the token cost, and browser-rendering every
routed page dominates the wall-clock cost. To let a model survey first and
drill down second, the scrape runs in two cheap-then-deep passes:

1. **Preview** (`scrape <url> --preview`, `preview: true`): the HTTP crawl and
   per-page routing verdict, but **no browser rendering** and **no full page
   bodies**. Each page's markdown is withheld and replaced by:
   - `charCount` — size of the extracted content, so the caller can gauge how
     heavy a full expand would be
   - `snippet` — a leading, whitespace-collapsed slice of the content
   - the page's existing `meta`/description and `contentType`

   The result is an outline the model reads cheaply to decide what matters.

2. **Expand** (`scrape <url> --only <url> [--only <url> …]`, `only: [...]`):
   scrapes **exactly the chosen URLs** at full fidelity (HTTP extract + browser
   render per the normal routing) instead of re-discovering the site. It is
   stateless — the model hands back the exact URLs it saw in the preview; no
   server-side session or cache is kept.

Both passes reuse the same navigation security stack: every preview crawl fetch
and every expand fetch (`FetchBytes` with the `CrawlGuard` policy) goes through
the same SSRF/redirect vetting as browser navigation.

## Report Structure

(See previous combined report templates with sections for:)
- Summary Score
- SEO & Metadata
- Content & Functionality
- Visual Differences
- Performance
- Console & JS Errors
- Broken Assets / Images
- Usability Issues
- Security Findings
- Recommendations

## Integration Points

- Library mode: `pinchtab.EnrichWithBrowser(seaPortalReport)`
- CLI for standalone use
- Docker-friendly for CI/CD
- Output formats: JSON (for LLM), Markdown/HTML report, PDF

## Authentication Support

- Cookie injection
- Login flow recording/replay
- Headless + stealth options

## Flags (examples)

- `--sample-size int`
- `--screenshot`
- `--network-monitor`
- `--visual-diff`
- `--llm-refine`
- `--output-dir`
- `--concurrency int`

---

**Status**: Ready for implementation planning.

This complements the SeaPortal spec perfectly.
