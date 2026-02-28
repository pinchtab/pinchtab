# Pinchtab Documentation

Welcome to Pinchtab — browser control for AI agents, scripts, and automation workflows.

## What is Pinchtab?

Pinchtab is a **standalone HTTP server** that gives you direct control over a Chrome browser. Instead of being locked into a specific agent framework, you can interact with it from anywhere — any AI agent, any programming language, or even `curl`.

```bash
# It's just HTTP. Use it anywhere.
curl http://localhost:9867/health
curl http://localhost:9867/text?tabId=abc123
curl -X POST http://localhost:9867/action -d '{"kind":"click","ref":"e5"}'
```

---

## Why Pinchtab?

### The Problem

Most browser automation tools are **framework-locked**:
- OpenClaw Browser → only works in OpenClaw
- Playwright MCP → only works with your MCP client
- Browser Use → only works in its own system

You can't use the same browser automation across different agents.

### The Solution

Pinchtab is **framework-agnostic HTTP server**:
- ✅ Works with any AI agent (Claude, ChatGPT, local models)
- ✅ Works with any language (bash, Python, Node.js, Go)
- ✅ Works with any tool (curl, Postman, your own scripts)
- ✅ Persistent sessions (log in once, stays logged in)
- ✅ Stealth mode (bypass bot detection)
- ✅ Token efficient (5-13x cheaper than alternatives)

---

## Key Features

### 🌲 Accessibility Tree
Structured tree with stable refs (e0, e1...) for click, type, and read. Deterministic — no coordinate guessing.

### 🎯 Smart Filters
`?filter=interactive` returns only buttons, links, and inputs. Fewer tokens per snapshot.

### 🕵️ Stealth Mode
Patches `navigator.webdriver`, spoofs UA, and hides automation flags to pass major bot checks.

### 💾 Persistent Sessions
Cookies, auth, and tabs survive restarts. Log in once and keep the session alive.

### 📝 Text Extraction
Readability mode strips nav and ads. Raw mode keeps full text for parser workflows.

### 🖱️ Direct Actions
Click, type, fill, press, focus, hover, select, and scroll by ref or selector.

### ⚡ JS Evaluation
Escape hatch for any workflow gap. Execute JavaScript in any tab on demand.

### 📸 Screenshots
JPEG output with quality control for visual verification and downstream auditing.

### 📄 PDF Export
Export full pages as PDF for sharing, archiving, and offline review.

### 🎭 Multi-Tab Support
Create, switch, and close tabs. Work with multiple pages simultaneously.

---

## Quick Comparison

| Feature | Pinchtab | OpenClaw Browser | Playwright | Selenium |
|---------|----------|------------------|-----------|----------|
| **Interface** | HTTP (any agent) | Framework-locked | Framework-locked | WebDriver |
| **Tokens/page** | ~800 (text) | ~10,000+ | N/A | N/A |
| **Stealth mode** | ✅ | ❌ | ❌ | ❌ |
| **Persistent sessions** | ✅ | ❌ | ❌ | ❌ |
| **Self-contained binary** | ✅ 12MB | ❌ | ❌ | ❌ |
| **Accessibility tree** | ✅ | ✅ | ❌ | ❌ |
| **PDF export** | ✅ | ❌ | ✅ | ❌ |
| **Tab-centric design** | ✅ | ❌ | ❌ | ❌ |

---

## Real Performance Numbers

### Token Efficiency

```
Reading a 1,500-word article:
  Pinchtab /text:        800-900 tokens    (5-13x cheaper)
  OpenClaw snapshot:     ~3,600 tokens
  Full screenshot:       ~10,000 tokens
  Vision + screenshot:   ~20,000 tokens
```

### Response Times

```
Navigate + snapshot:     1-3 seconds
Click + verify:          200-500ms
Text extraction:         100-300ms
PDF generation:          2-5 seconds
```

### Binary Size

```
Pinchtab:               12MB (includes Chrome)
Node.js equivalent:     100MB+
Python equivalent:      200MB+
```

---

## Architecture

### High-Level Design

```
┌─────────────────────────────────────────┐
│         Your Tool/Agent                  │
│   (curl, Python, Node.js, etc.)         │
└──────────────┬──────────────────────────┘
               │
               │ HTTP
               ↓
┌─────────────────────────────────────────┐
│    Pinchtab HTTP Server (Go)            │
│  ┌─────────────────────────────────┐    │
│  │  Tab Manager (multi-tab)         │    │
│  │  ┌────────────┐ ┌────────────┐  │    │
│  │  │  Tab 1     │ │  Tab 2     │  │    │
│  │  │  DOM       │ │  DOM       │  │    │
│  │  │  History   │ │  History   │  │    │
│  │  └────────────┘ └────────────┘  │    │
│  └─────────────────────────────────┘    │
│  ┌─────────────────────────────────┐    │
│  │  Chrome DevTools Protocol       │    │
│  │  (WebSocket to Chrome)          │    │
│  └─────────────────────────────────┘    │
└──────────────┬──────────────────────────┘
               │
               │ CDP
               ↓
┌─────────────────────────────────────────┐
│        Chrome Browser (Headless)        │
│  ┌────────────┐  ┌────────────┐        │
│  │  Tab 1     │  │  Tab 2     │        │
│  │  (website) │  │ (website)  │        │
│  └────────────┘  └────────────┘        │
└─────────────────────────────────────────┘
```

### What Makes It Different

1. **Tab-Centric** — Everything revolves around tabs, not URLs
2. **Stateful** — Sessions persist between requests
3. **Accessibility-First** — Uses accessibility tree, not coordinates
4. **HTTP API** — Standard web interface, no proprietary protocols
5. **Multi-Agent Safe** — Tab locking for agent coordination

---

## Getting Started

### Installation (30 seconds)

```bash
# macOS / Linux
curl -fsSL https://pinchtab.com/install.sh | bash

# Or with npm
npm install -g pinchtab

# Or build from source
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
go build -o pinchtab ./cmd/pinchtab
```

### First Steps

```bash
# Terminal 1: Start the server
pinchtab

# Terminal 2: Try a command
curl http://localhost:9867/health          # Health check
pinchtab quick https://example.com         # Quick start
pinchtab snap -i -c                        # Get interactive elements
```

### Next

- **Quick Start Guide** → [get-started.md](get-started.md)
- **CLI Commands** → [references/cli-commands.md](references/cli-commands.md)
- **API Reference** → [references/curl-commands.md](references/curl-commands.md)
- **Practical Examples** → [showcase.md](showcase.md)
- **Architecture Deep-Dive** → [architecture/pinchtab-architecture.md](architecture/pinchtab-architecture.md)

---

## Use Cases

### 1. AI Agent Automation
Any AI agent (Claude, ChatGPT, Anthropic models) can control browsers:
```bash
# Agent uses Pinchtab to fill forms, navigate, extract data
pinchtab nav https://example.com
pinchtab snap -i -c  # Get interactive elements
pinchtab click e5    # Agent decides based on snapshot
```

### 2. Data Scraping
Extract text from complex websites:
```bash
pinchtab nav https://example.com/article
pinchtab text  # 800 tokens instead of 10,000 with vision
```

### 3. Testing & QA
End-to-end testing with persistence:
```bash
# Login once
pinchtab nav https://myapp.com/login
pinchtab fill e3 username
pinchtab fill e5 password
pinchtab click e7  # Submit

# Subsequent tests reuse the session
pinchtab nav https://myapp.com/dashboard
pinchtab snap  # Already logged in
```

### 4. Report Generation
Create PDFs from web pages:
```bash
pinchtab nav https://analytics.example.com/report
pinchtab pdf -o report.pdf --landscape --displayHeaderFooter
```

### 5. Bot Detection Bypass
Stealth mode fools modern bot detection:
```bash
BRIDGE_STEALTH=full pinchtab
pinchtab nav https://protected-site.com  # Bypasses detection
```

---

## Documentation Structure

```
docs/
├── overview.md (you are here)
├── get-started.md              ← Start here for quick setup
├── showcase.md                 ← Practical examples
├── architecture/               ← How it works inside
│   ├── pinchtab-architecture.md
│   ├── chrome-lifecycle-and-orchestration.md
│   └── building.md
├── guides/                     ← How-to guides
│   ├── docker.md
│   ├── headed-mode-guide.md
│   ├── cdp-url-shared-chrome.md
│   └── identifying-instances.md
├── references/                 ← API documentation
│   ├── cli-commands.md
│   └── curl-commands.md
└── extras/                     ← Background reading
    ├── pinchtab-clean-slate.md
    ├── agent-optimization.md
    └── browser-extraction-spectrum.md
```

---

## Support & Community

- **GitHub Issues** — https://github.com/pinchtab/pinchtab/issues
- **Discussions** — https://github.com/pinchtab/pinchtab/discussions
- **Twitter/X** — [@pinchtabdev](https://x.com/pinchtabdev)

---

## Core Concepts (Brief)

### Tab-Centric Design
Every operation works on a tab, not a URL. Create a tab first, then use its `tabId`:

```bash
# Create tab + navigate → returns tabId
curl -X POST http://localhost:9867/tab \
  -d '{"action":"new","url":"https://example.com"}' | jq '.tabId'
# "abc123"

# Use tabId for all operations
curl "http://localhost:9867/snapshot?tabId=abc123"
curl "http://localhost:9867/text?tabId=abc123"
curl -X POST http://localhost:9867/action \
  -d '{"kind":"click","ref":"e5","tabId":"abc123"}'
```

### Refs Instead of Coordinates
The accessibility tree gives you stable refs (e0, e1, e2...) instead of pixel coordinates:

```json
{
  "elements": [
    {"ref": "e0", "role": "heading", "name": "Title"},
    {"ref": "e5", "role": "button", "name": "Click Me"},
    {"ref": "e8", "role": "link", "name": "Learn More"}
  ]
}
```

Then interact by ref:
```bash
curl -X POST http://localhost:9867/action \
  -d '{"kind":"click","ref":"e5"}'
```

### Persistent Sessions
Tabs and cookies survive restarts:

```bash
# Login in one session
pinchtab nav https://example.com/login
pinchtab fill e3 user@example.com
pinchtab fill e5 password
pinchtab click e7

# Restart Pinchtab, tab is still there + still logged in
pkill pinchtab
./pinchtab
# Tab is restored, cookies intact
```

---

## Next Steps

1. **Install:** Follow [get-started.md](get-started.md)
2. **Try it:** Run the quick start examples
3. **Learn:** Read [showcase.md](showcase.md) for workflows
4. **Build:** Check [architecture/](architecture/) for how it works
5. **Deploy:** See [guides/](guides/) for production setups

---

## License

Apache 2.0 — Free and open source.

---

**Ready?** → [Get Started](get-started.md)
