# PinchTab

Welcome to PinchTab — browser control for AI agents, scripts, and automation workflows.

## What is PinchTab?

PinchTab is a **standalone HTTP server** that gives you direct control over Chrome. Any AI agent can use the CLI or HTTP API.

The main concept to understand first is that PinchTab has two runtime roles:

- `pinchtab` or `pinchtab server` — the full control-plane server
- `pinchtab bridge` — the single-instance bridge runtime

The server is the normal entrypoint. It manages profiles, instances, routing, and the dashboard. The bridge runtime is the lightweight single-instance HTTP wrapper used for managed child instances.

That gives you a simple mental model:
- start the **server**
- create or attach **instances**
- operate on **tabs**

## Primary Usage

For most users, Pinchtab is a local browser service:

1. install Pinchtab
2. run `pinchtab`
3. connect your agent or tool to `http://localhost:9867`
4. use Pinchtab instead of embedding a browser runtime directly in the client

In that model:
- `pinchtab server` is the primary target
- `pinchtab bridge` is an internal or advanced runtime detail

**CLI example:**
```bash
# Navigate
pinchtab nav https://pinchtab.com

# Get interactive elements
pinchtab snap -i -c

# Click element by ref
pinchtab click e5
```

**HTTP example (realistic flow):**
```bash
# 1. Create an instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"work","mode":"headless"}' | jq -r '.id')

# 2. Open a tab
TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}' | jq -r '.tabId')

# 3. Get page structure
curl -s "http://localhost:9867/tabs/$TAB/snapshot?filter=interactive" | jq

# 4. Click element using the tabId
curl -s -X POST http://localhost:9867/tabs/$TAB/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'
```

---

## Characteristics

- **Server-first** — The default process is the control-plane server, not a raw browser wrapper
- **Bridge-backed instances** — Managed instances run as isolated bridge runtimes behind the server
- **Tab-Centric** — Everything revolves around tabs, not URLs
- **Stateful** — Sessions persist between requests. Log in once, stay logged in across restarts
- **Token Inexpensive** — Text extraction at 800 tokens/page (5-13x cheaper than full snapshots)
- **Flexible Modes** — Launch headed or headless browsers, keep persistent profiles, or attach to external Chrome when allowed
- **Monitoring & Control** — Tab locking for multi-agent safety, stealth mode for bot detection bypass

---

## Features

- 🌲 **Accessibility Tree** — Structured DOM with stable refs (e0, e1...) for click, type, read. No coordinate guessing.
- 🎯 **Smart Filters** — `?filter=interactive` returns only buttons, links, inputs. Fewer tokens per snapshot.
- 🕵️ **Stealth Mode** — Patches `navigator.webdriver`, spoofs UA, hides automation flags for bot detection bypass.
- 📝 **Text Extraction** — Readability mode (clean) or raw (full HTML). Choose based on workflow.
- 🖱️ **Direct Actions** — Click, type, fill, press, focus, hover, select, scroll by ref or selector.
- ⚡ **JavaScript Execution** — Run arbitrary JS in any tab. Escape hatch for workflow gaps.
- 📸 **Screenshots** — JPEG output with quality control.
- 📄 **PDF Export** — Full pages to PDF with headers, footers, landscape mode.
- 🎭 **Multi-Tab** — Create, switch, close tabs. Work with multiple pages concurrently.

## Expert Guides

If you are moving beyond the primary server-first workflow, use the expert guides:

- [Bridge Mode](guides/expert-bridge-mode.md) — run the single-instance runtime directly
- [Attach](guides/expert-attach.md) — register externally managed Chrome instances
- [Multi-Instance Strategies](guides/expert-strategies.md) — advanced routing and allocation behavior

---

## Support & Community

- **GitHub Issues** — https://github.com/pinchtab/pinchtab/issues
- **Discussions** — https://github.com/pinchtab/pinchtab/discussions
- **Twitter/X** — [@pinchtabdev](https://x.com/pinchtabdev)

---

## License

[MIT](https://github.com/pinchtab/pinchtab?tab=MIT-1-ov-file#readme)
