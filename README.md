<p align="center">
  <img src="assets/pinchtab-mascot.png" alt="pinchtab" width="200"/>
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
curl localhost:9867/text?tabId=X

# Click a button by ref from the last snapshot
curl -X POST localhost:9867/action -d '{"kind":"click","ref":"e5"}'
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
- **Stealth mode** — bypasses bot detection on major sites
- **Persistent sessions** — log in once, stays logged in across restarts

## Quick Start

### Docker (easiest)

```bash
docker run -d -p 9867:9867 --security-opt seccomp=unconfined pinchtab/pinchtab
curl http://localhost:9867/health
```

### With your AI agent

> Install Pinchtab and set it up for browser automation.

Your agent can clone, build, and configure Pinchtab using the [OpenClaw skill](skill/pinchtab/SKILL.md). Just ask.

### Manual

```bash
# Build
go build -o pinchtab .

# Headed mode (default) — Chrome window visible, human can watch and interact
./pinchtab

# Headless mode — no window, pure automation
BRIDGE_HEADLESS=true ./pinchtab
```

### Headless Mode (recommended)

The primary mode. Chrome runs invisibly in the background — no window, pure API. This is what Pinchtab is built and tested for. Best for servers, CI, Docker, and unattended automation.

```bash
BRIDGE_HEADLESS=true ./pinchtab
```

All 100+ tests run against headless. The full API surface is validated here.

### Headed Mode (experimental)

Chrome opens as a visible window. Useful for debugging, watching agents work, and manually logging into sites. Shows a 🦀 welcome page on startup.

```bash
./pinchtab  # headed is the default
```

> **⚠️ Headed mode is not fully tested.** It works for basic use but expect rough edges:
>
> - **Profile management is manual** — Pinchtab uses its own Chrome profile (`~/.pinchtab/chrome-profile/`), separate from your regular Chrome. To access sites that need login, you must either log in manually in the Pinchtab window, or copy your existing Chrome profile:
>   ```bash
>   # Copy your Chrome profile (while Chrome is closed)
>   cp -r ~/Library/Application\ Support/Google/Chrome/Default ~/.pinchtab/chrome-profile
>   # Or point to a custom location
>   BRIDGE_PROFILE=/path/to/profile ./pinchtab
>   ```
> - **Two Chrome instances can't share a profile** — if your regular Chrome is open, you must use a copied profile, not the original
> - **Some sites detect automation differently in headed mode** — stealth behaviour may vary
> - **Window management is not handled** — Chrome opens wherever the OS puts it

### First-Time Login

Pinchtab uses a persistent profile at `~/.pinchtab/chrome-profile/`. In headed mode, log into sites via the Chrome window that opens — cookies persist across restarts. In headless mode, either copy an existing profile or use the cookie API (`POST /cookies`) to inject session cookies programmatically.

## Features

- 🌲 **Accessibility-first** — structured tree with stable refs (`e0`, `e1`...) for click, type, read
- 🎯 **Smart filters** — `?filter=interactive` returns only buttons/links/inputs (~75% fewer tokens)
- 🖱️ **Direct actions** — click, type, fill, press, focus, hover, select, scroll by ref or CSS selector
- 🕵️ **Stealth mode** — patches `navigator.webdriver`, spoofs UA, hides automation flags
- 💾 **Session persistence** — cookies, auth, tabs survive restarts
- 🚫 **Image blocking** — skip image downloads for faster, leaner browsing (`BRIDGE_BLOCK_IMAGES` or per-request)
- 🎬 **Animation disabling** — freeze CSS animations/transitions for consistent snapshots (`BRIDGE_NO_ANIMATIONS` or `?noAnimations=true`)
- 📝 **Text extraction** — readability mode (strips nav/ads) or raw `innerText`
- 🔄 **Smart diff** — `?diff=true` returns only changes since last snapshot
- 📄 **Text format** — `?format=text` for indented tree (~40-60% fewer tokens than JSON)
- ⚡ **JS evaluation** — escape hatch for anything the API doesn't cover
- 📸 **Screenshots** — JPEG with quality control for visual verification

## Full API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Connection status |
| `GET` | `/tabs` | List open tabs |
| `GET` | `/snapshot` | Accessibility tree (primary interface) |
| `GET` | `/screenshot` | JPEG screenshot (opt-in) |
| `GET` | `/text` | Readable page text (readability or raw) |
| `POST` | `/navigate` | Go to URL |
| `POST` | `/action` | Click, type, fill, press, focus, hover, select, scroll |
| `POST` | `/evaluate` | Execute JavaScript |
| `POST` | `/tab` | Open/close tabs |
| `POST` | `/tab/lock` | Lock tab for exclusive agent access |
| `POST` | `/tab/unlock` | Release tab lock |

### Query Parameters (snapshot)
| Param | Description |
|-------|-------------|
| `tabId` | Target tab (default: first tab) |
| `filter=interactive` | Only buttons, links, inputs |
| `depth=N` | Max tree depth |
| `diff=true` | Return only added/changed/removed nodes since last snapshot |
| `format=text` | Indented plain text instead of JSON (~40-60% fewer tokens) |
| `format=compact` | One-line-per-node format (56-64% fewer tokens than JSON) |
| `selector=CSS` | Scope tree to a CSS selector subtree (e.g. `?selector=main`) |
| `maxTokens=N` | Truncate output to ~N tokens |
| `noAnimations=true` | Disable CSS animations before capture |
| `output=file` | Save snapshot to disk instead of returning |
| `path=/custom/path` | Custom file path (with `output=file`) |

### Query Parameters (screenshot)
| Param | Description |
|-------|-------------|
| `tabId` | Target tab (default: first tab) |
| `quality=N` | JPEG quality (default: 80) |
| `noAnimations=true` | Disable CSS animations before capture |
| `output=file` | Save screenshot to disk instead of returning |

### Query Parameters (text)
| Param | Description |
|-------|-------------|
| `tabId` | Target tab (default: first tab) |
| `mode=raw` | Raw `innerText` instead of readability extraction |

## Configuration

All via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `BRIDGE_PORT` | `9867` | HTTP server port |
| `BRIDGE_TOKEN` | *(none)* | Bearer token for auth |
| `BRIDGE_HEADLESS` | `false` | Run Chrome in headless mode (no window). Default is headed (visible). |
| `BRIDGE_PROFILE` | `~/.pinchtab/chrome-profile` | Chrome profile directory |
| `BRIDGE_STATE_DIR` | `~/.pinchtab` | State/session storage |
| `BRIDGE_NO_RESTORE` | `false` | Skip restoring tabs from previous session |
| `BRIDGE_CHROME_VERSION` | `133.0.6943.98` | Chrome version in User-Agent strings (keep current to avoid detection) |
| `BRIDGE_BLOCK_IMAGES` | `false` | Block image loading (faster, lower bandwidth) |
| `BRIDGE_NO_ANIMATIONS` | `false` | Disable CSS animations/transitions globally |
| `BRIDGE_TIMEOUT` | `15` | Action timeout (seconds) |
| `BRIDGE_NAV_TIMEOUT` | `30` | Navigation timeout (seconds) |
| `BRIDGE_CHROME_VERSION` | `133.0.6943.98` | Chrome UA version string |
| `BRIDGE_CONFIG` | `~/.pinchtab/config.json` | Path to config JSON file |
| `CDP_URL` | *(none)* | Connect to existing Chrome instead of launching |

## Architecture

```
┌─────────────┐     HTTP :9867    ┌──────────────┐                ┌─────────┐
│  Any Agent  │ ──────────────►   │  Pinchtab    │  ── CDP ──►    │ Chrome  │
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

- **Go 1.24+** (build from source) or download a [prebuilt binary](https://github.com/pinchtab/pinchtab/releases)
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

# Run tests (38 tests)
go test ./...
```

## ⚠️ Security — Read This

**Pinchtab gives AI agents full control of a real browser with your real accounts.**

When you log into sites through Pinchtab's Chrome window, those sessions — cookies, tokens, saved passwords — persist in `~/.pinchtab/chrome-profile/`. Any agent with HTTP access to Pinchtab can then act as you: read your email, post on your behalf, make purchases, access sensitive data.

**This is by design.** That's what makes it useful. But it means:

- **You are responsible for what agents do with your accounts.** Pinchtab is a tool, not a guardrail. If you give an agent access to your bank and it does something stupid, that's on you.
- **Set `BRIDGE_TOKEN`** — without it, anyone on your network can control your browser. In production, this is non-negotiable.
- **Treat `~/.pinchtab/` as sensitive** — it contains your Chrome profile with all saved sessions and cookies. Guard it like you'd guard your passwords.
- **Pinchtab binds to all interfaces by default** — use a firewall or reverse proxy if you're on a shared network.
- **Start with low-risk accounts.** Don't point an experimental agent at your primary email or bank account on day one. Test with throwaway accounts first.
- **No data leaves your machine** — all processing is local. But the agents you connect might send data wherever they want.

Think of Pinchtab like giving someone your unlocked laptop. Powerful if you trust them. Dangerous if you don't.

## Contributors

### Humans

<a href="https://github.com/luigi-agosti">
  <img src="https://github.com/luigi-agosti.png" width="60" style="border-radius:50%" alt="Luigi Agosti"/><br/>
  <sub><b>Luigi Agosti</b> · Agent Manager</sub>
</a>

### Agents

| | |
|:---:|:---:|
| <img src="assets/bosch-avatar.png" width="48"/> | <img src="assets/mario-avatar.png" width="48"/> |
| **Bosch** | **Mario** |
| Debugging the simulation 🔮 | First principles · ships fast |

## Works with OpenClaw

Pinchtab is built to work seamlessly with [OpenClaw](https://openclaw.ai) — the open-source personal AI assistant. Use Pinchtab as your agent's browser backend for faster, cheaper web automation.

<p align="right">
  <img src="assets/openclaw-logo.svg" alt="OpenClaw" height="120"/>
  <img src="assets/pinchtab-icon.png" alt="Pinchtab" height="80"/>
</p>

## Star History

<a href="https://star-history.com/#pinchtab/pinchtab&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=pinchtab/pinchtab&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=pinchtab/pinchtab&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=pinchtab/pinchtab&type=Date" />
 </picture>
</a>

