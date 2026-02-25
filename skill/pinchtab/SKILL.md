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

**Security Note:** Pinchtab runs a local Chrome browser under your control. It does not access your credentials, exfiltrate data, or connect to external services. All interactions stay local unless you explicitly navigate to external sites. Binary distributed via [GitHub releases](https://github.com/pinchtab/pinchtab/releases) with checksums. See [TRUST.md](TRUST.md) for full security model and VirusTotal flag explanation.

## Quick Start (Agent Workflow)

The 30-second pattern for browser tasks:

```bash
# 1. Start Pinchtab (runs forever, local on :9867)
pinchtab &

# 2. In your agent, follow this loop:
#    a) Navigate to a URL
#    b) Snapshot the page (get refs like e0, e5, e12)
#    c) Act on a ref (click e5, type e12 "search text")
#    d) Snapshot again to see the result
#    e) Repeat step c-d until done
```

**That's it.** Refs are stable—you don't need to re-snapshot before every action. Only snapshot when the page changes significantly.

## Setup

```bash
# Headless (default) — no visible window
pinchtab &

# Headed — visible Chrome window for human debugging
BRIDGE_HEADLESS=false pinchtab &

# With auth token
BRIDGE_TOKEN="your-secret-token" pinchtab &

# Custom port
BRIDGE_PORT=8080 pinchtab &

# Dashboard/orchestrator — profile manager + tab launcher
pinchtab dashboard &
```

Default: **port 9867**, no auth required (local). Set `BRIDGE_TOKEN` for remote access.

For advanced setup, see [references/profiles.md](references/profiles.md) and [references/env.md](references/env.md).

## What a Snapshot Looks Like

After calling `/snapshot`, you get the page's accessibility tree as JSON—flat list of elements with refs:

```json
{
  "refs": [
    {"id": "e0", "role": "link", "text": "Sign In", "selector": "a[href='/login']"},
    {"id": "e1", "role": "textbox", "label": "Email", "selector": "input[name='email']"},
    {"id": "e2", "role": "button", "text": "Submit", "selector": "button[type='submit']"}
  ],
  "text": "... readable text version of page ...",
  "title": "Login Page"
}
```

Then you act on refs: `click e0`, `type e1 "user@example.com"`, `press e2 Enter`.

## Core Workflow

The typical agent loop:

1. **Navigate** to a URL
2. **Snapshot** the accessibility tree (get refs)
3. **Act** on refs (click, type, press)
4. **Snapshot** again to see results

Refs (e.g. `e0`, `e5`, `e12`) are cached per tab after each snapshot — no need to re-snapshot before every action unless the page changed significantly.

### Quick examples

```bash
pinchtab nav https://example.com
pinchtab snap -i -c                    # interactive + compact
pinchtab click e5
pinchtab type e12 hello world
pinchtab press Enter
pinchtab text                          # readable text (~1K tokens)
pinchtab text | jq .text               # pipe to jq
pinchtab ss -o page.jpg                # screenshot
pinchtab eval "document.title"         # run JavaScript
pinchtab pdf -o page.pdf               # export PDF
```

For the full HTTP API (curl examples, download, upload, cookies, stealth, batch actions), see [references/api.md](references/api.md).

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

## Agent Optimization

**Validated Feb 2026**: Testing with AI agents revealed a critical pattern for reliable, token-efficient scraping.

**See the full guide:** [docs/agent-optimization.md](../../docs/agent-optimization.md)

### Quick Summary

**The 3-second pattern** — wait after navigate before snapshot:

```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' && \
sleep 3 && \
curl http://localhost:9867/snapshot | jq '.nodes[] | select(.name | length > 15) | .name'
```

**Token savings:** 93% reduction (3,842 → 272 tokens) when using prescriptive instructions vs. exploratory agent approach.

For detailed findings, system prompt templates, and site-specific notes, see [docs/agent-optimization.md](../../docs/agent-optimization.md).

## Tips

- **Always pass `tabId` explicitly** when working with multiple tabs
- Refs are stable between snapshot and actions — no need to re-snapshot before clicking
- After navigation or major page changes, take a new snapshot for fresh refs
- Pinchtab persists sessions — tabs survive restarts (disable with `BRIDGE_NO_RESTORE=true`)
- Chrome profile is persistent — cookies/logins carry over between runs
- Use `BRIDGE_BLOCK_IMAGES=true` or `"blockImages": true` on navigate for read-heavy tasks
- **Wait 3+ seconds after navigate before snapshot** — Chrome needs time to render 2000+ accessibility tree nodes
