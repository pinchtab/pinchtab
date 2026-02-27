# web_fetch: Lightweight Extreme Analysis

## Overview

This document benchmarks **web_fetch** (text-only extraction) as the lightweight baseline against semantic snapshots and Pinchtab. web_fetch uses Readability parser to extract main content as plain text/markdown, removing navigation, ads, and structural metadata.

**Executive Summary:**
- web_fetch: **~3K-12K tokens** (text content only, minimal structure)
- Snapshot: **~11K-95K tokens** (DOM + accessibility tree)
- Pinchtab: **~800-1K tokens** (optimized text extraction)
- **Comparison: 11-29x lighter than snapshots, 2-10x heavier than Pinchtab**

---

## Test Results

### Methodology

For each site (BBC, Corriere, Daily Mail):
1. Used `web_fetch` tool with `extractMode="markdown"`
2. Extracted content via Readability parser (removes nav/ads/chrome)
3. Counted total response size in KB
4. Estimated tokens: **4 characters ≈ 1 token** (text-only)
5. Extracted first 20 article headlines from cleaned text

### Data

| Site | Extract Size | Est. Tokens | Articles Found | Cost (Sonnet) |
|------|--------------|------------|-----------------|---------------|
| **BBC** | 18.8 KB | ~4,700 | 15-20 | $0.000014 |
| **Corriere** | 13.1 KB | ~3,275 | 15-20 | $0.000010 |
| **Daily Mail** | 50 KB | ~12,500 | 20+ | $0.000038 |
| **Average** | **27.3 KB** | **~6,825** | **~19** | **$0.000021** |

---

## Comparative Analysis

### Method 1: web_fetch (Text-Only)

**What it captures:**
- Main article/content text (Readability extraction)
- Markdown-formatted structure (headers, lists, links)
- NO DOM structure
- NO accessibility metadata
- NO interactive element references

**How it works:**
1. Fetches raw HTML
2. Applies Readability algorithm (removes boilerplate)
3. Converts to plain text or markdown
4. Returns cleaned content

**Token cost per site:**
- BBC: **4,700 tokens** ($0.000014)
- Corriere: **3,275 tokens** ($0.000010)
- Daily Mail: **12,500 tokens** ($0.000038)
- **Average: 6,825 tokens** ($0.000021 per page)

**Pros:**
- ✅ **82% lighter than snapshots** (29x smaller on Corriere)
- ✅ Fast (no rendering required)
- ✅ No Chrome/JavaScript overhead
- ✅ Readability parser removes ads/nav automatically
- ✅ Good for content extraction tasks

**Cons:**
- ❌ **No interactivity** (can't click, fill forms)
- ❌ **Loses structure** (no sections, hierarchy)
- ❌ **Can't extract forms or inputs**
- ❌ **Fails on JavaScript-rendered content**
- ❌ Still 2-10x heavier than Pinchtab

---

### Comparison: web_fetch vs. Snapshot

| Aspect | web_fetch | Snapshot | Difference |
|--------|-----------|----------|-----------|
| **Size** | 27.3 KB | 258 KB | 10.4x heavier (snapshot) |
| **Tokens** | 6,825 | 64,583 | 9.5x heavier (snapshot) |
| **Token Cost** | $0.000021 | $0.000194 | 9.2x more expensive (snapshot) |
| **Interactivity** | ❌ No | ✅ Yes | Snapshot wins |
| **Structure Info** | ❌ No | ✅ Yes | Snapshot wins |
| **Speed** | ✅ Fast | ⚠️ Slow | web_fetch wins |
| **Rendering** | ❌ No | ✅ Yes | Snapshot wins |
| **Content Quality** | ✅ Clean | ⚠️ Noisy | web_fetch wins |

### Comparison: web_fetch vs. Pinchtab

| Aspect | web_fetch | Pinchtab | Winner |
|--------|-----------|----------|--------|
| **Tokens** | 6,825 | ~1,200 | Pinchtab (5.7x lighter) |
| **Cost** | $0.000021 | $0.0000036 | Pinchtab (83% cheaper) |
| **Rendering** | ❌ No | ✅ Real Chrome | Pinchtab |
| **Selectors** | ❌ No | ✅ CSS filtering | Pinchtab |
| **Setup** | ✅ Built-in | Requires binary | web_fetch |
| **Performance** | ✅ Instant | ⚠️ Chrome startup | web_fetch |

---

## Cost Implications

### Per-Page Economics

| Method | Tokens | Cost (Sonnet) | Monthly (1K pages) |
|--------|--------|---------------|--------------------|
| **Snapshot** | ~64,583 | $0.000194 | $5.82 |
| **web_fetch** | ~6,825 | $0.000021 | $0.63 |
| **Pinchtab** | ~1,200 | $0.0000036 | $0.11 |

### Savings Tiers

**web_fetch vs. Snapshot:**
- Per page: saves $0.000173
- 1K pages/day: saves $0.173/day = **$5.19/month**
- 10K pages/day: saves $1.73/day = **$51.90/month**

**Pinchtab vs. web_fetch:**
- Per page: saves $0.000017
- 1K pages/day: saves $0.017/day = **$0.51/month**
- 10K pages/day: saves $0.17/day = **$5.10/month**

---

## Use Case Matrix

### When to Use web_fetch

| Scenario | Recommended? | Reason |
|----------|-------------|--------|
| Extract news article text | ✅ Yes | Perfect — fast, clean, cheap |
| Build search index | ✅ Yes | Text-only ideal for indexing |
| Content aggregation | ✅ Yes | Multiple sites, speed matters |
| Blog post extraction | ✅ Yes | Main content + meta-data |
| **Click/interact with page** | ❌ No | Can't do it |
| **Fill out form** | ❌ No | No form handling |
| **JavaScript-heavy site** | ❌ No | Won't render JS |
| **Coordinate-based clicking** | ❌ No | No element positions |
| **Need full page structure** | ❌ No | Loses all hierarchy |

### When to Use Pinchtab Instead

| Scenario | Recommended? | Reason |
|----------|-------------|--------|
| **Text + Real Chrome rendering** | ✅ Yes | Handles JS, still fast |
| **Selector-based extraction** | ✅ Yes | Target specific elements |
| **Token efficiency critical** | ✅ Yes | 5.7x lighter than web_fetch |
| **Agent workflow at scale** | ✅ Yes | Cost savings compound |
| **Quick text-only (no JS)** | ⚠️ Maybe | web_fetch is simpler |
| **No infra overhead** | ⚠️ Maybe | web_fetch is built-in |

### When to Use Snapshot

| Scenario | Recommended? | Reason |
|----------|-------------|--------|
| **Full page interaction** | ✅ Yes | Only option with click/form |
| **Page structure matters** | ✅ Yes | Accessibility tree included |
| **General-purpose agent UI** | ✅ Yes | Most flexible |
| **Cost is secondary** | ✅ Yes | Most expensive but complete |
| **Text extraction only** | ❌ No | Overkill (use web_fetch) |
| **Token efficiency critical** | ❌ No | Use Pinchtab instead |

---

## Practical Examples

### Scenario 1: News Article Pipeline (1,000 articles/day)

**Goal:** Extract headlines and summary text from news sites

**Tool choices:**
1. **Best:** web_fetch (~3K tokens avg, $0.63/month)
2. **Acceptable:** Snapshot (~65K tokens avg, $5.82/month) — 9x more expensive
3. **N/A:** Pinchtab — no advantage over web_fetch here

**Recommendation:** Use web_fetch. Fast, cheap, simple. Readability removes ads automatically.

---

### Scenario 2: Agent Workflow (Complex Extraction + Clicking)

**Goal:** Navigate a form, fill fields, extract structured data

**Tool choices:**
1. **Best:** Snapshot (~65K tokens) — only option with interactivity
2. **Fallback:** Pinchtab (~1.2K tokens) + separate Click API
3. **N/A:** web_fetch — no clicking capability

**Recommendation:** Snapshot for UI-heavy workflows. Pinchtab if you control the selectors.

---

### Scenario 3: High-Volume Agent Crawl (10,000 pages/day)

**Goal:** Crawl pages, extract text, minimize costs

**Tool choices:**
1. **Best:** Pinchtab (~1.2K tokens, $0.11/month)
2. **Good:** web_fetch (~6.8K tokens, $0.63/month) — 6x more expensive
3. **Expensive:** Snapshot (~65K tokens, $5.82/month) — 52x more expensive

**Recommendation:** Pinchtab dominates at scale. Real Chrome rendering + minimal tokens.

---

## Key Findings

1. **web_fetch is 9-29x lighter than snapshots** depending on site complexity
2. **Readability parsing removes boilerplate automatically** — no manual filtering needed
3. **Pinchtab still beats web_fetch 5-10x** on token efficiency
4. **web_fetch excels at content extraction**, not interaction
5. **At scale, Pinchtab ROI emerges** (~$5+/month on 1K pages/day)

---

## Limitations & Gotchas

### web_fetch Won't Work On

- ❌ JavaScript-rendered pages (SPA, dynamic content)
- ❌ Pages requiring authentication/cookies
- ❌ Forms or interactive widgets
- ❌ Real-time data (stock prices, live feeds)
- ❌ Heavy client-side navigation

### Readability Parser May Fail On

- Paywalled content (removes part of article)
- Non-article pages (shopping carts, dashboards)
- Custom layouts (if not semantic HTML)
- Frames/iframes (often ignored)

---

## Recommendation Flowchart

```
Need to interact/click?
├─ YES → Use Snapshot (full DOM + refs)
└─ NO → Can rendering?
    ├─ No → Use web_fetch (cheapest, instant)
    └─ Yes → Scale matters?
        ├─ Large (10K+ pages/day) → Use Pinchtab (90% savings)
        └─ Small (<1K pages/day) → Use web_fetch (simplicity)
```

---

## Raw Data

Full test results and calculations:

📦 **[webfetch-test-results.zip](./webfetch-test-results.zip)** (3.6 KB)

Contents:
- `test-summary.md` — Detailed web_fetch results (BBC, Corriere, Daily Mail)
- `token-calculations.md` — Token math and comparative analysis
- `test-metadata.json` — Test metadata, methodology, key findings

**Summary data:**
- BBC: 18.8 KB → ~4,700 tokens
- Corriere: 13.1 KB → ~3,275 tokens
- Daily Mail: 50 KB (truncated) → ~12,500 tokens

**Companion analyses:**
- 📊 **[browser-extraction-spectrum.md](./browser-extraction-spectrum.md)** — Compare all three methods
- 🖥️ **[default-isolated-browser.md](./default-isolated-browser.md)** — Snapshot baseline
- 📦 **[snapshot-test-results.zip](./snapshot-test-results.zip)** — Snapshot test data

---

## Footnotes

**Token estimation:**
- web_fetch text: **4 characters ≈ 1 token** (content-heavy)
- Claude Sonnet pricing: **$3 per million tokens**

**Readability parsing:**
- Industry standard algorithm (used by Pocket, Safari Reader)
- Removes ~70-90% of boilerplate (nav, ads, sidebars)
- Fails gracefully on non-article pages

**Test date:** February 26, 2026
**OpenClaw version:** 2026.2.23
