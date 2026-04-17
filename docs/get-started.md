# Getting Started

Get PinchTab running in a few minutes, from zero to browser automation.

This guide covers the default local setup. If you plan to publish ports beyond localhost, bind to non-loopback interfaces, or run a remote or distributed topology, treat that as an advanced deployment and read the [Security guide](guides/security.md) first.

---

## Installation

### Option 1: one-liner

**macOS / Linux**

```bash
curl -fsSL https://pinchtab.com/install.sh | bash
```

Then verify:

```bash
pinchtab --version
```

### Option 2: npm

**Requires:** Node.js 18+

```bash
npm install -g pinchtab
pinchtab --version
```

### Option 3: Docker

**Requires:** Docker

```bash
docker run -d -p 127.0.0.1:9867:9867 pinchtab/pinchtab
curl http://localhost:9867/health
```

### Option 4: build from source

**Requires:** Go 1.25+, Git, Chrome/Chromium

```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
./dev doctor
go build -o pinchtab ./cmd/pinchtab
./pinchtab --version
```

**[Full build guide ->](architecture/building.md)**

## Platform Support

PinchTab's primary tested workflow is local macOS and Linux.

Windows binaries are available, but Windows support is currently limited and best-effort because the project does not have the same level of test coverage there. On Windows, prefer direct runs with `pinchtab server` instead of expecting the full daemon workflow.

## Shell Completion

After installation, you can generate shell completions from the CLI:

```bash
# Generate and install zsh completions
pinchtab completion zsh > "${fpath[1]}/_pinchtab"

# Generate bash completions
pinchtab completion bash > /etc/bash_completion.d/pinchtab

# Generate fish completions
pinchtab completion fish > ~/.config/fish/completions/pinchtab.fish
```

---

## Quick start

The normal flow is:

1. start the server
2. start an instance
3. navigate
4. inspect or act

### Step 1: start the server

```bash
pinchtab server
# Response
🦀 PinchTab port=9867
dashboard ready url=http://localhost:9867
```

The server runs on `http://127.0.0.1:9867`.
You can open the dashboard at `http://127.0.0.1:9867` or `http://127.0.0.1:9867/dashboard`.

### Step 2: start your first instance

```bash
curl -s -X POST http://127.0.0.1:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq .
# CLI Alternative
pinchtab instance start
# Response
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "instance-1741400000000000000",
  "port": "9868",
  "mode": "headless",
  "headless": true,
  "status": "starting"
}
```

### Step 3: navigate

```bash
curl -s -X POST http://127.0.0.1:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com/pinchtab/pinchtab"}' | jq .
# CLI Alternative
pinchtab nav https://github.com/pinchtab/pinchtab
# Response
{
  "tabId": "CDP_TARGET_ID",
  "title": "GitHub - pinchtab/pinchtab",
  "url": "https://github.com/pinchtab/pinchtab"
}
```

### Step 4: inspect the page

```bash
curl -s "http://127.0.0.1:9867/snapshot?filter=interactive" | jq .
# CLI Alternative
pinchtab snap -i -c
# Response
{
  "nodes": [
    { "ref": "e0", "role": "link", "name": "Skip to content" },
    { "ref": "e14", "role": "button", "name": "Search or jump to…" }
  ]
}
```

You now have a working PinchTab server, a running browser instance, and a navigated tab.

---

## Troubleshooting

### Connection refused

```bash
curl http://localhost:9867/health
```

If that fails, start the server:

```bash
pinchtab server
```

### Port already in use

```bash
pinchtab config set server.port 9868
pinchtab server
```

### Chrome not found

```bash
# macOS
brew install chromium

# Linux (Ubuntu/Debian)
sudo apt install chromium-browser

# Custom Chrome binary (set in config)
pinchtab config set browser.binary /path/to/chrome
```

---

## Getting help

- [GitHub Issues](https://github.com/pinchtab/pinchtab/issues)
- [GitHub Discussions](https://github.com/pinchtab/pinchtab/discussions)
