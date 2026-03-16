# Lite Engine

PinchTab includes a **Lite Engine** that performs DOM capture — navigate, snapshot,
text extraction, click, and type — without requiring Chrome or Chromium.  It is
powered by [Gost-DOM](https://github.com/gost-dom/browser) (v0.11.0, MIT), a headless
browser written in pure Go.

**Issue:** [#201](https://github.com/pinchtab/pinchtab/issues/201)

---

## Why a Lite Engine?

Chrome is the default execution backend for PinchTab.  A real browser session handles
JavaScript rendering, bot-detection bypass, screenshots, and PDF generation.  For many
workloads — static sites, wikis, news articles, APIs — none of these are needed.

| Driver | Chrome | Lite |
|--------|--------|------|
| Memory per instance | ~200 MB | ~10 MB |
| Cold-start latency | 1–6 seconds | <100 ms |
| JavaScript rendering | yes | no |
| Screenshots / PDF | yes | no |
| No Chrome installation required | no | **yes** |

Lite wins at DOM-only workloads (3–4× faster navigate, 3× faster snapshot) and is the
right choice for containers, CI pipelines, and edge environments where Chrome is not
available.

---

## Architecture

### Engine Interface

All engines implement a common interface defined in `internal/engine/engine.go`:

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

The Chrome engine wraps the existing CDP/chromedp pipeline.  `LiteEngine` in
`internal/engine/lite.go` implements the same interface using Gost-DOM.

### Router (Strategy Pattern)

```
Request → Router → [Rule 1] → [Rule 2] → … → [Fallback Rule] → Engine
```

`Router` in `internal/engine/router.go` evaluates an ordered chain of `RouteRule`
implementations.  The first rule that returns a non-`Undecided` verdict wins.  Rules
are registered at startup and are hot-swappable via `AddRule()` / `RemoveRule()`.

No handler, bridge, or config change is needed when adding new routing logic — only a
`RouteRule` implementation and a single `router.AddRule(myRule)` call.

### Built-in Rules

| Rule | File | Behaviour |
|------|------|-----------|
| `CapabilityRule` | `rules.go` | Routes `screenshot`, `pdf`, `evaluate`, `cookies` → Chrome |
| `ContentHintRule` | `rules.go` | Routes URLs ending in `.html/.htm/.xml/.txt/.md` → Lite |
| `DefaultLiteRule` | `rules.go` | Catch-all: all remaining DOM ops → Lite (used in `lite` mode) |
| `DefaultChromeRule` | `rules.go` | Final fallback → Chrome (used in `chrome` and `auto` modes) |

### Three Modes

| Mode | Behaviour |
|------|-----------|
| `chrome` | All requests go through Chrome. Backward-compatible default. |
| `lite` | DOM operations (navigate, snapshot, text, click, type) use Gost-DOM. Screenshot / PDF / evaluate / cookies fall through to Chrome (501 if Chrome is unavailable). |
| `auto` | Per-request routing via rules: capability and content-hint rules are evaluated first; unknown URLs fall back to Chrome. |

---

## Request Flow (Lite Mode)

```
POST /navigate   (server.engine=lite)
    │
    ▼
handlers/navigation.go — HandleNavigate()
    │
    ├─ useLite() == true
    │       │
    │       ▼
    │   LiteEngine.Navigate(ctx, url)
    │       ├─ HTTP GET url
    │       ├─ Strip <script> tags (x/net/html tokenizer)
    │       ├─ browser.NewWindowReader(reader)  [Gost-DOM]
    │       └─ return NavigateResult{TabID, URL, Title}
    │
    └─ w.Header().Set("X-Engine", "lite")
       JSON {"tabId": "lp-1", "url": "…", "title": "…"}
```

Snapshot then traverses the Gost-DOM document tree and maps HTML semantics to
accessibility roles (heading, link, button, textbox, …).  Text walks the same tree and
collapses whitespace runs.

---

## Capability Boundaries

| Operation | Lite | Chrome |
|-----------|------|--------|
| Navigate | ✅ (HTTP fetch + DOM parse) | ✅ |
| Snapshot | ✅ | ✅ |
| Text extraction | ✅ | ✅ |
| Click | ✅ (DOM event dispatch) | ✅ |
| Type | ✅ (DOM input events) | ✅ |
| Screenshot | ❌ → `501 Not Implemented` | ✅ |
| PDF | ❌ → `501 Not Implemented` | ✅ |
| Evaluate (JS) | ❌ → `501 Not Implemented` | ✅ |
| Cookies | ❌ → `501 Not Implemented` | ✅ |
| JavaScript-rendered SPAs | ❌ | ✅ |
| Bot-detection bypass | ❌ | ✅ |

`CapabilityRule` ensures screenshot/pdf/evaluate/cookies are always routed to Chrome
even in `lite` mode.

---

## Known Limitations

| Limitation | Detail |
|------------|--------|
| `<script>` tags | Gost-DOM panics on an un-initialized `ScriptHost`. Scripts are stripped before parse via `x/net/html` tokenizer. |
| `<a href>` click | Gost-DOM navigates on anchor click and may encounter scripts. `Click()` wraps execution in `defer recover()` and returns an error instead of panicking. |
| CSS `display:none` | Lite has no CSS engine so hidden elements still appear in the snapshot. |
| JavaScript-rendered content | Only the initial HTML is captured. SPAs (React, Next.js etc.) should use Chrome. |
| Sites that block HTTP bots | Stack Overflow and similar sites return 4xx/5xx to plain HTTP clients. Chrome bypasses this via a real browser session. |

---

## Configuration

Set the engine in your config file:

```json
{
  "server": {
    "engine": "lite"
  }
}
```

The `engine` field is also forwarded to child bridge instances so every managed
instance in a multi-instance deployment uses the same mode.

### Response Header

Responses served by the Lite engine include:

```
X-Engine: lite
```

This header is present on `navigate`, `snapshot`, and `text` responses when the lite
path was taken and is useful for observability and debugging.

---

## Performance

Benchmark across 8 real-world websites (Navigate → Snapshot → Text pipeline, 7 sites
where both engines completed successfully):

| Metric | Lite | Chrome | Speedup |
|--------|-----:|-------:|--------:|
| Navigate total | 4,580 ms | 17,981 ms | **3.9×** faster |
| Snapshot total | 1,739 ms | 5,155 ms | **3.0×** faster |
| Text total | 925 ms | 500 ms | 0.5× (Chrome faster) |
| **Grand total** | **7,244 ms** | **23,636 ms** | **3.3× faster** |

Chrome is faster at text extraction because it runs Mozilla Readability.js in-browser.
Lite performs a raw DOM text walk which is slower for very large pages (e.g. Wikipedia
CS: 687 ms vs 130 ms).

### When to use each engine

| Workload | Recommendation |
|----------|---------------|
| Static sites, wikis, news, blogs | **Lite** — 3–12× faster, no Chrome overhead |
| JavaScript-rendered SPAs | **Chrome** — Lite captures pre-JS HTML only |
| Sites that block HTTP clients | **Chrome** — real browser bypasses bot detection |
| Large-page snapshot / traversal | **Lite** — 3× faster snapshot |
| Text extraction on large articles | **Chrome** — Readability.js is more accurate |
| Screenshots, PDF, evaluate, cookies | **Chrome** — not supported in Lite |

---

## Code Layout

| File | Purpose |
|------|---------|
| `internal/engine/engine.go` | `Engine` interface, `Capability` constants, `Mode` enum, `NavigateResult` / `SnapshotNode` types |
| `internal/engine/lite.go` | `LiteEngine` — HTTP fetch, script stripping, Gost-DOM parse, role mapping |
| `internal/engine/router.go` | `Router` — ordered rule chain, `AddRule` / `RemoveRule` |
| `internal/engine/rules.go` | `CapabilityRule`, `ContentHintRule`, `DefaultLiteRule`, `DefaultChromeRule` |
| `internal/handlers/navigation.go` | `useLite()` fast path, `X-Engine` header |
| `internal/handlers/snapshot.go` | `SnapshotNode → A11yNode` conversion for lite path |
| `internal/handlers/text.go` | Lite text fast path |
| `cmd/pinchtab/cmd_bridge.go` | Router wiring from `config.Engine` at startup |

---

## Dependency

| Package | Version | License | Purpose |
|---------|---------|---------|---------|
| `github.com/gost-dom/browser` | v0.11.0 | MIT | Headless browser: HTML parsing, DOM traversal, event dispatch |
| `github.com/gost-dom/css` | v0.1.0 | MIT | CSS selector evaluation |
| `golang.org/x/net` | existing | BSD-3 | HTML tokenizer used for script stripping |
