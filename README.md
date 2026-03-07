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

PinchTab is a **standalone HTTP server** that gives AI agents direct control over Chrome.

It has two runtime roles:
- `pinchtab` or `pinchtab server` — the full control-plane server
- `pinchtab bridge` — a single-instance bridge runtime

Most users only need the full server. It manages profiles, instances, routing, and the web dashboard. The `bridge` mode is the thin per-instance runtime used behind the scenes for managed child instances.

### Process Model

PinchTab is server-first:
- start `pinchtab` for the full control plane
- let the server manage profiles and instances
- let each managed instance run behind a lightweight `pinchtab bridge` runtime

In practice:
- **Server** is the public product entrypoint
- **Bridge** is the per-instance runtime for one browser
- **Attach** is the advanced path for registering an externally managed Chrome

### Primary Usage

The primary user journey is:

1. install Pinchtab
2. run `pinchtab`
3. point your agent or tool at `http://localhost:9867`
4. let Pinchtab act as your local browser service

That is the default “replace the browser runtime” scenario.
Most users should not need to think about `pinchtab bridge` directly.

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

**Terminal 2 — Control the browser:**
```bash
# Navigate
pinchtab nav https://pinchtab.com

# Get page structure
pinchtab snap -i -c

# Click an element
pinchtab click e5

# Extract text
pinchtab text
```

Or use the HTTP API directly:
```bash
# Create an instance (returns instance id)
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"work","mode":"headless"}' | jq -r '.id')

# Open a tab in that instance
TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}' | jq -r '.tabId')

# Get snapshot
curl "http://localhost:9867/tabs/$TAB/snapshot?filter=interactive"

# Click element
curl -X POST "http://localhost:9867/tabs/$TAB/action" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'
```

---

## Core Concepts

**Server** — The main PinchTab process. It manages profiles, instances, routing, and the dashboard.

**Instance** — A running Chrome process. Each instance can have one profile.

**Profile** — Browser state (cookies, history, local storage). Log in once, stay logged in across restarts.

**Tab** — A single webpage. Each instance can have multiple tabs.

**Bridge** — The single-instance runtime behind a managed instance. Usually spawned by the server, not started manually.

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
| **External Chrome attach** | ✅ |

---

## Security — Prompt Injection Defense (IDPI)

When an AI agent fetches arbitrary web pages, attackers can embed hidden instructions in the page content that attempt to override the agent's system prompt. PinchTab includes an optional, layered defense against this threat called **IDPI** (Indirect Prompt Injection defense).

Enable it in your `config.json`:

```json
{
  "security": {
    "idpi": {
      "enabled": true,
      "allowedDomains": ["github.com", "*.github.com"],
      "strictMode": false,
      "scanContent": true,
      "wrapContent": false,
      "customPatterns": []
    }
  }
}
```

| Option | Default | Description |
|--------|---------|-------------|
| `enabled` | `false` | Master switch. No IDPI checks are performed when false |
| `allowedDomains` | `[]` | Navigation whitelist. Patterns: `"example.com"`, `"*.example.com"`, `"*"` |
| `strictMode` | `false` | `true` = HTTP 403 block; `false` = warn via `X-IDPI-Warning` header |
| `scanContent` | `false` | Scan `/snapshot` and `/text` responses for injection phrases |
| `wrapContent` | `false` | Wrap `/text` output in `<untrusted_web_content>` delimiters for downstream LLMs |
| `customPatterns` | `[]` | Additional injection phrases to detect (case-insensitive) |

All fields default to `false`/empty — existing behaviour is completely unchanged unless you opt in.

---

## Documentation

Full docs at **[pinchtab.com/docs](https://pinchtab.com/docs)**

- **[Getting Started](https://pinchtab.com/docs/get-started)** — Install and run
- **[Core Concepts](https://pinchtab.com/docs/core-concepts)** — Instances, profiles, tabs
- **[Headless vs Headed](https://pinchtab.com/docs/headless-vs-headed)** — Choose the right mode
- **[API Reference](https://pinchtab.com/docs/api-reference)** — HTTP endpoints
- **[CLI Reference](https://pinchtab.com/docs/cli-reference)** — Command-line commands
- **[Configuration](https://pinchtab.com/docs/configuration)** — Config file and runtime settings

### MCP (SMCP) integration

An **SMCP plugin** in this repo lets AI agents control PinchTab via the [Model Context Protocol](https://github.com/sanctumos/smcp) (SMCP). One plugin exposes 15 tools (e.g. `pinchtab__navigate`, `pinchtab__snapshot`, `pinchtab__action`). No extra runtime deps (stdlib only). See **[plugins/README.md](plugins/README.md)** for setup (env vars and paths).

---

## Examples

### AI Agent Automation

```bash
# Your AI agent can:
pinchtab nav https://pinchtab.com
pinchtab snap -i  # Get clickable elements
pinchtab click e5 # Click by ref
pinchtab fill e3 "user@pinchtab.com"  # Fill input
pinchtab press e7 Enter              # Submit form
```

### Data Extraction

```bash
# Extract text (token-efficient)
pinchtab nav https://pinchtab.com/article
pinchtab text  # ~800 tokens instead of 10,000
```

### Multi-Instance Workflows

```bash
# Run multiple instances in parallel
curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"alice","mode":"headless"}'

curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"bob","mode":"headless"}'

# Each instance is isolated
curl http://localhost:9867/instances
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
