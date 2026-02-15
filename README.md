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

AI agents need browsers. Current options are either:
- **Screenshot-based** — expensive (vision model tokens), slow, unreliable
- **Framework-locked** — MCP-only, Python-only, requires specific agent
- **Cloud-first** — want you on their servers, not your machine

Pinchtab is different:
- **Accessibility tree as primary interface** — 4x cheaper than screenshots, works with any LLM
- **Plain HTTP API** — any agent, any language, even `curl`
- **Self-contained** — launches its own Chrome, manages the process
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

## Compared To

| | Pinchtab | Steel Browser | Playwright MCP | OpenClaw Browser |
|---|---|---|---|---|
| A11y snapshots | ✅ | ❌ | ✅ | ✅ |
| Element interaction | ✅ | ❌ | ✅ | ✅ |
| Interface | HTTP | HTTP | MCP | Internal |
| Any agent | ✅ | ✅ | ❌ | ❌ |
| Stealth mode | ✅ | ✅ | ❌ | ❌ |
| Session persistence | ✅ | ✅ | ❌ | ❌ |
| Self-launching Chrome | ✅ | Docker | ❌ | ✅ |
| Single binary | ✅ 12MB | ❌ | ❌ | ❌ |
| Lines of code | ~600 | ~12,000 | ~5,000 | — |

## Built With

| Dependency | What it does | License |
|---|---|---|
| [chromedp](https://github.com/chromedp/chromedp) | Chrome DevTools Protocol driver for Go | MIT |
| [cdproto](https://github.com/chromedp/cdproto) | Generated CDP types and commands | MIT |
| [gobwas/ws](https://github.com/gobwas/ws) | Low-level WebSocket (used by chromedp) | MIT |
| [go-json-experiment/json](https://github.com/go-json-experiment/json) | JSON v2 library (used by cdproto) | BSD-3-Clause |

Everything else is Go standard library.

## Works with OpenClaw

<p align="center">
  <img src="assets/pinchtab-mascot-transparent.png" alt="Works with OpenClaw" width="150"/>
</p>

Pinchtab is built to work seamlessly with [OpenClaw](https://openclaw.ai) — the open-source personal AI assistant. Use Pinchtab as your agent's browser backend for faster, cheaper web automation.

## License

MIT — [Giago Software Ltd](https://giago.co)
