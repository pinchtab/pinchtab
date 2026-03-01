<p align="center">
  <img src="assets/pinchtab-headless.png" alt="PinchTab" width="200"/>
</p>

<p align="center">
  <strong>Browser control for AI agents.</strong><br/>
  12MB Go binary • HTTP API • Token-efficient
</p>

<p align="center">
  <a href="https://pinchtab.com/docs"><strong>📖 Full Documentation</strong></a>
  •
  <a href="https://github.com/pinchtab/pinchtab/releases/latest">Latest Release</a>
</p>

<p align="center">
  <a href="https://github.com/pinchtab/pinchtab/releases/latest"><img src="https://img.shields.io/github/v/release/pinchtab/pinchtab?style=flat-square&color=FFD700" alt="Release"/></a>
  <a href="https://github.com/pinchtab/pinchtab/actions/workflows/go-verify.yml"><img src="https://img.shields.io/github/actions/workflow/status/pinchtab/pinchtab/go-verify.yml?branch=main&style=flat-square&label=Build" alt="Build"/></a>
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.25+"/>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square" alt="License"/></a>
</p>

---

## What is PinchTab?

PinchTab is a **standalone HTTP server** that gives AI agents direct control over a Chrome browser.

### Key Features

- **CLI or Curl** — Control via command-line or HTTP API
- **Token-efficient** — 800 tokens/page with text extraction (5-13x cheaper than screenshots)
- **Headless or Headed** — Run without a window or with visible Chrome
- **Multi-instance** — Run multiple parallel Chrome processes with isolated profiles
- **Self-contained** — 12MB binary, no external dependencies
- **Accessibility-first** — Stable element refs instead of fragile coordinates

---

## Quick Start

### Installation

**macOS / Linux:**
```bash
curl -fsSL https://pinchtab.com/install.sh | bash
```

**npm:**
```bash
npm install -g pinchtab
```

**Docker:**
```bash
docker run -d -p 9867:9867 pinchtab/pinchtab
```

### Use It

**Terminal 1 — Start the server:**
```bash
pinchtab
```

**Terminal 2 — Control the browser:**
```bash
# Navigate
pinchtab nav https://example.com

# Get page structure
pinchtab snap -i -c

# Click an element
pinchtab click e5

# Extract text
pinchtab text
```

Or use the HTTP API directly:
```bash
# Navigate (returns tabId)
TAB=$(curl -s -X POST http://localhost:9867/instances \
  -d '{"profile":"work"}' | jq -r '.id')

# Get snapshot
curl "http://localhost:9867/instances/$TAB/snapshot?filter=interactive"

# Click element
curl -X POST "http://localhost:9867/instances/$TAB/action" \
  -d '{"kind":"click","ref":"e5"}'
```

---

## Core Concepts

**Instance** — A running Chrome process. Each instance can have one profile.

**Profile** — Browser state (cookies, history, local storage). Log in once, stay logged in across restarts.

**Tab** — A single webpage. Each instance can have multiple tabs.

Read more in the [Core Concepts](https://pinchtab.com/docs/core-concepts) guide.

---

## Why PinchTab?

| Aspect | PinchTab |
|--------|----------|
| **Tokens performance** | ✅ |
| **Headless and Headed** | ✅ |
| **Profile** | ✅ |
| **Stealth mode** | ✅ |
| **Persistent sessions** | ✅ |
| **Binary size** | ✅ |
| **Multi-instance** | ✅ |
| **Remote Chrome** | ✅ |

---

## Documentation

Full docs at **[pinchtab.com/docs](https://pinchtab.com/docs)**

- **[Getting Started](https://pinchtab.com/docs/getting-started)** — Install and run
- **[Core Concepts](https://pinchtab.com/docs/core-concepts)** — Instances, profiles, tabs
- **[Headless vs Headed](https://pinchtab.com/docs/headless-vs-headed)** — Choose the right mode
- **[API Reference](https://pinchtab.com/docs/api-reference)** — HTTP endpoints
- **[CLI Reference](https://pinchtab.com/docs/cli-reference)** — Command-line commands
- **[Configuration](https://pinchtab.com/docs/configuration)** — Environment variables

---

## Examples

### AI Agent Automation

```bash
# Your AI agent can:
pinchtab nav https://example.com
pinchtab snap -i  # Get clickable elements
pinchtab click e5 # Click by ref
pinchtab fill e3 "user@example.com"  # Fill input
pinchtab press e7 Enter              # Submit form
```

### Data Extraction

```bash
# Extract text (token-efficient)
pinchtab nav https://example.com/article
pinchtab text  # ~800 tokens instead of 10,000
```

### Multi-Instance Workflows

```bash
# Run multiple instances in parallel
pinchtab instances create --profile=alice --port=9868
pinchtab instances create --profile=bob --port=9869

# Each instance is isolated
curl http://localhost:9868/text?tabId=X  # Alice's instance
curl http://localhost:9869/text?tabId=Y  # Bob's instance
```

---

## License

MIT — Free and open source.

---

**Get started:** [pinchtab.com/docs](https://pinchtab.com/docs)
