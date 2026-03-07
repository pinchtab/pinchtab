# Architecture

## Overview

Pinchtab is an HTTP server (Go binary, ~12MB) that wraps Chrome DevTools Protocol (CDP)
to give AI agents browser control via a simple REST API.

The product has two process roles:
- **Server** — the default `pinchtab` process that manages profiles, instances, routing, and the dashboard
- **Bridge** — the single-instance runtime used for one managed browser

**Managed mode (default):** the Pinchtab server launches and manages bridge-backed Chrome instances.

```
┌─────────────┐     HTTP      ┌──────────────┐      CDP       ┌──────────────┐
│   AI Agent  │ ────────────▶ │   Pinchtab   │ ─────────────▶ │    Chrome    │
│  (any LLM)  │ ◀──────────── │  (Go binary) │ ◀───────────── │ self-launched │
└─────────────┘    JSON/text  └──────────────┘   WebSocket    └──────────────┘
```

**Attach mode (advanced):** the Pinchtab server can register an externally managed Chrome instance through the instance API when attach is enabled by policy.

```
┌─────────────┐     HTTP      ┌──────────────┐      CDP       ┌──────────────┐
│  Multiple   │ ────────────▶ │  Pinchtab    │ ─────────────▶ │  External    │
│  Agents     │ ◀──────────── │   Server     │ ◀───────────── │   Chrome     │
│             │    JSON/text  │              │   WebSocket    │   Instance   │
└─────────────┘               └──────────────┘                └──────────────┘
```

Agents never touch CDP directly. They send HTTP requests, get back JSON.
The accessibility tree (a11y) is the primary interface — not screenshots, not DOM.

## Instance Mental Model

The cleanest way to think about instances is with two axes:

- **Source** — who created or registered the instance
- **Runtime** — how the server reaches the browser

### Chart 1: Process Roles

```text
pinchtab server
  ├─ manages profiles
  ├─ manages instances
  ├─ routes requests
  └─ serves dashboard/API

pinchtab bridge
  ├─ wraps one browser
  ├─ exposes single-instance HTTP API
  └─ is usually spawned by the server
```

### Chart 2: Instance Taxonomy

```text
Instance
  ├─ source: managed
  │    ├─ runtime: bridge
  │    └─ runtime: direct-cdp   (possible future model)
  │
  └─ source: attached
       └─ runtime: direct-cdp
```

This separation matters:

- `managed` means Pinchtab owns instance lifecycle
- `attached` means Pinchtab registers an already running browser
- `bridge` means the server talks HTTP to a child Pinchtab runtime
- `direct-cdp` means the server talks to Chrome over CDP directly

### Suggested Instance Schema

If you model this explicitly in the instance object, the clean shape is:

```json
{
  "id": "inst_0a89a5bb",
  "name": "work",
  "source": "managed",
  "runtime": "bridge",
  "ownership": "pinchtab",
  "status": "starting",
  "profileId": "prof_278be873",
  "profileName": "work",
  "port": "9868",
  "baseUrl": "http://127.0.0.1:9868",
  "cdpUrl": "",
  "attached": false
}
```

Recommended fields:

- `id` — stable instance identifier
- `name` — human-oriented instance/profile label
- `source` — `managed` or `attached`
- `runtime` — `bridge` or `direct-cdp`
- `ownership` — `pinchtab`, `external`, or `adopted`
- `status` — `starting`, `running`, `stopping`, `stopped`, `error`
- `profileId` / `profileName` — associated profile, when relevant
- `port` / `baseUrl` — bridge-facing address when the instance has an HTTP runtime
- `cdpUrl` — discovered or attached CDP endpoint when relevant
- `attached` — compatibility field for old clients; derivable from `source == "attached"`

In other words:

```text
source   = who introduced the instance
runtime  = how the server reaches it
ownership = who controls its lifecycle
```

That gives these combinations:

```text
managed + bridge + pinchtab
managed + direct-cdp + pinchtab
attached + direct-cdp + external
```

### Chart 3: Routing Paths

```text
Managed + bridge
  server -> bridge -> Chrome -> tabs

Managed + direct-cdp
  server -> Chrome -> tabs

Attached + direct-cdp
  server -> external Chrome -> tabs
```

### Chart 4: What Lives In The Pool

```text
Pinchtab server
  └─ instance pool
       ├─ instance A
       │    └─ tabs
       ├─ instance B
       │    └─ tabs
       └─ instance C
            └─ tabs
```

The pool contains **instances**, not tabs.
Tabs always belong to an instance.

### Current And Future Scope

Today, the intended architecture is:

- `managed + bridge` for Pinchtab-launched instances
- `attached + direct-cdp` for externally managed browsers

A plausible future expansion is:

- `managed + direct-cdp` to remove the extra HTTP hop when the server can own Chrome directly

For a focused comparison, see [Managed Bridge vs Managed Direct-CDP](managed-bridge-vs-managed-direct-cdp.md).
For the visual version of the model, see [Instance Model Charts](instance-model-charts.md).

## Design Principles

1. **A11y tree over screenshots** — 4x cheaper in tokens, works with any LLM
2. **HTTP over WebSocket** — Stateless requests, no connection management for agents
3. **Ref stability** — Snapshot refs (e0, e1...) are cached and reused by action endpoints
4. **Self-contained by default** — Launches and manages Chrome itself unless you explicitly use attach
5. **Decoupled Architecture** — Interface-driven design for testability and maintainability

## Project Layout

The project follows the standard Go `internal/` pattern to ensure encapsulation and clean boundaries:

```
pinchtab/
├── cmd/pinchtab/        # Application entry points and CLI commands
├── internal/
│   ├── bridge/          # Core CDP logic, tab management, and state logic
│   ├── handlers/        # HTTP API handlers and middleware
│   ├── orchestrator/    # Multi-instance lifecycle and process management
│   ├── profiles/        # Chrome profile management and identity discovery
│   ├── dashboard/       # Backend logic and static assets for the web UI
│   ├── assets/          # Centralized embedded files (stealth scripts, HTML)
│   ├── human/           # Human-like interaction simulation (Bezier mouse, typing)
│   ├── config/          # Centralized configuration management
│   └── web/             # Shared HTTP and JSON utilities
├── Dockerfile           # Alpine + Chromium container image
└── scripts/             # Deployment and automation scripts
```

## Core Components

### Bridge (`internal/bridge`)

The central state holder. Owns the Chrome browser context, tab registry, and snapshot caches. It implements the `BridgeAPI` interface.

Key responsibilities:
- **Tab lifecycle** — `CreateTab`, `CloseTab`, `TabContext` (resolve "" to first tab)
- **Ref caching** — Each tab's last snapshot is cached. When `/action` receives `ref: "e5"`,
  it looks up the cached `BackendDOMNodeID` without re-fetching the a11y tree.
- **State Logic** — Diffing snapshots and manage session persistence (`SaveState`/`RestoreState`).

### Orchestrator (`internal/orchestrator`)

Manages multiple isolated browser instances. It uses a `HostRunner` interface to decouple business logic from OS process management.

Key responsibilities:
- **Instance Registry** — Tracking running instances, their ports, and statuses.
- **Process Management** — Spawning, signaling, and stopping instances.
- **Health Monitoring** — Probing instance health via HTTP.

### Profiles (`internal/profiles`)

Handles Chrome user data directories and metadata.

Key responsibilities:
- **CRUD Operations** — Creating, importing, and resetting profiles.
- **Identity Discovery** — Parsing internal Chrome JSON files to find user identity info.
- **Activity Tracking** — Recording and analyzing agent actions per profile.

### Snapshot Pipeline (`internal/bridge/snapshot.go`)

The a11y tree is Pinchtab's core abstraction. Flow:

```
Chrome a11y tree (CDP)
       │
       ▼
  Raw JSON parse (RawAXNode)     ← Manual parsing to avoid cdproto crash
       │                            on "uninteresting" PropertyName values
       ▼
  Flatten to []A11yNode           ← DFS walk, assign refs (e0, e1, e2...)
       │
       ├──▶ JSON (default)        ← Full structured output
       ├──▶ Text (indented tree)  ← Low-token format for agents
       └──▶ YAML                  ← Alternative structured format
```

**Ref caching**: When `/snapshot` is called, the ref→nodeID mapping is stored per tab.
When `/action` receives `{"ref": "e5", "kind": "click"}`, it looks up `e5` in the cache.

### Human Interaction (`internal/human`)

Two main simulation engines for anti-detection:

**`MouseMove`** — Cubic bezier curve from A to B:
- Random control points for natural curvature
- Step count scales with distance (5-30 steps)
- Per-step jitter and variable timing

**`Type`** — Keystroke-level simulation:
- Base delay: 80ms/char (40ms in fast mode)
- Random long pauses ("thinking")
- Simulated typos and backspace corrections

## Deployment

### Binary (recommended)
```bash
# Build
go build -o pinchtab ./cmd/pinchtab

# Run
PINCHTAB_TOKEN=secret ./pinchtab
```

### Docker
```bash
docker build -t pinchtab .
docker run -d -p 9867:9867 -e PINCHTAB_TOKEN=secret pinchtab
```

---

## CDP Architecture

PinchTab sits between your tools/agents and Chrome:

```text
┌─────────────────────────────────────────┐
│         Your Tool/Agent                 │
│   (CLI, curl, Python, Node.js, etc.)    │
└──────────────┬──────────────────────────┘
               │
               │ HTTP
               ↓
┌─────────────────────────────────────────┐
│    PinchTab HTTP Server (Go)            │
│  ┌─────────────────────────────────┐    │
│  │  Tab Manager                    │    │
│  │  (tracks tabs + sessions)       │    │
│  └─────────────────────────────────┘    │
│  ┌─────────────────────────────────┐    │
│  │  Chrome DevTools Protocol (CDP) │    │
│  └─────────────────────────────────┘    │
└──────────────┬──────────────────────────┘
               │
               │ CDP WebSocket
               ↓
┌─────────────────────────────────────────┐
│        Chrome Browser                   │
│  (Headless, headed, or external)        │
└─────────────────────────────────────────┘
```

PinchTab wraps Chrome's DevTools Protocol (CDP) to translate HTTP requests into CDP commands, manage browser state, and deliver structured responses (accessibility trees, screenshots, PDFs) back to your agents.
