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

- `POST /instances/attach` — starts a child `pinchtab bridge --cdp-attach ...`
  process that wraps the external browser. The bridge speaks the normal
  PinchTab HTTP API; the orchestrator registers the bridge's HTTP URL (not
  the raw `ws://` CDP URL) as the routable instance URL.
- `POST /instances/attach-bridge` — registers an already-running PinchTab
  bridge as an instance (unchanged).
- attach policy in config under `security.attach`
- attached-instance metadata in `GET /instances`

The attach request body is:

```json
{
  "name": "shared-chrome",
  "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/...",
  "provider": "chrome",
  "browserTarget": "chrome-local"
}
```

`browserTarget` is optional. When `browser.targets` is configured, an omitted
value attaches to the configured default target and the target's browser is used.
If you also pass `provider`, it must match that target. Without browser targets,
`provider` defaults to `chrome`; use `cloak` for a CloakBrowser endpoint
(equivalent to `--browser cloak` on the CLI).

Accepted `cdpUrl` shapes:

- browser-level WebSocket URL: `ws://host:port/devtools/browser/<id>`
- HTTP DevTools origin: `http://host:port` (resolved through `/json/version`)
- HTTP `/json/version` URL

Page-level URLs (`/devtools/page/...`) are rejected.

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
      "allowSchemes": ["ws", "wss"],
      "forwardProxyAuth": false
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
    "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/abc123",
    "browserTarget": "chrome-local"
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
  "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/abc123",
  "browserTarget": "chrome-local",
  "browserProvider": "chrome"
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

Attached instances are externally owned, but PinchTab still owns a *bridge
wrapper* around them.

That means:

- PinchTab did not launch the browser
- PinchTab spawned a child `pinchtab bridge --cdp-attach ...` process that wraps
  the external CDP endpoint and serves normal PinchTab routes
- the external Chrome/CloakBrowser process remains outside PinchTab lifecycle
  ownership

In practical terms:

- `POST /instances/{id}/stop` shuts down the child PinchTab bridge — the
  external Chrome process is **left running**
- routes like `/tabs`, `/snapshot`, `/action`, `/screenshot` go to the child
  bridge, which talks CDP to the external browser through a
  `chromedp.NewRemoteAllocator`

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
- leave `forwardProxyAuth` disabled unless the attached browser process and CDP transport are trusted
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

---

## Related guides

- [cloakbrowser.md](cloakbrowser.md) — full CloakBrowser configuration and
  the `--browser cloak` attach variant
- [docker.md](docker.md) — running PinchTab (and the local CloakBrowser
  smoke image) in containers
- [headed-mode.md](headed-mode.md) — manual headed setup outside the
  bundled image
- [security.md](security.md) — attach policy details, IDPI, token handling
