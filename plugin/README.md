# Pinchtab OpenClaw Plugin

Browser control for AI agents via [Pinchtab](https://pinchtab.com). Single-tool design — one `pinchtab` tool handles all browser operations. Minimal context bloat.

## Install

```bash
openclaw plugins install @pinchtab/openclaw-plugin
openclaw gateway restart
```

## Prerequisites

```bash
# Start Pinchtab
pinchtab &

# With auth token (recommended)
BRIDGE_TOKEN=my-secret pinchtab &

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
| `text` | Extract readable text (cheapest) | ~800 |
| `tabs` | List/open/close tabs | — |
| `screenshot` | JPEG screenshot (vision fallback) | ~2K |
| `evaluate` | Run JavaScript in page | — |
| `pdf` | Export page as PDF | — |
| `health` | Check connectivity | — |

## Agent Usage Example

```
1. pinchtab({ action: "navigate", url: "https://example.com/search" })
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

**Token strategy:** `text` for reading, `snapshot` with `filter=interactive&format=compact` for interactions, `diff=true` on subsequent snapshots, `screenshot` only for visual verification.

## Security Notes

- **`evaluate`** executes arbitrary JavaScript in the page — restrict to trusted agents and domains
- Use `BRIDGE_TOKEN` to gate API access; rotate regularly
- In production, run behind HTTPS reverse proxy (Caddy/nginx)

## Requirements

- Running Pinchtab instance (Go binary or Docker)
- OpenClaw Gateway
