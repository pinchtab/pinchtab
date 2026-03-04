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

### Use It

**Terminal 1 — Start the server:**
```bash
pinchtab
```

**Terminal 2 — Control via HTTP API:**
```bash
# Navigate to a page
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

# Get accessibility tree snapshot
curl "http://localhost:9867/snapshot?filter=interactive&compact=true"

# Click an element
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'

# Extract page text
curl http://localhost:9867/text
```

Or use a client library (Playwright, Puppeteer, Cypress):
```python
import requests

BASE = "http://localhost:9867"

# Navigate
requests.post(f"{BASE}/navigate", json={"url": "https://example.com"})

# Get snapshot
resp = requests.get(f"{BASE}/snapshot?filter=interactive&compact=true")
print(resp.json())

# Click element
requests.post(f"{BASE}/action", json={"kind": "click", "ref": "e5"})
```

**Configuration:**
```bash
# Initialize config file
pinchtab config init

# Set config values
pinchtab config set server.port 9999
pinchtab config set chrome.headless false
pinchtab config set orchestrator.strategy session

# View config
pinchtab config show --format yaml

# Validate config
pinchtab config validate

# Patch with JSON
pinchtab config patch '{"chrome": {"maxTabs": 50}}'
```

**Management commands:**
```bash
pinchtab health              # Server status
pinchtab profiles            # List available profiles
pinchtab instances           # List running instances
pinchtab tabs                # List open tabs
pinchtab connect myprofile   # Get URL for a profile instance
```

---

## Core Concepts

**Instance** — A running Chrome process. Each instance can have one profile.

**Profile** — Browser state (cookies, history, local storage). Log in once, stay logged in across restarts.

**Tab** — A single webpage. Each instance can have multiple tabs.

Read more in the [Core Concepts](https://pinchtab.com/docs/core-concepts) guide.

---

## Configuration

PinchTab uses a JSON config file at `~/.config/pinchtab/config.json` (macOS/Linux) or `%APPDATA%\pinchtab\config.json` (Windows).

### Config Management

```bash
# Create default config
pinchtab config init

# Set individual values
pinchtab config set server.port 9999
pinchtab config set chrome.headless false
pinchtab config set chrome.maxTabs 50
pinchtab config set orchestrator.strategy session
pinchtab config set orchestrator.allocationPolicy round_robin

# Merge a JSON object
pinchtab config patch '{"chrome": {"headless": false, "maxTabs": 100}}'

# View config
pinchtab config show                    # JSON format
pinchtab config show --format yaml      # YAML format

# Validate config
pinchtab config validate
```

### Config Sections

**server** — Server settings
```json
{
  "port": "9867",
  "stateDir": "~/.config/pinchtab",
  "profileDir": "~/.config/pinchtab/chrome-profile",
  "token": "secret-key",
  "cdpUrl": "ws://localhost:9222"
}
```

**chrome** — Browser settings
```json
{
  "headless": true,
  "maxTabs": 20,
  "noRestore": false
}
```

**orchestrator** — Instance management (dashboard mode)
```json
{
  "strategy": "simple",
  "allocationPolicy": "fcfs",
  "instancePortStart": 9868,
  "instancePortEnd": 9968
}
```

**timeouts** — Timing settings (seconds)
```json
{
  "timeoutSec": 15,
  "navigateSec": 30
}
```

### Environment Variable Override

Environment variables take precedence over config file:
- `BRIDGE_PORT` — Server port
- `BRIDGE_HEADLESS` — Headless mode
- `PINCHTAB_STRATEGY` — Allocation strategy
- `PINCHTAB_ALLOCATION_POLICY` — Instance selection policy

See [Configuration](https://pinchtab.com/docs/configuration) for full list.

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
