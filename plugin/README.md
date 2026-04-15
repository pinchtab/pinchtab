# Pinchtab OpenClaw Plugin

Browser control for AI agents via [Pinchtab](https://pinchtab.com). Single-tool design — one `pinchtab` tool handles all browser operations. Minimal context bloat.

## Install

```bash
openclaw plugins install @pinchtab/pinchtab
openclaw gateway restart
```

## Prerequisites

```bash
# Start Pinchtab
pinchtab &

# With auth token (recommended)
PINCHTAB_TOKEN=my-secret pinchtab &

# Docker
docker run -d -p 9867:9867 ghcr.io/pinchtab/pinchtab:latest
```

## Configure

```json5
{
  plugins: {
    entries: {
      pinchtab: {
        enabled: true,
        config: {
          baseUrl: "http://localhost:9867",
          token: "my-secret",
          timeout: 30000,
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

## Single Tool, All Actions

One tool definition, many actions — keeps context lean:

| Action | Description | Typical tokens |
|---|---|---|
| `navigate` | Go to URL | — |
| `snapshot` | Accessibility tree (refs for interactions) | ~3,600 (interactive) |
| `click/type/press/fill/hover/scroll/select/focus` | Act on element by ref | — |
| `mousemove/mousedown/mouseup/mousewheel` | Low-level mouse controls by ref/selector/coordinates | — |
| `wait` | Wait for selector/text/url/load/fn/ms conditions | — |
| `handoff` | Human-in-the-loop pause/resume for CAPTCHA/login/2FA | — |
| `text` | Extract readable text (cheapest) | ~800 |
| `tabs` | List/open/close tabs | — |
| `screenshot` | JPEG screenshot (vision fallback) | ~2K |
| `evaluate` | Run JavaScript in page | — |
| `pdf` | Export page as PDF | — |
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
3. pinchtab({ action: "mousemove", ref: "e5" })
4. pinchtab({ action: "mousedown", ref: "e5", button: "left" })
5. pinchtab({ action: "mouseup", ref: "e5", button: "left" })
6. pinchtab({ action: "mousewheel", ref: "e5", wheelDeltaY: 240 })
```

Coordinate-driven test (viewport):

```
pinchtab({ action: "mousemove", x: 400, y: 300 })
pinchtab({ action: "mousedown", x: 400, y: 300, button: "left" })
pinchtab({ action: "mouseup", x: 400, y: 300, button: "left" })
pinchtab({ action: "mousewheel", x: 400, y: 300, wheelDeltaY: -320 })
```

**Token strategy:** `text` for reading, `snapshot` with `filter=interactive&format=compact` for interactions, `diff=true` on subsequent snapshots, `screenshot` only for visual verification.

## Human Handoff (CAPTCHA / Login / 2FA)

Use `handoff` when manual intervention is required, then resume with a wait condition:

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

- **`evaluate`** executes arbitrary JavaScript in the page — restrict to trusted agents and domains
- Use `PINCHTAB_TOKEN` to gate API access; rotate regularly
- In production, run behind HTTPS reverse proxy (Caddy/nginx)

## Requirements

- Running Pinchtab instance (Go binary or Docker)
- OpenClaw Gateway
