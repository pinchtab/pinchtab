# PinchTab

Welcome to PinchTab — browser control for AI agents, scripts, and automation workflows.

## What is PinchTab?

PinchTab is a **standalone HTTP server** that gives you direct control over a Chrome browser. Any AI agent can use the CLI or HTTP API.

**CLI example:**
```bash
pinchtab nav https://example.com    # Navigate
pinchtab snap -i -c                 # Get interactive elements
pinchtab click e5                   # Click element by ref
```

**HTTP example (realistic flow):**
```bash
# 1. Navigate to URL (returns tabId)
TAB=$(curl -s -X POST http://localhost:9867/tab \
  -d '{"action":"new","url":"https://example.com"}' | jq -r '.tabId')

# 2. Get page structure
curl -s "http://localhost:9867/snapshot?tabId=$TAB&filter=interactive" | jq

# 3. Click element using the tabId
curl -s -X POST http://localhost:9867/action \
  -d "{\"kind\":\"click\",\"ref\":\"e5\",\"tabId\":\"$TAB\"}"
```

---

## Core Characteristics

- **Tab-Centric** — Everything revolves around tabs, not URLs
- **Stateful** — Sessions persist between requests. Log in once, stay logged in across restarts
- **Token Inexpensive** — Text extraction at 800 tokens/page (5-13x cheaper than full snapshots)
- **Flexible Modes** — Run headless, headed, with browser profiles, or connect to external Chrome via CDP
- **Monitoring & Control** — Tab locking for multi-agent safety, stealth mode for bot detection bypass

---

## Key Features

- 🌲 **Accessibility Tree** — Structured DOM with stable refs (e0, e1...) for click, type, read. No coordinate guessing.
- 🎯 **Smart Filters** — `?filter=interactive` returns only buttons, links, inputs. Fewer tokens per snapshot.
- 🕵️ **Stealth Mode** — Patches `navigator.webdriver`, spoofs UA, hides automation flags for bot detection bypass.
- 📝 **Text Extraction** — Readability mode (clean) or raw (full HTML). Choose based on workflow.
- 🖱️ **Direct Actions** — Click, type, fill, press, focus, hover, select, scroll by ref or selector.
- ⚡ **JavaScript Execution** — Run arbitrary JS in any tab. Escape hatch for workflow gaps.
- 📸 **Screenshots** — JPEG output with quality control.
- 📄 **PDF Export** — Full pages to PDF with headers, footers, landscape mode.
- 🎭 **Multi-Tab** — Create, switch, close tabs. Work with multiple pages concurrently.

---

## Architecture

PinchTab sits between your tools/agents and Chrome:

```text
┌─────────────────────────────────────────┐
│         Your Tool/Agent                  │
│   (CLI, curl, Python, Node.js, etc.)    │
└──────────────┬──────────────────────────┘
               │
               │ HTTP
               ↓
┌─────────────────────────────────────────┐
│    PinchTab HTTP Server (Go)            │
│  ┌─────────────────────────────────┐    │
│  │  Tab Manager                     │    │
│  │  (tracks tabs + sessions)        │    │
│  └─────────────────────────────────┘    │
│  ┌─────────────────────────────────┐    │
│  │  Chrome DevTools Protocol (CDP) │    │
│  └─────────────────────────────────┘    │
└──────────────┬──────────────────────────┘
               │
               │ CDP WebSocket
               ↓
┌─────────────────────────────────────────┐
│        Chrome Browser                   │
│  (Headless, headed, or external)        │
└─────────────────────────────────────────┘
```

---

## Core Concepts

### Tab-Centric Design

Every operation targets a specific tab by `tabId`. Create a tab first:

```bash
curl -X POST http://localhost:9867/tab \
  -d '{"action":"new","url":"https://example.com"}' | jq '.tabId'
# Returns: "abc123"
```

Then use that `tabId` for all subsequent operations:

```bash
# Snapshot: curl "http://localhost:9867/snapshot?tabId=abc123"
# Text:     curl "http://localhost:9867/text?tabId=abc123"
# Action:   curl -X POST http://localhost:9867/action \
#             -d '{"kind":"click","ref":"e5","tabId":"abc123"}'
```

### Refs Instead of Coordinates

The accessibility tree provides **stable element references** instead of pixel coordinates:

```json
{
  "elements": [
    {"ref": "e0", "role": "heading", "name": "Title"},
    {"ref": "e5", "role": "button", "name": "Submit"},
    {"ref": "e8", "role": "input", "name": "Email"}
  ]
}
```

Click or interact by ref:

```bash
curl -X POST http://localhost:9867/action \
  -d '{"kind":"click","ref":"e5","tabId":"abc123"}'
```

### Persistent Sessions

Tabs, cookies, and login state survive server restarts:

```bash
# Login
pinchtab nav https://example.com/login
pinchtab fill e3 user@example.com
pinchtab fill e5 password
pinchtab click e7

# Restart the server
pkill pinchtab
sleep 2
./pinchtab

# Tab is restored, still logged in
pinchtab nav https://example.com/dashboard
pinchtab snap  # Works without re-login
```

---

## Support & Community

- **GitHub Issues** — https://github.com/pinchtab/pinchtab/issues
- **Discussions** — https://github.com/pinchtab/pinchtab/discussions
- **Twitter/X** — [@pinchtabdev](https://x.com/pinchtabdev)

---

## License

Apache 2.0 — Free and open source.
