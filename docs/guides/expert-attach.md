# Expert Guide: Attach

This guide is for advanced setups where Chrome already exists outside Pinchtab and you want the server to register it as an instance.

## What Attach Means

Attach does not launch Chrome.

Instead:
- Chrome is already running somewhere else
- Pinchtab server receives a `cdpUrl`
- Pinchtab registers that browser as an instance
- requests can then be routed through the server

Mental model:

```text
launch  = Pinchtab creates the browser
attach  = Pinchtab registers an existing browser
```

## When Attach Makes Sense

Use attach when:
- Chrome is managed by another system
- Chrome runs in another container or service
- you need Pinchtab routing around an existing CDP endpoint
- you want the server to treat external Chrome as an instance

## When Attach Does Not Make Sense

Do not use attach if your goal is simply:
- “start a browser for my agent”
- “run Pinchtab locally as my browser service”

In those cases, use managed launch via the full server.

## Enable Attach Policy

Attach is controlled by config policy.

Example:

```json
{
  "attach": {
    "enabled": true,
    "allowHosts": ["127.0.0.1", "localhost", "::1"],
    "allowSchemes": ["ws", "wss"]
  }
}
```

This does not define a browser.
It only defines which attach requests are allowed.

## Attach Request

Example:

```bash
curl -X POST http://localhost:9867/instances/attach \
  -H "Content-Type: application/json" \
  -d '{
    "name": "shared-chrome",
    "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/..."
  }'
```

## Getting A CDP URL

If you are attaching to a browser you started yourself, the usual flow is:

```bash
# Start Chrome with remote debugging enabled
google-chrome --remote-debugging-port=9222

# Or on some systems:
# chromium --remote-debugging-port=9222
```

Then ask Chrome for its browser-level websocket endpoint:

```bash
curl http://localhost:9222/json/version
```

Example response:

```json
{
  "webSocketDebuggerUrl": "ws://localhost:9222/devtools/browser/abc123"
}
```

That `webSocketDebuggerUrl` is the value you pass as `cdpUrl` in the attach request.

Example:

```bash
curl -X POST http://localhost:9867/instances/attach \
  -H "Content-Type: application/json" \
  -d '{
    "name": "shared-chrome",
    "cdpUrl": "ws://localhost:9222/devtools/browser/abc123"
  }'
```

## What Pinchtab Owns

By default, attached instances are externally owned:

```text
source    = attached
runtime   = direct-cdp
ownership = external
```

That means:
- Pinchtab routes to them
- Pinchtab did not launch them
- lifecycle ownership may remain outside Pinchtab

## Security Considerations

Attach should be treated as an expert feature because it widens trust boundaries.

Recommended rules:
- keep `attach.enabled` off by default
- restrict `allowHosts`
- restrict `allowSchemes`
- require `PINCHTAB_TOKEN` when the server is reachable by anything other than localhost
- only attach to CDP endpoints you trust

## Operational Model

```text
agent -> pinchtab server -> attached external Chrome
```

There is no bridge child in the attach path.

## Current Scope

Attach is the right abstraction for:
- external Chrome
- containerized Chrome managed elsewhere
- CDP endpoints outside Pinchtab lifecycle control

It is not the primary user path.
The primary user path remains:

```bash
pinchtab
```
