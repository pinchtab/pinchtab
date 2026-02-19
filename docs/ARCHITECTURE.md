# Pinchtab Architecture

## Overview

Pinchtab is an HTTP server (Go binary, ~12MB) that wraps Chrome DevTools Protocol (CDP)
to give AI agents browser control via a simple REST API. It self-launches Chrome,
manages tabs, and exposes the accessibility tree as flat JSON with stable refs.

```
┌─────────────┐     HTTP      ┌──────────────┐      CDP       ┌──────────────┐
│   AI Agent  │ ────────────▶ │   Pinchtab   │ ─────────────▶ │    Chrome    │
│  (any LLM)  │ ◀──────────── │  (Go binary) │ ◀───────────── │ headed/headless │
└─────────────┘    JSON/text  └──────────────┘   WebSocket    └──────────────┘
```

Agents never touch CDP directly. They send HTTP requests, get back JSON.
The accessibility tree (a11y) is the primary interface — not screenshots, not DOM.

## Design Principles

1. **A11y tree over screenshots** — 4x cheaper in tokens, works with any LLM
2. **HTTP over WebSocket** — Stateless requests, no connection management for agents
3. **Ref stability** — Snapshot refs (e0, e1...) are cached and reused by action endpoints
4. **Self-contained** — Launches its own Chrome, manages its own state, zero config needed
5. **Internal tool** — Not a product, not a SaaS, not a platform

## File Structure

```
pinchtab/
├── main.go              # Entry point: Chrome launch, route registration, signal handling
├── config.go            # Configuration: env vars, config file, embedded assets
├── bridge.go            # Bridge struct: tab management, Chrome lifecycle
├── interfaces.go        # BridgeAPI interface (for test mocking)
├── handlers.go          # HTTP handlers: navigate, screenshot, tabs, lock, shutdown
├── handler_snapshot.go  # Snapshot handler: a11y tree fetch, format, file output
├── handler_actions.go   # Action/actions handlers: click, type, press, fill, etc.
├── handler_cookies.go   # Cookie get/set handlers
├── handler_stealth.go   # Stealth status, fingerprint rotation
├── snapshot.go          # A11y tree parsing: CDP response → flat A11yNode list
├── lock.go              # Tab locking for multi-agent coordination
├── animations.go        # CSS animation disabling (global + per-request)
├── human.go             # Human-like interaction: bezier mouse, natural typing
├── cdp.go               # Raw CDP helpers: navigate, readyState polling
├── state.go             # Session persistence: save/restore tabs to disk
├── middleware.go         # HTTP middleware: logging, auth token, CORS
├── stealth.js           # Stealth script: light/full modes, injected via CDP
├── readability.js       # Readability extraction for /text endpoint
├── welcome.html         # Headed mode welcome page (🦀 branding)
├── Dockerfile           # Alpine + Chromium container image
└── scripts/
    ├── install-autostart.sh    # LaunchAgent/systemd installer
    ├── launchd/                # macOS plist
    └── systemd/                # Linux service unit
```

## Core Components

### Bridge (`bridge.go`)

The central state holder. Owns the Chrome browser context, tab registry, and snapshot caches.

```go
type Bridge struct {
    allocCtx   context.Context    // Chrome allocator context
    browserCtx context.Context    // Browser-level context
    tabs       map[string]*TabEntry  // tabID → context
    snapshots  map[string]*refCache  // tabID → last snapshot refs
    mu         sync.RWMutex
}
```

Key responsibilities:
- **Tab lifecycle** — `CreateTab`, `CloseTab`, `TabContext` (resolve "" to first tab)
- **Ref caching** — Each tab's last snapshot is cached. When `/action` receives `ref: "e5"`,
  it looks up the cached `backendNodeID` without re-fetching the a11y tree.
- **Thread safety** — `sync.RWMutex` protects tab and cache maps

### BridgeAPI Interface (`interfaces.go`)

```go
type BridgeAPI interface {
    TabContext(tabID string) (ctx context.Context, resolvedID string, err error)
    ListTargets() ([]*target.Info, error)
    CreateTab(url string) (string, context.Context, context.CancelFunc, error)
    CloseTab(tabID string) error
    GetRefCache(tabID string) *refCache
    SetRefCache(tabID string, cache *refCache)
    DeleteRefCache(tabID string)
}
```

Handlers receive `BridgeAPI`, not `*Bridge`. Tests can mock it without Chrome.

### Snapshot Pipeline (`snapshot.go`, `handler_snapshot.go`)

The a11y tree is Pinchtab's core abstraction. Flow:

```
Chrome a11y tree (CDP)
       │
       ▼
  Raw JSON parse (rawAXNode)     ← Manual parsing to avoid cdproto crash
       │                            on "uninteresting" PropertyName values
       ▼
  Flatten to []A11yNode           ← DFS walk, assign refs (e0, e1, e2...)
       │
       ├──▶ JSON (default)        ← Full structured output
       ├──▶ Text (indented tree)  ← Low-token format for agents
       └──▶ YAML                  ← Alternative structured format
```

**A11yNode** is the universal unit:
```go
type A11yNode struct {
    Ref      string  // "e0", "e1" — stable within snapshot
    Role     string  // "button", "link", "textbox"
    Name     string  // Accessible name
    Depth    int     // Tree depth for indentation
    Value    string  // Input values
    Disabled bool
    Focused  bool
    NodeID   int64   // CDP backendNodeID (internal, for actions)
}
```

**Ref caching**: When `/snapshot` is called, the ref→nodeID mapping is stored per tab.
When `/action` receives `{"ref": "e5", "kind": "click"}`, it looks up `e5` in the cache
to find the backendNodeID. This avoids a second a11y tree fetch (which could drift).

**Filters**:
- `?filter=interactive` — Only clickable/typeable elements (~65% token reduction)
- `?depth=3` — Limit tree depth
- `?diff=true` — Return only changes since last snapshot

### Action System (`handlers.go`)

Actions are registered in an `actionRegistry` map:

```go
var registry = map[string]actionFunc{
    "click":      ...,  // Standard click
    "type":       ...,  // Type text into focused element
    "press":      ...,  // Key press (Enter, Tab, etc.)
    "select":     ...,  // Select dropdown option
    "hover":      ...,  // Mouse hover
    "scroll":     ...,  // Scroll viewport
    "humanClick": ...,  // Bezier-curve mouse movement + click
    "humanType":  ...,  // Natural typing with variable delays
}
```

Actions resolve targets in priority order: `ref` → `selector` → `nodeId`.
Ref resolution: look up in snapshot cache → find backendNodeID → execute CDP command.

**Action chaining** (`POST /actions`): Batch multiple actions in one request.
Executes sequentially, returns results array. Saves round-trips for multi-step interactions.

### Human Interaction (`human.go`)

Two main functions for anti-detection:

**`humanMouseMove`** — Cubic bezier curve from A to B:
- Random control points for natural curvature
- Step count scales with distance (5-30 steps, capped to prevent timeout)
- Per-step jitter (±2px) and variable timing (16-24ms)

**`humanType`** — Keystroke-level simulation:
- Base delay: 80ms/char (40ms in fast mode)
- 5% chance of long pause ("thinking")
- 3% chance of typo → backspace → correct character
- Faster for repeated characters

### Stealth System (`stealth.js`, `handlers.go`)

Two layers:

**Layer 1: Chrome launch flags** (compile-time)
```
--exclude-switches=enable-automation
--disable-blink-features=AutomationControlled
--disable-infobars
+ 15 more flags
```

**Layer 2: JavaScript injection** (runtime, via `AddScriptToEvaluateOnNewDocument`)
- `navigator.webdriver` → undefined
- `navigator.plugins` → 3 realistic entries
- `navigator.languages` → ['en-US', 'en']
- WebGL vendor → "Intel Inc." / "Intel Iris OpenGL Engine"
- Canvas fingerprint noise → temp canvas with pixel-level noise
- Font metrics noise → Proxy on `measureText` (width only)
- WebRTC → `iceTransportPolicy: 'relay'` (no local IP leak)
- Hardware values → seeded PRNG, consistent within session
- Timezone → configurable via `window.__pinchtab_timezone`

**Stealth status** (`GET /stealth/status`) — live-probes the browser by evaluating JS
to verify features are actually active (not hardcoded booleans).

**Fingerprint rotation** (`POST /fingerprint/rotate`) — injects UA/platform/screen/timezone
overrides via `AddScriptToEvaluateOnNewDocument` (persists across navigations).

### Navigation (`cdp.go`)

Custom `navigatePage` instead of chromedp's built-in `Navigate`:
- Uses raw `Page.navigate` CDP command
- Polls `document.readyState` until "interactive" or "complete"
- 10-second timeout prevents hangs on heavy SPAs
- Doesn't wait for full `load` event (which SPAs may never fire)

### Session Persistence (`state.go`)

On clean shutdown (SIGINT/SIGTERM):
1. Iterate all open tabs, collect URL + title
2. Write to `~/.pinchtab/sessions.json`

On startup:
1. Read `sessions.json` if it exists
2. Re-open tabs with concurrency limiting (max 3 tabs, max 2 navigations)
3. `markCleanExit()` patches Chrome prefs to suppress "didn't shut down correctly" bar

### Configuration (`config.go`)

Priority order (highest wins):
1. Environment variables (`BRIDGE_PORT`, `BRIDGE_TOKEN`, etc.)
2. Config file (`~/.pinchtab/config.json`)
3. Defaults

```go
//go:embed stealth.js
var stealthScript string

//go:embed readability.js
var readabilityJS string
```

Both JS files are compiled into the binary via Go embed.

### Middleware (`middleware.go`)

Request pipeline:
```
Request → Logging → Auth (Bearer token) → CORS → Handler → Response
```

- **Logging**: slog with method, path, status, duration
- **Auth**: Optional `BRIDGE_TOKEN` — if set, all requests need `Authorization: Bearer <token>`
- **CORS**: Permissive (localhost tool, not public)

## Data Flow

### Typical Agent Interaction

```
Agent                          Pinchtab                        Chrome
  │                               │                              │
  │  POST /navigate               │                              │
  │  {"url": "https://..."}       │                              │
  │ ─────────────────────────────▶│  Page.navigate               │
  │                               │─────────────────────────────▶│
  │                               │  poll readyState             │
  │                               │◀─────────────────────────────│
  │  {"tabId": "AB12..."}         │                              │
  │◀──────────────────────────────│                              │
  │                               │                              │
  │  GET /snapshot?filter=interactive                            │
  │ ─────────────────────────────▶│  Accessibility.getFullAXTree │
  │                               │─────────────────────────────▶│
  │                               │  ◀── raw AX nodes ────────── │
  │                               │  flatten + assign refs       │
  │                               │  cache ref→nodeID            │
  │  [{"ref":"e0","role":"button",│                              │
  │    "name":"Sign In"}, ...]    │                              │
  │◀──────────────────────────────│                              │
  │                               │                              │
  │  POST /action                 │                              │
  │  {"ref":"e0","kind":"click"}  │                              │
  │ ─────────────────────────────▶│  lookup e0 → nodeID 1234     │
  │                               │  DOM.resolveNode(1234)       │
  │                               │  DOM.getBoxModel             │
  │                               │  Input.dispatchMouseEvent    │
  │                               │─────────────────────────────▶│
  │  {"clicked": true}            │                              │
  │◀──────────────────────────────│                              │
```

### Typical Agent Interaction in Headed mode

In headed mode, the human sets up and validates profiles first, then the agent runs against those profiles.

```
Human Operator        Dashboard (`pinchtab dashboard`)     Profile Instance        Agent
     │                            │                              │                   │
     │ create/import profile      │                              │                   │
     │───────────────────────────▶│                              │                   │
     │ launch profile on :9868    │                              │                   │
     │───────────────────────────▶│ spawn `pinchtab`             │                   │
     │                            │─────────────────────────────▶│ headed Chrome     │
     │ manual login / 2FA         │                              │                   │
     │<──────────────────────────────────── visible browser ────────────────────────-│
     │ validate state             │                              │                   │
     │                            │ profile URL ready            │                   │
     │                            │─────────────┬───────────────-│                   │
     │                            │             │                                    │
     │                            │             │            POST /navigate          │
     │                            │             │◀──────────────────────────────────-│
     │                            │             │            GET /snapshot           │
     │                            │             │◀──────────────────────────────────-│
     │                            │             │            POST /action            │
     │                            │             │◀──────────────────────────────────-│
```

Key point: the dashboard coordinates profiles and lifecycle, but agents call the running profile instance (`http://localhost:<profile-port>`), not the dashboard proxy endpoint.

### Token Efficiency

| Method | ~Tokens | Use Case |
|--------|---------|----------|
| Full snapshot (JSON) | 10,500 | Initial page understanding |
| Interactive filter | 3,600 | Action planning (buttons, links, inputs) |
| `/text` (readability) | 800 | Content extraction |
| `/screenshot` | 2,000 | Visual verification |
| Diff snapshot | varies | Monitoring changes |

## API Reference

### Read Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Server status + version |
| `/tabs` | GET | List all open tabs |
| `/snapshot` | GET | A11y tree snapshot. Params: `tabId`, `filter`, `depth`, `format`, `diff`, `output` |
| `/screenshot` | GET | JPEG screenshot. Params: `tabId`, `quality`, `output` |
| `/text` | GET | Readability-extracted text. Params: `tabId` |
| `/cookies` | GET | Get cookies for current page. Params: `tabId` |
| `/stealth/status` | GET | Live stealth feature verification |

### Write Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/navigate` | POST | Navigate tab to URL. Body: `{"url", "tabId", "newTab"}` |
| `/action` | POST | Execute single action. Body: `{"kind", "ref", "selector", "text", ...}` |
| `/actions` | POST | Execute action batch. Body: `[{action}, {action}, ...]` |
| `/evaluate` | POST | Run JavaScript. Body: `{"expression", "tabId"}` |
| `/tab` | POST | Tab management. Body: `{"action": "close", "tabId"}` |
| `/cookies` | POST | Set cookies. Body: `{"cookies": [...]}` |
| `/fingerprint/rotate` | POST | Rotate browser fingerprint. Body: `{"os", "browser", "screen", ...}` |

### Action Kinds

| Kind | Required Fields | Description |
|------|----------------|-------------|
| `click` | `ref` or `selector` | Standard click |
| `type` | `ref`/`selector` + `text` | Type into element |
| `press` | `key` | Keyboard press (Enter, Tab, Escape, etc.) |
| `select` | `ref`/`selector` + `values` | Select dropdown option(s) |
| `hover` | `ref` or `selector` | Mouse hover |
| `scroll` | (optional) `scrollX`, `scrollY` | Scroll viewport (default: down 800px) |
| `humanClick` | `ref` or `selector` | Click with bezier mouse movement |
| `humanType` | `ref`/`selector` + `text` | Type with natural timing + typos |

### Snapshot Formats

**JSON** (default): `GET /snapshot`
```json
[
  {"ref": "e0", "role": "heading", "name": "Welcome", "depth": 0},
  {"ref": "e1", "role": "button", "name": "Sign In", "depth": 1}
]
```

**Text**: `GET /snapshot?format=text`
```
heading "Welcome"
  button "Sign In" [e1]
  textbox "Email" [e2]
```

**YAML**: `GET /snapshot?format=yaml`

## Deployment

### Binary (recommended)
```bash
# Build
go build -o pinchtab .

# Run
BRIDGE_TOKEN=secret ./pinchtab
```

### Docker
```bash
docker run -d -p 9867:9867 -e BRIDGE_TOKEN=secret pinchtab
```

### Auto-start
```bash
# macOS
./scripts/install-autostart.sh

# Linux (systemd)
sudo cp scripts/systemd/pinchtab.service /etc/systemd/system/
sudo systemctl enable --now pinchtab
```

### Configuration
```json
// ~/.pinchtab/config.json
{
  "port": "9867",
  "token": "secret",
  "headless": true,
  "stateDir": "~/.pinchtab"
}
```

## State & Persistence

```
~/.pinchtab/
├── chrome-profile/     # Chrome user data (cookies, localStorage, etc.)
├── sessions.json       # Saved tab state (written on shutdown)
├── screenshots/        # File output from ?output=file
├── snapshots/          # File output from ?output=file
└── config.json         # Optional config file
```

## Security Model

Pinchtab has **no authentication by default**. When `BRIDGE_TOKEN` is set:
- All requests must include `Authorization: Bearer <token>`
- `/health` is exempt (for monitoring)

**Pinchtab gives full browser control.** Anyone with HTTP access can:
- Read any page content (cookies, passwords, session tokens)
- Navigate to any URL
- Execute arbitrary JavaScript
- Take screenshots

**Always run behind a firewall.** Bind to localhost unless you explicitly need remote access.
This is a power tool for trusted agents on a trusted machine — not a public service.
