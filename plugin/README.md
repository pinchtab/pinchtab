# Pinchtab OpenClaw Plugin

Browser control for AI agents via [Pinchtab](https://pinchtab.com). Gives agents structured tool calls instead of raw HTTP/curl — lower token cost, stable refs, no shell escaping.

## Install

**From npm (recommended):**
```bash
openclaw plugins install @pinchtab/openclaw-plugin
openclaw gateway restart
```

**From local path (development):**
```bash
openclaw plugins install -l ./plugin
openclaw gateway restart
```

## Prerequisites

Start Pinchtab before using the plugin:

```bash
# Headless (default)
pinchtab &

# With auth token (recommended)
BRIDGE_TOKEN=my-secret pinchtab &

# Docker
docker run -d -p 9867:9867 ghcr.io/pinchtab/pinchtab:latest
```

## Configure

Add to your OpenClaw config:

```json5
{
  plugins: {
    entries: {
      pinchtab: {
        enabled: true,
        config: {
          baseUrl: "http://localhost:9867",  // default
          token: "my-secret",               // matches BRIDGE_TOKEN
          timeout: 30000,                   // ms, default 30s
        },
      },
    },
  },
  // Enable tools for your agent
  agents: {
    list: [{
      id: "main",
      tools: { allow: ["pinchtab"] },
    }],
  },
}
```

## Tools

| Tool | Description | Typical tokens |
|---|---|---|
| `pinchtab_navigate` | Navigate to a URL | — |
| `pinchtab_snapshot` | Accessibility tree with filter/format/diff | ~3,600 (interactive) |
| `pinchtab_action` | Click, type, press, fill, hover, scroll, select, focus | — |
| `pinchtab_text` | Extract readable text (cheapest) | ~800 |
| `pinchtab_tabs` | List/open/close tabs | — |
| `pinchtab_screenshot` | JPEG screenshot (vision fallback) | ~2K |
| `pinchtab_evaluate` | Run JavaScript in page | — |
| `pinchtab_health` | Check connectivity | — |

All tools are optional (opt-in via agent allowlist). Never auto-enabled.

## Agent Usage Example

A typical agent browsing loop:

```
1. pinchtab_navigate({ url: "https://example.com/search" })
2. pinchtab_snapshot({ filter: "interactive", format: "compact" })
   → Returns refs like e0, e5, e12 for each interactive element
3. pinchtab_action({ kind: "click", ref: "e5" })
   → Clicks the search input
4. pinchtab_action({ kind: "type", ref: "e5", text: "pinchtab" })
5. pinchtab_action({ kind: "press", key: "Enter" })
6. pinchtab_snapshot({ diff: true, format: "compact" })
   → Only returns what changed (saves tokens)
7. pinchtab_text()
   → Extract readable results (~800 tokens)
```

**Token strategy:** Use `pinchtab_text` for reading content. Use `pinchtab_snapshot` with `filter=interactive&format=compact` when you need to click/type. Use `diff=true` on subsequent snapshots. Use `pinchtab_screenshot` only for visual verification.

## Requirements

- Running Pinchtab instance (Go binary or Docker)
- OpenClaw Gateway
