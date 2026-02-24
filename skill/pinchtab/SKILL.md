---
name: pinchtab
description: >
  Control a headless or headed Chrome browser via Pinchtab's HTTP API. Use for web automation,
  scraping, form filling, navigation, and multi-tab workflows. Pinchtab exposes the accessibility
  tree as flat JSON with stable refs — optimized for AI agents (low token cost, fast).
  Use when the task involves: browsing websites, filling forms, clicking buttons, extracting
  page text, taking screenshots, or any browser-based automation. Requires a running Pinchtab
  instance (Go binary).
homepage: https://pinchtab.com
metadata:
  openclaw:
    emoji: "🦀"
    requires:
      bins: ["pinchtab"]
      env:
        - name: BRIDGE_TOKEN
          secret: true
          optional: true
          description: "Bearer auth token for Pinchtab API"
        - name: BRIDGE_PORT
          optional: true
          description: "HTTP port (default: 9867)"
        - name: BRIDGE_HEADLESS
          optional: true
          description: "Run Chrome headless (true/false)"
---

# Pinchtab

Fast, lightweight browser control for AI agents via HTTP + accessibility tree.

## Setup

```bash
# Headless (default)
pinchtab &

# Headed — visible Chrome for human + agent workflows
BRIDGE_HEADLESS=false pinchtab &

# Dashboard/orchestrator — profile manager + launcher
pinchtab dashboard &
```

Default port: `9867`. Auth: set `BRIDGE_TOKEN=<secret>` and pass `Authorization: Bearer <secret>`.

For dashboard/profile workflows, see [references/profiles.md](references/profiles.md).
For all environment variables, see [references/env.md](references/env.md).

## Core Workflow

The typical agent loop:

1. **Navigate** to a URL
2. **Snapshot** the accessibility tree (get refs)
3. **Act** on refs (click, type, press)
4. **Snapshot** again to see results

Refs (e.g. `e0`, `e5`, `e12`) are cached per tab after each snapshot — no need to re-snapshot before every action unless the page changed significantly.

### Quick examples (CLI)

```bash
pinchtab nav https://example.com
pinchtab snap -i -c                    # interactive + compact
pinchtab click e5
pinchtab type e12 hello world
pinchtab press Enter
pinchtab text                          # readable text (~1K tokens)
pinchtab text | jq .text               # pipe to jq
```

### Quick examples (curl)

```bash
curl -X POST http://localhost:9867/navigate \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com"}'

curl "http://localhost:9867/snapshot?filter=interactive&format=compact"

curl -X POST http://localhost:9867/action \
  -H 'Content-Type: application/json' \
  -d '{"kind": "click", "ref": "e5"}'

curl -X POST http://localhost:9867/action \
  -H 'Content-Type: application/json' \
  -d '{"kind": "type", "ref": "e12", "text": "hello world"}'

curl http://localhost:9867/text
```

For the full API (download, upload, screenshot, evaluate, tabs, cookies, stealth, batch actions), see [references/api.md](references/api.md).

## Token Cost Guide

| Method | Typical tokens | When to use |
|---|---|---|
| `/text` | ~800 | Reading page content |
| `/snapshot?filter=interactive` | ~3,600 | Finding buttons/links to click |
| `/snapshot?diff=true` | varies | Multi-step workflows (only changes) |
| `/snapshot?format=compact` | ~56-64% less | One-line-per-node, best efficiency |
| `/snapshot` | ~10,500 | Full page understanding |
| `/screenshot` | ~2K (vision) | Visual verification |

**Strategy**: Start with `?filter=interactive&format=compact`. Use `?diff=true` on subsequent snapshots. Use `/text` when you only need readable content. Full `/snapshot` only when needed.

## Tips

- **Always pass `tabId` explicitly** when working with multiple tabs
- Refs are stable between snapshot and actions — no need to re-snapshot before clicking
- After navigation or major page changes, take a new snapshot for fresh refs
- Pinchtab persists sessions — tabs survive restarts (disable with `BRIDGE_NO_RESTORE=true`)
- Chrome profile is persistent — cookies/logins carry over between runs
- Use `BRIDGE_BLOCK_IMAGES=true` or `"blockImages": true` on navigate for read-heavy tasks
