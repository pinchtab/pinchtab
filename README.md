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

For day-to-day local use, the server is typically installed as a user-level daemon, allowing agent tools to reuse the same browser control plane running in the background.

```bash
curl -fsSL https://pinchtab.com/install.sh | bash
# or
pinchtab daemon install
```

This installs the control-plane server and starts a default headless Chrome instance, ready to accept requests from agents or manual API calls.


If you prefer not to run a daemon, or if you're on Windows, you can instead run:

`pinchtab server` — runs the control-plane server directly
`pinchtab bridge` — runs a single browser instance as a lightweight runtime

PinchTab also provides a CLI with an interactive entry point for local setup and common tasks:

`pinchtab`

## Security

PinchTab defaults to a **local-first security posture**:

- `server.bind = 127.0.0.1`
- sensitive endpoint families are disabled by default
- `attach` is disabled by default
- IDPI is enabled with a **local-only website allowlist**

> [!CAUTION]
> By default, IDPI restricts browsing to **locally hosted websites only**.
> This prevents agents from navigating the public internet until you explicitly allow it.
> The restriction exists to make the security implications of browser automation clear before enabling wider access.

See the full guide: [docs/guides/security.md](docs/guides/security.md)

## What can you use it for

### Headless navigation

With the daemon installed and an agent skill configured, an agent can execute tasks like:

```
"What are the main news about aliens on news.com?"
```

PinchTab exposes browser tools that allow agents to navigate pages, extract structured content, and interact with the DOM without wasting tokens on raw HTML or images.

### Headed navigation

In addition to headless automation, PinchTab supports headed Chrome profiles.

You can create profiles configured with authentication, cookies, extensions, or specific environments. Each profile can have a name and description.

For example, an agent request like:

```
"Log into my work profile and download the weekly report"
```

can automatically select the appropriate profile and perform the action.

### Local container isolation

If you prefer stronger isolation, PinchTab can run inside Docker.

This allows agents to control browsers in a sandboxed environment, reducing risk when running automation tasks locally.

### Distributed automation

PinchTab can manage multiple Chrome instances (headless or headed) across containers or remote machines.

Typical use cases include:

- QA automation
- testing environments
- distributed browsing tasks
- development tooling

You can connect to multiple PinchTab servers, or attach to Chrome instances running in remote debug mode.

## Process Model

PinchTab is server-first:
1. install the daemon or run `pinchtab server` for the full control plane
2. let the server manage profiles and instances
3. let each managed instance run behind a lightweight `pinchtab bridge` runtime

In practice:
- Server — the main product entry point and control plane
- Bridge — the runtime that manages a single browser instance
- Attach — an advanced mode for registering externally managed Chrome instances

### Primary Usage

The primary user journey is:

1. install Pinchtab
2. install and start the daemon with `pinchtab daemon install`
3. point your agent or tool at `http://localhost:9867`
4. let PinchTab act as your local browser service

That is the default “replace the browser runtime” scenario.
Most users should not need to think about `pinchtab bridge` directly, and only need `pinchtab` when they want the local interactive menu.

### Key Features

- **CLI or Curl** — Control via command-line or HTTP API
- **Token-efficient** — 800 tokens/page with text extraction (5-13x cheaper than screenshots)
- **Headless or Headed** — Run without a window or with visible Chrome
- **Multi-instance** — Run multiple parallel Chrome processes with isolated profiles
- **Self-contained** — ~15MB binary, no external dependencies
- **Accessibility-first** — Stable element refs instead of fragile coordinates
- **ARM64-optimized** — First-class Raspberry Pi support with automatic Chromium detection

---

## Quick Start

### Installation

**macOS / Linux:**
```bash
curl -fsSL https://pinchtab.com/install.sh | bash
```

**Homebrew (macOS / Linux):**
```bash
brew install pinchtab/tap/pinchtab
```

**npm:**
```bash
npm install -g pinchtab
```

### Shell Completion

Generate and install shell completions after `pinchtab` is on your `PATH`:

```bash
# Generate and install zsh completions
pinchtab completion zsh > "${fpath[1]}/_pinchtab"

# Generate bash completions
pinchtab completion bash > /etc/bash_completion.d/pinchtab

# Generate fish completions
pinchtab completion fish > ~/.config/fish/completions/pinchtab.fish
```

**Docker:**
```bash
docker run -d \
  --name pinchtab \
  -p 127.0.0.1:9867:9867 \
  -v pinchtab-data:/data \
  --shm-size=2g \
  pinchtab/pinchtab
```

The bundled container persists its managed config at `/data/.config/pinchtab/config.json`.
If you want to supply your own config file instead, mount it and point `PINCHTAB_CONFIG` at it:

```bash
docker run -d \
  --name pinchtab \
  -p 127.0.0.1:9867:9867 \
  -e PINCHTAB_CONFIG=/config/config.json \
  -v "$PWD/config.json:/config/config.json:ro" \
  -v pinchtab-data:/data \
  --shm-size=2g \
  pinchtab/pinchtab
```

### Use It

**Terminal 1 — Start the server:**
```bash
pinchtab server
```

**Recommended for daily local use — install the daemon once:**
```bash
pinchtab daemon install
pinchtab daemon
```

That keeps PinchTab running in the background so your agent tools can reuse it without an open terminal.

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
| **Advanced CDP control** | ✅ |
| **Persistent sessions** | ✅ |
| **Binary size** | ✅ |
| **Multi-instance** | ✅ |
| **External Chrome attach** | ✅ |

---

## Privacy

PinchTab is a fully open-source, local-only tool. No telemetry, no analytics, no outbound connections. The binary binds to `127.0.0.1` by default. Persistent profiles store browser sessions locally on your machine — similar to how a human reuses their browser. The single Go binary (~16 MB) is fully verifiable: build from source at [github.com/pinchtab/pinchtab](https://github.com/pinchtab/pinchtab).

---

## Documentation

Full docs at **[pinchtab.com/docs](https://pinchtab.com/docs)**

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

See [chrome-files.md](chrome-files.md) for technical details on how PinchTab manages Chrome user data directories and ensures isolation between parallel instances.

---

## Development

Want to contribute? Start with [CONTRIBUTING.md](CONTRIBUTING.md).
The full setup and workflow guide lives at [docs/guides/contributing.md](docs/guides/contributing.md).

**Quick start:**
```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
./dev doctor                # Verifies environment, offers hooks/deps setup
./dev --help                # Shows the developer toolkit commands
go build ./cmd/pinchtab     # Build pinchtab binary
```

---

## License

MIT — Free and open source.

---

**Get started:** [pinchtab.com/docs](https://pinchtab.com/docs)
