# Architecture

## Overview

Pinchtab is an HTTP server (Go binary, ~12MB) that wraps Chrome DevTools Protocol (CDP)
to give AI agents browser control via a simple REST API.

**Self-hosted mode (default):** Pinchtab launches and manages its own Chrome instance.

```
┌─────────────┐     HTTP      ┌──────────────┐      CDP       ┌──────────────┐
│   AI Agent  │ ────────────▶ │   Pinchtab   │ ─────────────▶ │    Chrome    │
│  (any LLM)  │ ◀──────────── │  (Go binary) │ ◀───────────── │ self-launched │
└─────────────┘    JSON/text  └──────────────┘   WebSocket    └──────────────┘
```

**Remote Chrome mode (CDP_URL):** Pinchtab connects to an existing Chrome instance via CDP_URL.

```
┌─────────────┐     HTTP      ┌──────────────┐      CDP       ┌──────────────┐
│  Multiple   │ ────────────▶ │  Multiple    │ ─────────────▶ │  Shared      │
│  Agents     │ ◀──────────── │  Pinchtab    │ ◀───────────── │  Chrome      │
│             │    JSON/text  │  instances   │   WebSocket    │  instance    │
└─────────────┘               └──────────────┘                └──────────────┘
```

See [docs/cdp-url-shared-chrome.md](cdp-url-shared-chrome.md) for multi-agent resource sharing and container deployment patterns.

Agents never touch CDP directly. They send HTTP requests, get back JSON.
The accessibility tree (a11y) is the primary interface — not screenshots, not DOM.

## Design Principles

1. **A11y tree over screenshots** — 4x cheaper in tokens, works with any LLM
2. **HTTP over WebSocket** — Stateless requests, no connection management for agents
3. **Ref stability** — Snapshot refs (e0, e1...) are cached and reused by action endpoints
4. **Self-contained** — Launches its own Chrome, manages its own state, zero config needed
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
BRIDGE_TOKEN=secret ./pinchtab
```

### Docker
```bash
docker build -t pinchtab .
docker run -d -p 9867:9867 -e BRIDGE_TOKEN=secret pinchtab
```
