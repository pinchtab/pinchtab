# Pinchtab OpenClaw Plugin

Browser control for AI agents via [Pinchtab](https://pinchtab.com). Single-tool design — one `pinchtab` tool handles all browser operations. Minimal context bloat.

## Install

```bash
openclaw plugins install @pinchtab/pinchtab
openclaw gateway restart
```

## Quick Start

The plugin auto-starts a local Pinchtab server when needed. Just install and go:

```bash
openclaw plugins install @pinchtab/pinchtab
openclaw gateway restart
```

For remote servers or Docker, disable auto-start and set `baseUrl`.

## Configure

```json5
{
  plugins: {
    entries: {
      pinchtab: {
        enabled: true,
        config: {
          // Connection
          baseUrl: "http://localhost:9867",
          token: "my-secret",
          timeoutMs: 30000,

          // Startup (local only)
          autoStart: true,           // auto-start server if not running
          binaryPath: "pinchtab",    // path to binary
          startupTimeoutMs: 30000,   // max wait for startup

          // Policy
          allowEvaluate: false,      // block JS evaluate by default
          allowedDomains: [],        // empty = allow all
          allowDownloads: false,
          allowUploads: false,

          // Defaults
          defaultSnapshotFormat: "compact",
          defaultSnapshotFilter: "interactive",
          screenshotFormat: "jpeg",
          screenshotQuality: 80,

          // Session
          persistSessionTabs: true,  // remember last active tab

          // Tools & Profiles
          registerBrowserTool: true, // register OpenClaw-compatible 'browser' tool
          defaultProfile: "openclaw",
          profiles: {
            "staging": { instanceId: "staging-instance" },
            "user": { attach: true }
          },
        },
      },
    },
  },
  agents: {
    list: [{
      id: "main",
      tools: { allow: ["pinchtab"] },
    }],
  },
}
```

### Manual Server Setup

If auto-start is disabled or you're using Docker:

```bash
# Local
PINCHTAB_TOKEN=my-secret pinchtab server &

# Docker
docker run -d -p 9867:9867 ghcr.io/pinchtab/pinchtab:latest
```

## Two Tools: `browser` and `pinchtab`

The plugin registers two tools:

| Tool | Use Case |
|------|----------|
| `browser` | OpenClaw-compatible, simplified interface for common flows |
| `pinchtab` | Advanced control with all actions (mouse, wait, handoff, evaluate) |

Disable the browser tool with `registerBrowserTool: false` if you only want `pinchtab`.

## Profiles

Map browser sessions to OpenClaw profile semantics:

| Profile | Behavior |
|---------|----------|
| `openclaw` | Default isolated automation profile |
| `user` | Attach to existing browser session (cookies/logins preserved) |
| Custom | Map to specific Pinchtab instance via config |

```json5
{
  config: {
    defaultProfile: "openclaw",
    profiles: {
      "staging": { instanceId: "staging-browser" },
      "user": { attach: true }
    }
  }
}
```

Usage: `browser({ action: "navigate", url: "...", profile: "user" })`

## Browser Tool Actions

| Action | Description |
|--------|-------------|
| `navigate` | Go to URL (url, profile?, newTab?) |
| `snapshot` | Accessibility tree (selector?, format?, maxTokens?) |
| `screenshot` | Capture image (quality?, format?) |
| `click/type/fill/press/hover/scroll/select` | Element actions (ref, text?, key?) |
| `tabs` | List/new/close tabs (tabAction?, url?, tabId?) |
| `pdf` | Export PDF (landscape?, scale?) |
| `status` | Health check with config/warnings |

## Pinchtab Tool: All Actions

One tool definition, many actions — keeps context lean:

| Action | Description | Typical tokens |
|---|---|---|
| `navigate` | Go to URL | — |
| `snapshot` | Accessibility tree (refs for interactions) | ~3,600 (interactive) |
| `click/type/press/fill/hover/scroll/select/focus` | Act on element by ref | — |
| `mouse-move/mouse-down/mouse-up/mouse-wheel` | Low-level mouse controls by ref/selector/coordinates | — |
| `wait` | Wait for selector/text/url/load/fn/ms conditions | — |
| `handoff` | Human-in-the-loop pause/resume for CAPTCHA/login/2FA | — |
| `text` | Extract readable text (cheapest) | ~800 |
| `tabs` | List/open/close tabs | — |
| `screenshot` | JPEG screenshot (vision fallback) | ~2K |
| `evaluate` | Run JavaScript in page | — |
| `pdf` | Export page as PDF | — |
| `download` | Download file from URL | — |
| `upload` | Upload files to file input | — |
| `network` | Capture/inspect network requests | — |
| `health` | Check connectivity | — |

## Agent Usage Example

```
1. pinchtab({ action: "navigate", url: "https://pinchtab.com/search" })
2. pinchtab({ action: "snapshot", filter: "interactive", format: "compact" })
   → Returns refs: e0, e5, e12...
3. pinchtab({ action: "click", ref: "e5" })
4. pinchtab({ action: "type", ref: "e5", text: "pinchtab" })
5. pinchtab({ action: "press", key: "Enter" })
6. pinchtab({ action: "snapshot", diff: true, format: "compact" })
   → Only changes since last snapshot
7. pinchtab({ action: "text" })
   → Readable results (~800 tokens)
```

## Manual Mouse Tests (OpenClaw)

Use these calls to validate low-level mouse behavior through the plugin:

```
1. pinchtab({ action: "navigate", url: "https://pinchtab.com" })
2. pinchtab({ action: "snapshot", filter: "interactive", format: "compact" })
  → Pick a target ref like e5
3. pinchtab({ action: "mouse-move", ref: "e5" })
4. pinchtab({ action: "mouse-down", button: "left" })
5. pinchtab({ action: "mouse-up", button: "left" })
6. pinchtab({ action: "mouse-wheel", ref: "e5", deltaY: 240 })
```

Coordinate-driven test (viewport):

```
pinchtab({ action: "mouse-move", x: 400, y: 300 })
pinchtab({ action: "mouse-down", button: "left" })
pinchtab({ action: "mouse-up", button: "left" })
pinchtab({ action: "mouse-wheel", x: 400, y: 300, deltaY: -320 })
```

**Token strategy:** `text` for reading, `snapshot` with `filter=interactive&format=compact` for interactions, `diff=true` on subsequent snapshots, `screenshot` only for visual verification.

## Human Handoff (CAPTCHA / Login / 2FA)

Use `handoff` when manual intervention is required, then resume with a wait condition:

Current limitation: this is advisory/non-blocking right now. The plugin uses `handoff` as coordination plus waiting behavior, but it does not guarantee that later automation is blocked across the server. Treat it as a temporary workflow helper, not as an enforced pause boundary.

```
1. pinchtab({ action: "handoff", humanReason: "captcha", humanPrompt: "Please solve CAPTCHA in headed browser" })
2. pinchtab({ action: "handoff", selector: "text:Dashboard", timeout: 120000 })
  → resumes when condition is met or returns a timeout error
```

You can also call `wait` directly:

```
pinchtab({ action: "wait", text: "Welcome back", timeout: 120000 })
```

## Built-In DOM Sync Safeguards

- Ref-like selectors (for example `selector: "e56"`) are normalized to `ref` automatically.
- Element actions perform one bounded stale-ref recovery attempt after refreshing interactive snapshot.
- `fill` auto-falls back to `type` once when controlled inputs reject direct fill.
- `tabs` list uses instance-scoped fallback if global `/tabs` returns empty.

## Security Notes

- **`evaluate`** is blocked by default (`allowEvaluate: false`) — enable only for trusted agents
- Use `allowedDomains` to restrict navigation (e.g., `["*.example.com"]`)
- Use `PINCHTAB_TOKEN` to gate API access; rotate regularly
- In production, run behind HTTPS reverse proxy (Caddy/nginx)

## Migrating from OpenClaw Bundled Browser

To replace the bundled `browser` plugin with Pinchtab:

### 1. Disable bundled browser
```json5
{
  plugins: {
    deny: ["browser"],  // disable bundled
    entries: {
      pinchtab: { enabled: true }
    }
  }
}
```

### 2. Action mapping

| OpenClaw `browser` | Pinchtab equivalent |
|--------------------|---------------------|
| `browser.open(url)` | `browser({ action: "navigate", url })` |
| `browser.snapshot()` | `browser({ action: "snapshot" })` |
| `browser.screenshot()` | `browser({ action: "screenshot" })` |
| `browser.act({ kind: "click", ref })` | `browser({ action: "click", ref })` |
| `browser.act({ kind: "type", ref, text })` | `browser({ action: "type", ref, text })` |
| `browser.tabs()` | `browser({ action: "tabs" })` |
| `browser.status()` | `browser({ action: "status" })` |

### 3. Profile mapping

| OpenClaw profile | Pinchtab config |
|------------------|-----------------|
| `openclaw` (default) | Default isolated profile |
| `user` | `{ attach: true }` - existing session |
| Custom CDP | `profiles: { "name": { instanceId: "..." } }` |

### 4. Key differences

- **Auto-start**: Pinchtab auto-starts locally by default (disable with `autoStart: false`)
- **Policy**: `allowEvaluate`, `allowDownloads`, `allowUploads` are `false` by default
- **Advanced actions**: Use `pinchtab` tool for mouse controls, wait, handoff, evaluate

## Requirements

- Pinchtab binary in PATH (or set `binaryPath`)
- OpenClaw Gateway
