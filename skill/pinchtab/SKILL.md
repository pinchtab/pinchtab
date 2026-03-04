---
name: pinchtab
description: Control a headless or headed Chrome browser via Pinchtab's HTTP API for web automation, scraping, form filling, navigation, screenshots, and extraction with stable accessibility refs.
metadata:
  short-description: Browser automation via Pinchtab HTTP API
---

# Pinchtab

Fast, lightweight browser control for AI agents via HTTP + accessibility tree.

**Security Note:** Pinchtab runs entirely locally. It does not contact external services, send telemetry, or exfiltrate data. However, it controls a real Chrome instance — if pointed at a profile with saved logins, agents can access authenticated sites. Always use a dedicated empty profile and set BRIDGE_TOKEN when exposing the API. See [TRUST.md](TRUST.md) for the full security model.

## Quick Start (Agent Workflow)

The 30-second pattern for browser tasks:

```bash
# 1. Start Pinchtab server (runs forever, local on :9867)
pinchtab

# 2. In your agent, use HTTP API via curl:
#    a) Navigate to a URL
#    b) Snapshot the page (get refs like e0, e5, e12)
#    c) Act on a ref (click e5, type e12 "search text")
#    d) Snapshot again to see the result
#    e) Repeat step c-d until done
```

**That's it.** Refs are stable—you don't need to re-snapshot before every action. Only snapshot when the page changes significantly.

### Recommended Secure Setup

```bash
# Best practice for AI agents
BRIDGE_BIND=127.0.0.1 \
BRIDGE_TOKEN="your-strong-secret" \
BRIDGE_PROFILE=~/.pinchtab/automation-profile \
pinchtab &
```

**Never expose to 0.0.0.0 without a token. Never point at your daily Chrome profile.**

## Setup

```bash
# Start server (headless by default)
pinchtab

# Or with environment variables:
BRIDGE_HEADLESS=false pinchtab          # Headed mode (visible window)
BRIDGE_TOKEN="secret-key" pinchtab      # With auth token
BRIDGE_PORT=8080 pinchtab               # Custom port
```

Default: **port 9867**, no auth required (local). Set `BRIDGE_TOKEN` for remote access.

For advanced setup (profiles, env vars, config), see [docs/references/configuration.md](docs/references/configuration.md).

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

The typical agent loop (via HTTP API):

1. **Navigate** — POST /navigate with URL
2. **Snapshot** — GET /snapshot to get accessibility tree with refs
3. **Act** — POST /action on refs (click, type, press)
4. **Snapshot** — GET /snapshot again to see results

Refs (e.g. `e0`, `e5`, `e12`) are cached per tab after each snapshot — no need to re-snapshot before every action unless the page changed significantly.

### Quick examples (curl)

```bash
# Navigate to a URL
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

# Get page snapshot (interactive + compact for efficiency)
curl "http://localhost:9867/snapshot?filter=interactive&compact=true"

# Find elements by text/role/label (before acting)
curl -X POST http://localhost:9867/find \
  -H "Content-Type: application/json" \
  -d '{"text":"Sign In"}'

# Click an element by ref
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'

# Type text
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"type","ref":"e12","text":"hello world"}'

# Extract readable text (~800 tokens)
curl http://localhost:9867/text | jq .text

# Take screenshot
curl http://localhost:9867/screenshot > page.png

# Run JavaScript
curl -X POST http://localhost:9867/evaluate \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.title"}'
```

For the full HTTP API (batch actions, downloads, uploads, cookies, stealth, PDF export), see the [API Reference](docs/references/endpoints.md).

## Token Optimization

Most efficient endpoints:

- **`/find`** — ~800 tokens, search for elements by text/role/label
- **`/text`** — ~800 tokens, extract readable page content

Use `/find` to locate elements precisely. Use `/text` to read page content. Both are minimal token cost compared to `/snapshot`.

---

## 3-Second Wait Pattern

After navigation, wait for Chrome to render the accessibility tree:

```bash
# Navigate
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

# Wait 3+ seconds for page render
sleep 3

# Then snapshot or find elements
curl "http://localhost:9867/snapshot?filter=interactive&compact=true"
curl -X POST http://localhost:9867/find -d '{"text":"Sign In"}'
```

This ensures the accessibility tree is complete before you snapshot or search.

## Configuration (Optional)

For setup and advanced options:

```bash
# Initialize config file (optional)
pinchtab config init

# Show current configuration
pinchtab config show

# Set a value (e.g., custom port)
pinchtab config set server.port 9999

# Validate configuration
pinchtab config validate
```

See [docs/references/cli-quick-reference.md](docs/references/cli-quick-reference.md) for full CLI reference.

## Tips

- **Always pass `tabId` explicitly** when working with multiple tabs
- Refs are stable between snapshot and actions — no need to re-snapshot before clicking
- After navigation or major page changes, take a new snapshot for fresh refs
- Pinchtab persists sessions — tabs survive restarts (disable with `BRIDGE_NO_RESTORE=true`)
- Chrome profile is persistent — cookies/logins carry over between runs
- Use `"blockImages": true` on navigate for read-heavy tasks
- **Wait 3+ seconds after navigate before snapshot** — Chrome needs time to render accessibility tree
