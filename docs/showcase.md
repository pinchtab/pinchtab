# Showcase

## Browser for your agents

Start the server and one instance first:

```bash
pinchtab server
#or
pinchtab daemon install
```

Starting an instance may be optional, depending on strategy/config.

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

### Navigate

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

### Snapshot

```bash
curl -s "http://127.0.0.1:9867/snapshot?filter=interactive" | jq .
# CLI Alternative
pinchtab snap -i -c
# Response
{
  "nodes": [
    { "ref": "e0", "role": "link", "name": "Skip to content" },
    { "ref": "e1", "role": "link", "name": "GitHub Homepage" },
    { "ref": "e14", "role": "button", "name": "Search or jump to…" }
  ]
}
```

### Extract Text

```bash
curl -s http://127.0.0.1:9867/text | jq .
# CLI Alternative
pinchtab text
# Response
{
  "text": "High-performance browser automation bridge and multi-instance orchestrator...",
  "title": "GitHub - pinchtab/pinchtab",
  "url": "https://github.com/pinchtab/pinchtab"
}
```

### Click By Ref

```bash
curl -s -X POST http://127.0.0.1:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e14"}' | jq .
# CLI Alternative
pinchtab click e14
# Response
{
  "success": true,
  "result": {
    "clicked": true
  }
}
```

### Screenshot

```bash
curl -s http://127.0.0.1:9867/screenshot > smoke.jpg
ls -lh smoke.jpg
# CLI Alternative
pinchtab ss -o smoke.jpg
# Response
Saved smoke.jpg (55876 bytes)
```

### Export a PDF

```bash
curl -s http://127.0.0.1:9867/pdf > smoke.pdf
ls -lh smoke.pdf
# CLI Alternative
pinchtab pdf -o smoke.pdf
# Response
Saved smoke.pdf (1494657 bytes)
```

## Automation tool for the web

Use PinchTab as a scriptable browser endpoint for repeatable web tasks.

### Fill a form field

```bash
curl -s -X POST http://127.0.0.1:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e3","text":"user@example.com"}' | jq .
# CLI Alternative
pinchtab fill e3 "user@example.com"
# Response
{
  "success": true,
  "result": {
    "filled": "user@example.com"
  }
}
```

### Press a key

```bash
curl -s -X POST http://127.0.0.1:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Enter"}' | jq .
# CLI Alternative
pinchtab press Enter
# Response
{
  "success": true,
  "result": {
    "pressed": "Enter"
  }
}
```

### Generate artifacts

```bash
curl -s http://127.0.0.1:9867/pdf > report.pdf
ls -lh report.pdf
# CLI Alternative
pinchtab pdf -o report.pdf
# Response
Saved report.pdf (1494657 bytes)
```

```bash
curl -s http://127.0.0.1:9867/screenshot > page.jpg
ls -lh page.jpg
# CLI Alternative
pinchtab ss -o page.jpg
# Response
Saved page.jpg (55876 bytes)
```

This fits:

- browser-driven scripts
- content extraction and reporting
- visual checks and artifacts
- automation tools that need a local browser endpoint

## Human-agent development surface

When Chrome is already running in remote-debugging mode, PinchTab can attach to it and expose it through the same API.

### 1. Start Chrome with remote debugging

```bash
google-chrome --remote-debugging-port=9222
# Or on some systems:
# chromium --remote-debugging-port=9222
```

### 2. Read the browser CDP URL

```bash
curl -s http://127.0.0.1:9222/json/version | jq .
# Response
{
  "webSocketDebuggerUrl": "ws://127.0.0.1:9222/devtools/browser/abc123"
}
```

### 3. Attach that browser to PinchTab

```bash
CDP_URL=$(curl -s http://127.0.0.1:9222/json/version | jq -r '.webSocketDebuggerUrl')

curl -s -X POST http://127.0.0.1:9867/instances/attach \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"dev-chrome\",\"cdpUrl\":\"$CDP_URL\"}" | jq .
# Response
{
  "id": "inst_abc12345",
  "profileId": "prof_def67890",
  "profileName": "dev-chrome",
  "attached": true,
  "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/abc123",
  "status": "running"
}
```

### 4. Inspect it through PinchTab

```bash
curl -s http://127.0.0.1:9867/instances | jq .
# CLI Alternative
pinchtab instances
```

This is useful when:

- you are developing in a real browser session
- you want an agent to inspect the page you already have open
- you do not want PinchTab to launch a separate managed browser
- you want one local API for both managed and attached browser work
