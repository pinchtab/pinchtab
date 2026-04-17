# Attach Chrome

Use this guide when:

- Chrome already exists outside PinchTab
- you want the PinchTab server to register that browser as an instance
- you already have a browser-level DevTools WebSocket URL

Do not use this guide if your goal is simply:

- start a browser for your agent
- run the normal local PinchTab workflow

For that, use managed instances with `pinchtab` and `POST /instances/start`.

---

## Launch vs attach

The mental model is:

```text
launch = PinchTab starts and owns the browser
attach = PinchTab registers an already running browser
```

With attach:

- Chrome is started somewhere else
- PinchTab receives a `cdpUrl`
- the server registers that browser as an attached instance

---

## What is implemented today

The current codebase implements:

- `POST /instances/attach`
- attach policy in config under `security.attach`
- attached-instance metadata in `GET /instances`

The attach request body is:

```json
{
  "name": "shared-chrome",
  "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/..."
}
```

There is currently no CLI attach command.

---

## Step 1: enable attach policy

Attach is disabled unless you allow it in config.

Example:

```json
{
  "security": {
    "attach": {
      "enabled": true,
      "allowHosts": ["127.0.0.1", "localhost", "::1"],
      "allowSchemes": ["ws", "wss"]
    }
  }
}
```

What this does:

- enables the attach endpoint
- restricts which hosts are accepted
- restricts which URL schemes are accepted

What this does not do:

- it does not start Chrome
- it does not define a global remote browser
- it does not replace managed instances

---

## Step 2: start Chrome with remote debugging

Example:

```bash
google-chrome --remote-debugging-port=9222
# Or on some systems:
# chromium --remote-debugging-port=9222
```

This makes Chrome expose a browser-level DevTools endpoint.

---

## Step 3: get the browser WebSocket URL

Query Chrome:

```bash
curl -s http://127.0.0.1:9222/json/version | jq .
# Response
{
  "webSocketDebuggerUrl": "ws://127.0.0.1:9222/devtools/browser/abc123"
}
```

The value of `webSocketDebuggerUrl` is the `cdpUrl` you pass to PinchTab.

---

## Step 4: attach it to PinchTab

```bash
curl -X POST http://localhost:9867/instances/attach \
  -H "Content-Type: application/json" \
  -d '{
    "name": "shared-chrome",
    "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/abc123"
  }'
# Response
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "shared-chrome",
  "port": "",
  "mode": "headed",
  "headless": false,
  "status": "running",
  "attached": true,
  "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/abc123"
}
```

Notes:

- `name` is optional; if omitted, the server generates one like `attached-...`
- the server validates the URL against `security.attach.allowHosts` and `security.attach.allowSchemes`

---

## Step 5: confirm it is registered

```bash
curl -s http://localhost:9867/instances | jq .
# CLI Alternative
pinchtab instances
```

An attached instance appears in the normal instance list with:

- `attached: true`
- `cdpUrl: ...`
- `status: "running"`

---

## Ownership and lifecycle

Attached instances are externally owned.

That means:

- PinchTab did not launch the browser
- PinchTab stores metadata about that browser as an instance
- the external Chrome process remains outside PinchTab lifecycle ownership

In practical terms:

- stopping the attached instance in PinchTab unregisters it from the server
- it does not imply that PinchTab launched or can fully manage the external Chrome process

---

## When attach makes sense

Use attach when:

- Chrome is managed by another system
- Chrome is already running in a separate service or container
- you want the server to know about an externally managed browser
- you want to keep browser ownership outside PinchTab

---

## Security

Attach widens the trust boundary, so keep it locked down.

Recommended rules:

- leave attach disabled unless you need it
- keep `allowHosts` narrow
- keep `allowSchemes` narrow
- set `PINCHTAB_TOKEN` when the server is reachable outside localhost
- only attach to CDP endpoints you trust

If you set `allowHosts` to `["*"]`, PinchTab accepts any reachable attach host with an allowed scheme. That is a documented, non-default, security-reducing override: it removes host allowlisting entirely and should only be used on isolated, operator-controlled networks.

Also remember:

- Chrome DevTools gives powerful browser control
- a reachable CDP endpoint should be treated as sensitive infrastructure

If Chrome is remote, prefer a tunnel rather than exposing the debugging port broadly.

---

## Operational model

The intended model is:

```text
agent -> PinchTab server -> attached external Chrome
```

This is an expert path, not the default user path.

The default path remains:

```bash
pinchtab
```

then managed instance start via:

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}'
# CLI Alternative
pinchtab instance start
```
