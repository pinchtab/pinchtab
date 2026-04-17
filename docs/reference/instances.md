# Instances

Instances are running Chrome processes managed by PinchTab. Each managed instance has:

- an instance ID
- a profile
- a port
- a mode (`headless` or `headed`)
- an execution status

One profile can have at most one active managed instance at a time.

## List Instances

```bash
curl http://localhost:9867/instances
# CLI Alternative
pinchtab instances
```

`pinchtab instances` is the simplest way to inspect the current fleet from the CLI.

Response shape:

```json
[
  {
    "id": "inst_0a89a5bb",
    "profileId": "prof_278be873",
    "profileName": "instance-1741410000000",
    "port": "9999",
    "mode": "headed",
    "headless": false,
    "status": "running",
    "securityPolicy": {
      "allowedDomains": ["127.0.0.1", "localhost", "::1", "wikipedia.org"]
    }
  }
]
```

`GET /instances` returns a bare JSON array, not an envelope like `{"instances":[...]}`. Each instance response includes both `mode` (`"headless"` or `"headed"`) and the legacy-compatible `headless` boolean.

## Start An Instance

### `POST /instances/start`

Use `/instances/start` when you want to start by profile ID or profile name, or let PinchTab create a temporary profile.

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"prof_278be873","mode":"headed","port":"9999","securityPolicy":{"allowedDomains":["wikipedia.org","wikimedia.org"]}}'
# CLI Alternative
pinchtab instance start --profile prof_278be873 --mode headed --port 9999 --allow-domain wikipedia.org --allow-domain wikimedia.org
```

Request body:

- `profileId`: optional; accepts a profile ID or an existing profile name
- `mode`: optional; use `headed` for a visible browser, anything else is treated as headless
- `port`: optional
- `securityPolicy.allowedDomains`: optional additive instance-scoped IDPI/domain allowlist entries

Notes:

- if `profileId` is omitted, PinchTab creates an auto-generated temporary profile
- if `port` is omitted, PinchTab allocates one from the configured instance port range
- the CLI flag is `--profile`, even though the API field is `profileId`
- `securityPolicy.allowedDomains` is merged with the server-level `security.allowedDomains` baseline for that instance only
- you can widen a single instance without changing the server default. For example, `{"securityPolicy":{"allowedDomains":["*"]}}` makes that instance unrestricted while other instances still use the server baseline
- request-supplied extension paths are rejected; configure `browser.extensionPaths` on the server instead. By default, PinchTab uses the local `extensions/` directory under its state/config folder.

### `POST /instances/launch`

`/instances/launch` is a compatibility alias for `/instances/start`.

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"profileId":"prof_278be873","mode":"headed","securityPolicy":{"allowedDomains":["wikipedia.org"]}}'
```

Request body:

- `profileId`: optional existing profile ID or existing profile name
- `mode`: optional; `headed` or headless by default
- `port`: optional
- `securityPolicy.allowedDomains`: optional additive instance-scoped IDPI/domain allowlist entries

Important:

- `/instances/launch` does not read a `headless` field. Use `mode:"headed"` when you want a headed browser.
- `name` is no longer supported on `/instances/launch`. Create the profile first via `POST /profiles`, then use the returned `id` as `profileId`.
- request-supplied extension paths are rejected; configure `browser.extensionPaths` on the server instead. By default, PinchTab uses the local `extensions/` directory under its state/config folder.

## Get One Instance

```bash
curl http://localhost:9867/instances/inst_ea2e747f
```

Common status values:

- `starting`
- `running`
- `stopping`
- `stopped`
- `error`

Instance responses include:

- `mode`: `"headless"` or `"headed"`
- `headless`: boolean kept for compatibility

## Get Instance Logs

```bash
curl http://localhost:9867/instances/inst_ea2e747f/logs
# CLI Alternative
pinchtab instance logs inst_ea2e747f
```

Response is plain text. There is also an SSE stream at `GET /instances/{id}/logs/stream`.

## Stop An Instance

```bash
curl -X POST http://localhost:9867/instances/inst_ea2e747f/stop
# CLI Alternative
pinchtab instance stop inst_ea2e747f
```

Stopping an instance preserves the profile unless it was a temporary auto-generated profile.

## Start By Profile

You can also start an instance from a profile-oriented route:

```bash
curl -X POST http://localhost:9867/profiles/prof_278be873/start \
  -H "Content-Type: application/json" \
  -d '{"headless":false,"port":"9999","securityPolicy":{"allowedDomains":["wikipedia.org"]}}'
```

This route accepts a profile ID or profile name in the path. Unlike `/instances/start` and `/instances/launch`, its request body uses `headless` instead of `mode`.

## Open A Tab In An Instance

```bash
curl -X POST http://localhost:9867/instances/inst_ea2e747f/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'
```

There is no dedicated instance-scoped `tab open` CLI command. The CLI shortcut is:

```bash
pinchtab instance navigate inst_ea2e747f https://pinchtab.com
```

That command opens a blank tab for the instance and then navigates it.

## List Tabs For One Instance

```bash
curl http://localhost:9867/instances/inst_ea2e747f/tabs
```

## List All Tabs Across Running Instances

```bash
curl http://localhost:9867/instances/tabs
```

This is the fleet-wide tab listing endpoint. It is different from `GET /tabs`, which is shorthand or bridge scoped.

## List Metrics Across Instances

```bash
curl http://localhost:9867/instances/metrics
```

## Attach An Existing Chrome

```bash
curl -X POST http://localhost:9867/instances/attach \
  -H "Content-Type: application/json" \
  -d '{"name":"shared-chrome","cdpUrl":"ws://127.0.0.1:9222/devtools/browser/..."}'
```

Notes:

- there is no CLI attach command
- attach is allowed only when enabled in config under `security.attach`
- `security.attach.allowHosts` must allow the `cdpUrl` host
- `allowHosts: ["*"]` is a documented, non-default, security-reducing override. It disables host allowlisting entirely and allows any reachable CDP host with an allowed scheme. Use it only on isolated, operator-controlled networks.

## Attach An Existing Bridge

```bash
curl -X POST http://localhost:9867/instances/attach-bridge \
  -H "Content-Type: application/json" \
  -d '{
    "name":"shared-bridge",
    "baseUrl":"http://10.0.12.24:9868",
    "token":"bridge-secret-token"
  }'
```

Notes:

- `baseUrl` must be a bare bridge origin; do not include credentials, query strings, fragments, or a path
- the orchestrator performs a health check before registering it
- `security.attach.allowHosts` must allow the bridge host
- `allowHosts: ["*"]` is a documented, non-default, security-reducing override. It disables host allowlisting entirely and allows any reachable bridge host with an allowed scheme. Use it only on isolated, operator-controlled networks.
