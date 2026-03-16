# Lite Engine: Chrome-Free DOM Capture using Gost-DOM

**Branch:** `feat/lite-engine-gostdom`
**Issue:** [#201](https://github.com/pinchtab/pinchtab/issues/201)
**Related Draft PR:** [#200](https://github.com/pinchtab/pinchtab/pull/200)
**Dependency:** [gost-dom/browser v0.11.0](https://github.com/gost-dom/browser) (MIT, ~255 stars, Go 78.4%)

---

## Overview

This implementation adds a **Lite Engine** that can perform DOM capture (navigate, snapshot, text extraction, click, type) without requiring Chrome/Chromium. It uses [Gost-DOM](https://github.com/gost-dom/browser), a headless browser written in pure Go, to parse and traverse HTML documents.

The architecture follows the maintainer's guidance for **"clever routing that is expandable without touching the rest of the code"** — implemented via a strategy-pattern Router with pluggable rules.

## Architecture

### Engine Interface (`internal/engine/engine.go`)

```go
type Engine interface {
    Name() string
    Navigate(ctx context.Context, url string) (*NavigateResult, error)
    Snapshot(ctx context.Context, filter string) ([]SnapshotNode, error)
    Text(ctx context.Context) (string, error)
    Click(ctx context.Context, ref string) error
    Type(ctx context.Context, ref, text string) error
    Capabilities() []Capability
    Close() error
}
```

### Router (`internal/engine/router.go`)

The Router evaluates an ordered chain of `RouteRule` implementations. The first rule to return a non-`Undecided` verdict wins.

```
Request → Router → [Rule 1] → [Rule 2] → ... → [Fallback Rule] → Engine
```

Rules are hot-swappable at runtime via `AddRule()` / `RemoveRule()` — no handler code changes needed.

### Three Modes

| Mode | Behavior | Default Rules |
|------|----------|---------------|
| `chrome` | All requests → Chrome (default, backward compatible) | DefaultChromeRule |
| `lite` | DOM ops → Gost-DOM, screenshots/PDF/evaluate → Chrome | CapabilityRule → DefaultLiteRule |
| `auto` | Per-request routing based on URL patterns | CapabilityRule → ContentHintRule → DefaultChromeRule |

### Built-in Rules (`internal/engine/rules.go`)

| Rule | Purpose |
|------|---------|
| `CapabilityRule` | Routes screenshot/pdf/evaluate/cookies → Chrome (lite can't do these) |
| `ContentHintRule` | Routes `.html/.htm/.xml/.txt/.md` URLs → Lite (for navigate/snapshot/text) |
| `DefaultLiteRule` | Catch-all: routes all DOM ops → Lite |
| `DefaultChromeRule` | Final fallback: routes everything → Chrome |

### Expandability

Adding new routing logic requires only:
1. Implement `RouteRule` interface (2 methods: `Name()`, `Decide()`)
2. Call `router.AddRule(myRule)` — inserted before the fallback rule

No handler, config, or CMD changes needed.

## Files Changed

### New Files (8)
| File | Purpose | Lines |
|------|---------|-------|
| `internal/engine/engine.go` | Engine interface, types, capabilities | ~70 |
| `internal/engine/lite.go` | LiteEngine implementation using Gost-DOM | ~430 |
| `internal/engine/router.go` | Router with AddRule/RemoveRule | ~120 |
| `internal/engine/rules.go` | 4 built-in RouteRule implementations | ~95 |
| `internal/engine/lite_test.go` | LiteEngine unit tests | ~280 |
| `internal/engine/router_test.go` | Router unit tests | ~130 |
| `internal/engine/rules_test.go` | Rule unit tests | ~115 |
| `internal/engine/realworld_test.go` | Real-world website comparison tests | ~570 |

### Modified Files (8)
| File | Change |
|------|--------|
| `internal/config/config.go` | Added `Engine` field to RuntimeConfig + ServerConfig |
| `internal/handlers/handlers.go` | Added `Router *engine.Router` field, `useLite()` helper |
| `internal/handlers/navigation.go` | Lite fast path before ensureChrome |
| `internal/handlers/snapshot.go` | Lite fast path with SnapshotNode → A11yNode conversion |
| `internal/handlers/text.go` | Lite fast path returning plain text |
| `cmd/pinchtab/cmd_bridge.go` | Engine router wiring based on config mode |
| `go.mod` | Added gost-dom/browser v0.11.0, gost-dom/css v0.1.0 |
| `go.sum` | Updated checksums |

## Improvements Over PR #200 Draft

| Area | PR #200 | This Implementation |
|------|---------|-------------------|
| Tab management | Single window | Multi-tab with sequential IDs |
| HTML parsing | `browser.Open()` double-fetches | HTTP fetch → strip scripts → `html.NewWindowReader` |
| Script handling | Panics on `<script>` tags | Pre-parse stripping via `x/net/html` tokenizer |
| Click safety | No panic protection | `defer recover()` in Click method |
| Text output | Raw DOM text | `normalizeWhitespace()` — collapses runs of whitespace |
| Role mapping | Basic (a, button, input, etc.) | Extended: section→region, details→group, summary→button, dialog, article |
| Interactive detection | Basic tags | Adds summary, ARIA roles (tab, menuitem, switch) |
| Routing | None (always lite) | Strategy-pattern Router with pluggable rules |
| Configuration | None | Config file support (`server.engine`) |

## Test Results

### Engine Package Tests (40+ tests, all passing)

```
=== Unit Tests ===
TestLiteEngine_Navigate          PASS
TestLiteEngine_Snapshot_All      PASS
TestLiteEngine_Snapshot_Interactive  PASS
TestLiteEngine_Text              PASS
TestLiteEngine_Click             PASS
TestLiteEngine_Type              PASS
TestLiteEngine_RefNotFound       PASS
TestLiteEngine_ScriptStyleSkipped  PASS
TestLiteEngine_AriaAttributes    PASS
TestLiteEngine_MultiTab          PASS
TestLiteEngine_Close             PASS
TestLiteEngine_Capabilities      PASS
TestLiteEngine_Name              PASS
TestNormalizeWhitespace          PASS

=== Router Tests ===
TestRouterChromeMode             PASS
TestRouterLiteMode               PASS
TestRouterAutoModeStaticContent  PASS
TestRouterAutoModeLiteNil        PASS
TestRouterAddRemoveRule          PASS
TestRouterRulesSnapshot          PASS

=== Rule Tests ===
TestCapabilityRule (9 cases)     PASS
TestContentHintRule (9 cases)    PASS
TestDefaultLiteRule (7 cases)    PASS
TestDefaultChromeRule (4 cases)  PASS
```

### Real-World Website Comparison Tests (16 suites, 63+ subtests)

| Suite | Simulates | Subtests | Result |
|-------|-----------|----------|--------|
| WikipediaStyle | Wikipedia article page | 9 | PASS |
| HackerNewsStyle | HN front page | 4 | PASS |
| EcommerceStyle | Product page with forms | 9 | PASS |
| FormHeavy | Registration form | 7 | PASS |
| AriaHeavy | Dashboard with ARIA roles | 11 | PASS |
| DeeplyNested | 5+ levels of div nesting | 4 | PASS |
| SpecialCharacters | Unicode, HTML entities, CJK | 3 | PASS |
| EmptyPage | Empty HTML body | 1 | PASS |
| NonHTMLContentType | JSON response | 1 | PASS |
| HTTP404 | 404 error page | 1 | PASS |
| LargePagePerformance | 200 sections, 800+ nodes | 1 | PASS |
| MultipleScriptTags | 5 script tags in head+body | 1 | PASS |
| InlineStyles | Style tags in head+body | 1 | PASS |
| ClickWorkflow | Button clicks | 1 | PASS |
| ClickLinkRecovery | Anchor click panic recovery | 1 | PASS |
| TypeWorkflow | Type into all textboxes | 1 | PASS |

### Full Project Test Suite

```
ok   cmd/pinchtab           2.8s
ok   internal/allocation    2.0s
ok   internal/config        1.6s
ok   internal/dashboard     3.1s
ok   internal/engine        1.4s   ← new package
ok   internal/handlers      6.8s
ok   internal/human         10.7s
ok   internal/idpi          2.0s
ok   internal/idutil        1.8s
ok   internal/instance      2.6s
ok   internal/orchestrator  3.2s
ok   internal/profiles      2.8s
ok   internal/proxy         2.8s
ok   internal/scheduler     4.0s
ok   internal/semantic      1.6s
ok   internal/strategy      1.7s
ok   internal/uameta        1.1s
ok   internal/web           1.5s
```

## Known Edge Cases & Limitations

| Edge Case | Behavior | Mitigation |
|-----------|----------|------------|
| `<script>` tags in HTML | Gost-DOM panics (nil ScriptHost) | Pre-parse stripping via x/net/html tokenizer |
| Click on `<a href>` | Gost-DOM navigates, may encounter scripts | `defer recover()` in Click, returns error |
| CSS `display:none` | Elements still appear in snapshot | Lite engine has no CSS engine |
| JavaScript-rendered content | Not captured (SPA, dynamic DOM) | Falls back to Chrome in auto mode |
| Screenshots / PDF | Not supported in lite | CapabilityRule routes to Chrome |
| Cookies / Evaluate | Not supported in lite | CapabilityRule routes to Chrome |
| `<noscript>` content | Stripped from snapshot | Consistent with script-disabled behavior |

## Configuration

Set the engine in your config file:
```json
{
  "server": {
    "engine": "lite"
  }
}
```

### Response Headers
Lite-served responses include `X-Engine: lite` header for observability.

## Dependency Analysis

| Package | Size | License | Purpose |
|---------|------|---------|---------|
| gost-dom/browser v0.11.0 | ~2.5MB source | MIT | Headless browser (HTML parsing, DOM traversal) |
| gost-dom/css v0.1.0 | ~200KB | MIT | CSS selector support |
| golang.org/x/net (existing) | already in go.mod | BSD-3 | HTML tokenizer for script stripping |

## Performance Benchmark: Lite vs Chrome

**Lite run:** 2026-03-09 | **Chrome run:** 2026-03-09
**Method:** 8 real-world websites × 4 operations each (Navigate → Snapshot All → Snapshot Interactive → Text)

### Response Times (ms)

| Website | Lite Navigate | Lite Snap (all) | Lite Text | Chrome Navigate | Chrome Snap (all) | Chrome Text | Winner |
|---------|:------------:|:--------------:|:---------:|:--------------:|:----------------:|:-----------:|:------:|
| Example.com | 38ms | 23ms | 29ms | 396ms | 46ms | 34ms | **LITE** |
| Wikipedia (Go) | 657ms | 775ms | 120ms | 1310ms | 2703ms | 201ms | **LITE** |
| Hacker News | 1032ms | 188ms | 21ms | 1218ms | 247ms | 27ms | **LITE** |
| httpbin.org | 1117ms | 31ms | 24ms | 4745ms | 187ms | 47ms | **LITE** |
| GitHub Explore | 1402ms | 161ms | 24ms | 6156ms | 329ms | 20ms | **LITE** |
| DuckDuckGo | 119ms | 26ms | 20ms | 1488ms | 394ms | 41ms | **LITE** |
| Wikipedia (CS) | 215ms | 535ms | 687ms | 2668ms | 1249ms | 130ms | **LITE** |
| Stack Overflow | ❌ 502 | 694ms | 111ms | 6433ms | 376ms | 61ms | **CHROME** |

> Stack Overflow blocks bot HTTP requests — the Lite engine's `Navigate` got a 502. Chrome handles this via a real browser session.

### Totals (7 sites where both engines succeeded)

| Metric | Lite | Chrome | Speedup |
|--------|-----:|-------:|--------:|
| Navigate Total | 4,580ms | 17,981ms | **3.9×** faster |
| Snapshot Total | 1,739ms | 5,155ms | **3.0×** faster |
| Text Total | 925ms | 500ms | 0.5× (Chrome faster) |
| **Grand Total** | **7,244ms** | **23,636ms** | **3.3× faster** |

> Lite wins **7/8 sites** overall. Chrome is faster at text extraction because it runs Mozilla Readability.js in-browser. Lite performs raw DOM text walk which is slower for very large articles (e.g. Wikipedia CS: 687ms vs 130ms).

### Node Count Comparison

| Website | Lite Nodes | Chrome Nodes | Lite Interactive | Chrome Interactive | Lite Text (chars) | Chrome Text (chars) |
|---------|:----------:|:------------:|:----------------:|:-----------------:|:----------------:|:-----------------:|
| Example.com | 6 | 8 | 1 | 1 | 125 | 209 |
| Wikipedia (Go) | 6,074 | 7,110 | 1,276 | 1,063 | 75,659 | 62,859 |
| Hacker News | 805 | 975 | 229 | 229 | 4,025 | 4,169 |
| httpbin.org | 62 | 113 | 5 | 29 | 274 | 1,179 |
| GitHub Explore | 1,533 | 830 | 331 | 240 | 8,340 | 368 |
| DuckDuckGo | 143 | 655 | 20 | 102 | 123 | 7,231 |
| Wikipedia (CS) | 4,941 | 4,653 | 1,627 | 1,061 | 79,799 | 58,071 |
| Stack Overflow | — | 779 | — | 192 | — | 23,671 |

> **Why node counts differ:** Lite strips `<script>` tags before parsing and has no CSS engine so hidden elements still appear. Chrome's accessibility tree prunes hidden/invisible elements. DuckDuckGo and GitHub Explore show lower Chrome text because Chrome's Readability.js strips nav/sidebar content, while Lite captures all visible text.

### Key Takeaways

| Scenario | Recommendation |
|----------|---------------|
| Static sites, wikis, news, blogs | **Lite** — 3–12× faster, no Chrome overhead |
| JavaScript-rendered SPAs (React, Next.js, etc.) | **Chrome** — Lite captures pre-JS HTML only |
| Sites that block HTTP bots (Stack Overflow, some social) | **Chrome** — real browser bypasses bot detection |
| Snapshot / DOM traversal on large pages | **Lite** — 3× faster snapshot on Wikipedia |
| Text extraction on large articles | **Chrome** — Readability.js is more accurate and faster |
| Pipelines needing screenshots / PDF / evaluate | **Chrome** — Lite doesn't support these |

*Benchmark run from `tests/lite_engine_benchmark.ps1` on 2026-03-09*
