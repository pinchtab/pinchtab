<p align="center">
  <img src="assets/pinchtab-mascot-transparent.png" alt="pinchtab" width="200"/>
</p>

<p align="center">
  <strong>Browser control for AI agents.</strong><br/>
  12MB Go binary. Zero config. Accessibility-first.<br/><br/>
  🦀 <em>PINCH! PINCH!</em>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/lang-Go-00ADD8?style=flat-square" alt="Go"/>
  <img src="https://img.shields.io/badge/binary-12MB-FFD700?style=flat-square" alt="12MB"/>
  <img src="https://img.shields.io/badge/interface-HTTP-00ff88?style=flat-square" alt="HTTP"/>
  <img src="https://img.shields.io/badge/license-MIT-888?style=flat-square" alt="MIT"/>
</p>

---

## Why

Most agent browser tools (OpenClaw, Playwright MCP, Browser Use) are tightly coupled — they only work inside their own framework. If you switch agents or want to script something in bash, you're out of luck.

Pinchtab is a standalone HTTP server. Any agent, any language, even `curl`:

```bash
# Read a page — 800 tokens instead of 10,000
curl localhost:18800/text?tabId=X

# Click a button by ref from the last snapshot
curl -X POST localhost:18800/action -d '{"kind":"click","ref":"e5"}'
```

| | Pinchtab | OpenClaw Browser |
|---|---|---|
| **Tokens per page** | **~800** (`/text`) / ~3,600 (interactive) | ~10,000+ (full snapshot) |
| Interface | HTTP — any agent, any language | Internal only |
| A11y snapshots | ✅ | ✅ |
| Element interaction | ✅ | ✅ |
| Stealth mode | ✅ | ❌ |
| Session persistence | ✅ | ❌ |
| Self-contained binary | ✅ 12MB | ❌ |

- **5-13x cheaper** than screenshots or full snapshots for read-heavy tasks ([real measurements](#token-efficiency--real-numbers))
- **Plain HTTP API** — not locked to any agent framework
- **Self-contained** — 12MB binary, launches its own Chrome, zero config
- **Stealth mode** — bypasses bot detection (Google, X/Twitter, etc.)
- **Persistent sessions** — log in once, stays logged in across restarts

## Quick Start

```bash
# Build
go build -o pinchtab .

# Run (launches Chrome window — you can see and interact)
./pinchtab

# Run headless
BRIDGE_HEADLESS=true ./pinchtab
```

Chrome opens. You log into your sites. Agents drive the rest.

### First-Time Login

Pinchtab launches its own Chrome with a persistent profile at `~/.browser-bridge/chrome-profile/`. The first time you run it, you'll need to log into any sites you want agents to access (X/Twitter, Google, etc.) — just do it in the Chrome window that opens. Cookies and sessions persist across restarts, so you only need to do this once.

## Features

### 🌲 Accessibility-First Snapshots
The primary interface. Returns a structured tree of every element on the page with stable refs (`e0`, `e1`, `e2`...) that agents can click, type into, or read.

```bash
curl localhost:18800/snapshot?tabId=X
```
```json
{
  "url": "https://x.com/search?q=%24hims",
  "title": "$hims - Search / X",
  "count": 47,
  "nodes": [
    {"ref": "e0", "role": "searchbox", "name": "Search query", "nodeId": 206},
    {"ref": "e1", "role": "link", "name": "Deep Value Investing @DeepIceValue", "nodeId": 412},
    {"ref": "e2", "role": "button", "name": "Like", "nodeId": 445}
  ]
}
```

### 🎯 Smart Filters
Don't waste tokens on 200 nodes when you need 10 buttons:
```bash
# Only interactive elements (buttons, links, inputs)
curl localhost:18800/snapshot?tabId=X&filter=interactive

# Limit tree depth
curl localhost:18800/snapshot?tabId=X&depth=3
```

### 🖱️ Direct Actions
Click, type, fill — by accessibility ref or CSS selector:
```bash
# Click by ref
curl -X POST localhost:18800/action -d '{"tabId":"X","kind":"click","ref":"e5"}'

# Type in a field
curl -X POST localhost:18800/action -d '{"tabId":"X","kind":"type","ref":"e0","text":"$hims"}'

# Press a key
curl -X POST localhost:18800/action -d '{"tabId":"X","kind":"press","key":"Enter"}'

# By CSS selector
curl -X POST localhost:18800/action -d '{"tabId":"X","kind":"click","selector":"button.submit"}'
```

### 🕵️ Stealth Mode
Pinchtab patches `navigator.webdriver`, spoofs user agent, hides automation flags. Sites like X.com and Google treat it as a normal browser. Log in, stay logged in.

### 💾 Session Persistence
Chrome profile saved at `~/.browser-bridge/chrome-profile/`. Cookies, localStorage, auth tokens — all persist across restarts. Tabs are saved to `~/.browser-bridge/sessions.json` on shutdown.

### 📝 Text Extraction
Get raw page text (body innerText) without the tree overhead:
```bash
curl localhost:18800/text?tabId=X
```

### ⚡ JavaScript Evaluation
Escape hatch for anything the API doesn't cover:
```bash
curl -X POST localhost:18800/evaluate \
  -d '{"tabId":"X","expression":"document.querySelectorAll(\".tweet\").length"}'
```

### 📸 Screenshot (Opt-in)
Available when you need visual verification. Not the default:
```bash
curl localhost:18800/screenshot?tabId=X          # base64 JSON
curl localhost:18800/screenshot?tabId=X&raw=true  # raw JPEG
```

## Full API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Connection status |
| `GET` | `/tabs` | List open tabs |
| `GET` | `/snapshot` | Accessibility tree (primary interface) |
| `GET` | `/screenshot` | JPEG screenshot (opt-in) |
| `GET` | `/text` | Readable page text |
| `POST` | `/navigate` | Go to URL |
| `POST` | `/action` | Click, type, fill, press, focus |
| `POST` | `/evaluate` | Execute JavaScript |
| `POST` | `/tab` | Open/close tabs |

### Query Parameters (snapshot)
| Param | Description |
|-------|-------------|
| `tabId` | Target tab (default: first tab) |
| `filter=interactive` | Only buttons, links, inputs |
| `depth=N` | Max tree depth |

## Configuration

All via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `BRIDGE_PORT` | `18800` | HTTP server port |
| `BRIDGE_TOKEN` | *(none)* | Bearer token for auth |
| `BRIDGE_HEADLESS` | `false` | Run Chrome headless |
| `BRIDGE_PROFILE` | `~/.browser-bridge/chrome-profile` | Chrome profile directory |
| `BRIDGE_STATE_DIR` | `~/.browser-bridge` | State/session storage |
| `BRIDGE_NO_RESTORE` | `false` | Skip restoring tabs from previous session |
| `CDP_URL` | *(none)* | Connect to existing Chrome instead of launching |

## Architecture

```
┌─────────────┐     HTTP :18800    ┌──────────────┐                ┌─────────┐
│  Any Agent  │ ──────────────►   │  Pinchtab    │  ── CDP ──►   │ Chrome  │
│  (OpenClaw, │  snapshot, act,   │              │                │         │
│   PicoClaw, │  navigate, eval   │  stealth +   │                │  your   │
│   curl,     │                   │  sessions +  │                │  tabs   │
│   scripts)  │                   │  a11y tree   │                │         │
└─────────────┘                   └──────────────┘                └─────────┘
```

## Why Not Screenshots?

| | Screenshots | Accessibility Tree |
|---|---|---|
| **Tokens** | ~2,000/image | ~200-500/page |
| **Speed** | Render → encode → transfer | Instant structured data |
| **Reliability** | Vision model guesses coordinates | Deterministic refs |
| **LLM requirement** | Vision model required | Any text LLM works |
| **Cost (10-step task)** | ~$0.06 | ~$0.015 |

Playwright MCP, OpenClaw, and Browser Use all default to accessibility trees for the same reason.

## Token Efficiency — Real Numbers

Measured on a live search results page:

| Method | Size | ~Tokens |
|---|---|---|
| Full a11y snapshot | 42 KB | 10,500 |
| Interactive-only (`?filter=interactive`) | 14 KB | 3,600 |
| Text extraction (`/text`) | 3 KB | 800 |
| Screenshot (vision model) | — | ~2,000 |

For read-heavy tasks (monitoring feeds, scraping search results), `/text` at ~800 tokens per page is **5x cheaper** than a full snapshot and **13x cheaper** than the same page via screenshots.

**Example: 50-page search monitoring task**

| Approach | Tokens | Est. cost |
|---|---|---|
| Screenshots (vision) | ~100,000 | $0.30 |
| Full snapshots | ~525,000 | $0.16 |
| Pinchtab `/text` | ~40,000 | $0.01 |
| Pinchtab interactive filter | ~180,000 | $0.05 |

Use `/text` when you only need content. Use `?filter=interactive` when you need to act. Use the full snapshot when you need page structure.

## Built With

| Dependency | What it does | License |
|---|---|---|
| [chromedp](https://github.com/chromedp/chromedp) | Chrome DevTools Protocol driver for Go | MIT |
| [cdproto](https://github.com/chromedp/cdproto) | Generated CDP types and commands | MIT |
| [gobwas/ws](https://github.com/gobwas/ws) | Low-level WebSocket (used by chromedp) | MIT |
| [go-json-experiment/json](https://github.com/go-json-experiment/json) | JSON v2 library (used by cdproto) | BSD-3-Clause |

Everything else is Go standard library.

## Requirements

- **Go 1.21+** (build from source) or download a [prebuilt binary](https://github.com/pinchtab/pinchtab/releases)
- **Google Chrome** or Chromium installed

## Install

```bash
# From source
go install github.com/pinchtab/pinchtab@latest

# Or clone and build
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
go build -o pinchtab .
```

## Development

```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
go build -o pinchtab .
./pinchtab

# Run tests (coming soon)
go test ./...
```

## Security

- Set `BRIDGE_TOKEN` in production — without it, anyone on the network can control your browser
- Chrome profile persists cookies/sessions — treat `~/.browser-bridge/` as sensitive
- Pinchtab binds to all interfaces by default — use a firewall or reverse proxy in exposed environments
- No data leaves your machine — all processing is local

## Contributors

### Humans

<a href="https://github.com/luigi-agosti"><img src="https://github.com/luigi-agosti.png" width="60" style="border-radius:50%" alt="Luigi Agosti"/></a>

### Agents

<a href="https://github.com/luigiagent"><img src="https://github.com/luigiagent.png" width="60" style="border-radius:50%" alt="Bosch"/></a>
<a href="https://github.com/luigiagent"><img src="https://github.com/luigiagent.png" width="60" style="border-radius:50%" alt="Mario"/></a>

## Works with OpenClaw

Pinchtab is built to work seamlessly with [OpenClaw](https://openclaw.ai) — the open-source personal AI assistant. Use Pinchtab as your agent's browser backend for faster, cheaper web automation.

<p align="right">
  <img src="assets/openclaw-logo.svg" alt="OpenClaw" height="120"/>
  <img src="assets/pinchtab-icon.png" alt="Pinchtab" height="80"/>
</p>

