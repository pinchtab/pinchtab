# PinchTab

Welcome to PinchTab: browser control for AI agents, scripts, and automation workflows.

## What PinchTab is

PinchTab is a standalone HTTP server that gives you direct control over Chrome through a CLI and HTTP API.

PinchTab has two runtimes:

- `pinchtab server`: the server
- `pinchtab bridge`: the single-instance bridge runtime

The server is the normal entry point. It manages profiles, instances, routing, security policy, and the dashboard.
The bridge is the lightweight per-instance HTTP runtime used behind managed child instances.

The basic model is:

- start the server
- start or attach instances
- operate on tabs

## Main usage patterns

Start `pinchtab server` or even better `pinchtab daemon install` and leave it running:

- use it as a browser for agents
- use it as a local automation endpoint
- attach an existing debug browser when needed

## Minimal working flow

### 1. Start the server

```bash
pinchtab server
```

### 2. Start an instance

By default we use always-on strategy. This is optional now and not necessary.

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}'
# CLI Alternative
pinchtab instance start
# Response
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "instance-1741400000000000000",
  "port": "9868",
  "headless": true,
  "status": "starting"
}
```

### 3. Navigate

```bash
curl -s -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}' | jq .
# CLI Alternative
pinchtab nav https://pinchtab.com
# Response
{
  "tabId": "CDP_TARGET_ID",
  "title": "PinchTab",
  "url": "https://pinchtab.com"
}
```

### 4. Inspect interactive elements

```bash
curl -s "http://localhost:9867/snapshot?filter=interactive" | jq .
# CLI Alternative
pinchtab snap -i -c
# Response
{
  "nodes": [
    { "ref": "e0", "role": "link", "name": "Docs" },
    { "ref": "e1", "role": "button", "name": "Get started" }
  ]
}
```

### 5. Click by ref

```bash
curl -s -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e1"}' | jq .
# CLI Alternative
pinchtab click e1
# Response
{
  "success": true,
  "result": {
    "clicked": true
  }
}
```

## Characteristics

- Server-first: the main process is the control-plane server
- Bridge-backed instances: managed instances run behind isolated bridge runtimes
- Tab-oriented: interaction happens at the tab level
- Stateful: profiles persist cookies and browser state
- Token-efficient: snapshot and text endpoints are cheaper than screenshot-driven workflows
- Flexible: headless, headed, profile-backed, or attached Chrome
- Controlled: health, metrics, auth, and tab locking are built into the system

## Common features

- Accessibility-tree snapshots with `e0`, `e1`, and similar refs
- Text extraction
- Direct actions such as click, type, fill, press, focus, hover, select, and scroll
- Screenshots and PDF export
- Multi-instance orchestration
- External Chrome attach
- Optional JavaScript evaluation

## Support

- [GitHub Issues](https://github.com/pinchtab/pinchtab/issues)
- [GitHub Discussions](https://github.com/pinchtab/pinchtab/discussions)
- [@pinchtabdev](https://x.com/pinchtabdev)

## License

[MIT](https://github.com/pinchtab/pinchtab?tab=MIT-1-ov-file#readme)
