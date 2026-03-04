<p align="center">
  <img src="assets/pinchtab-headless.png" alt="PinchTab" width="200"/>
</p>

<p align="center">
  <strong>PinchTab</strong><br/>
  <strong>Browser control for AI agents</strong><br/>
  12MB Go binary • HTTP API • Token-efficient
</p>


<table align="center">
  <tr>
    <td align="center" valign="middle">
      <a href="https://pinchtab.com/docs"><img src="assets/docs-no-background-256.png" alt="Full Documentation" width="92"/></a>
    </td>
    <td align="left" valign="middle">
      <a href="https://github.com/pinchtab/pinchtab/releases/latest"><img src="https://img.shields.io/github/v/release/pinchtab/pinchtab?style=flat-square&color=FFD700" alt="Release"/></a><br/>
      <a href="https://github.com/pinchtab/pinchtab/actions/workflows/go-verify.yml"><img src="https://img.shields.io/github/actions/workflow/status/pinchtab/pinchtab/go-verify.yml?branch=main&style=flat-square&label=Build" alt="Build"/></a><br/>
      <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.25+"/><br/>
      <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square" alt="License"/></a>
    </td>
  </tr>
</table>

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
- **ARM64-optimized** — First-class Raspberry Pi support with automatic Chromium detection

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

### Quick Start

**Terminal 1 — Start the server:**
```bash
pinchtab
# Server running on http://localhost:9867
```

**Terminal 2 — Control via HTTP API:**
```bash
# Navigate to a page
curl -X POST http://localhost:9867/navigate \
  -d '{"url":"https://example.com"}' -H "Content-Type: application/json"

# Get page structure
curl "http://localhost:9867/snapshot?filter=interactive&compact=true"

# Interact with the page
curl -X POST http://localhost:9867/action \
  -d '{"kind":"click","ref":"e5"}' -H "Content-Type: application/json"
```

**Management:**
```bash
pinchtab health        # Server status
pinchtab profiles      # List profiles
pinchtab instances     # List running instances
pinchtab config show   # View configuration
```

See **[Full Documentation](https://pinchtab.com/docs)** for detailed guides and API reference.

---

## Core Concepts

**Instance** — A running Chrome process. Each instance can have one profile.

**Profile** — Browser state (cookies, history, local storage). Log in once, stay logged in across restarts.

**Tab** — A single webpage. Each instance can have multiple tabs.

Read more in the [Core Concepts](https://pinchtab.com/docs/core-concepts) guide.

---


## Configuration

### Tab Limit Management

Control what happens when Chrome hits the tab limit (default: 20 tabs):

**Policy Options:**

- **`reject`** (default) — Return 429 error when limit reached
  - Safest option, requires manual cleanup
  - Prevents unexpected tab closures
  - Best for production environments

- **`close_oldest`** — Auto-close oldest tab when limit reached
  - Closes tab with earliest creation timestamp
  - Useful for long-running sessions
  - Automatic cleanup, no manual intervention needed

- **`close_lru`** — Auto-close least-recently-used tab (future)
  - Closes tab with earliest access timestamp
  - Best for interactive use (keeps active tabs open)

**Configuration:**

```bash
# Via environment variable
TAB_EVICTION_POLICY=reject pinchtab         # Default
TAB_EVICTION_POLICY=close_oldest pinchtab   # Auto-close oldest

# Via config file
pinchtab config set tabEvictionPolicy close_oldest
```

**HTTP Response:**

When `reject` policy is active and limit is reached:

```json
{
  "code": "error",
  "error": "tab limit reached (20/20)"
}
```

HTTP Status: `429 Too Many Requests`

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

- **[Getting Started](https://pinchtab.com/docs/get-started)** — Install and run
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

## Development

Want to contribute? See [DEVELOPMENT.md](DEVELOPMENT.md) for setup instructions.

**Quick start:**
```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
./doctor.sh                 # Verifies environment, installs hooks/deps
go build ./cmd/pinchtab     # Build pinchtab binary
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.

---

## License

MIT — Free and open source.

---

**Get started:** [pinchtab.com/docs](https://pinchtab.com/docs)
