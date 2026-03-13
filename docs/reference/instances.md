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
# Response
[
  {
    "id": "inst_0a89a5bb",
    "profileId": "prof_278be873",
    "profileName": "work",
    "port": "9868",
    "headless": true,
    "status": "running",
    "startTime": "2026-03-01T05:21:38.27432Z"
  }
]
```

`pinchtab instances` is the simplest way to inspect the current fleet from the CLI.

## Start An Instance

### Primary: Start by profileId and mode

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"prof_278be873","mode":"headed","port":"9999"}'
# CLI Alternative
pinchtab instance start --profileId prof_278be873 --mode headed --port 9999
# Response
{
  "id": "inst_ea2e747f",
  "profileId": "prof_278be873",
  "profileName": "work",
  "port": "9999",
  "headless": false,
  "status": "starting",
  "startTime": "2026-03-01T05:21:38.27432Z"
}
```

Notes:

- if `profileId` is omitted, PinchTab creates an auto-generated temporary profile such as `instance-...`
- if `port` is omitted, PinchTab allocates one from the configured instance port range

### Alternative: Start by profile name

You can also start an instance using a profile name and headless flag:

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"work","headless":false}'
# Response
{
  "id": "inst_ea2e747f",
  "profileId": "prof_278be873",
  "profileName": "work",
  "port": "9999",
  "headless": false,
  "status": "starting",
  "startTime": "2026-03-01T05:21:38.27432Z"
}
```

The `POST /instances/launch` endpoint takes:
- `name` — profile name (required)
- `headless` — true for headless mode, false for headed mode (optional, defaults to true)

Port is automatically allocated from the configured instance port range.

## Get One Instance

```bash
curl http://localhost:9867/instances/inst_ea2e747f
# Response
{
  "id": "inst_ea2e747f",
  "profileId": "prof_278be873",
  "profileName": "work",
  "port": "9999",
  "headless": false,
  "status": "running",
  "startTime": "2026-03-01T05:21:38.27432Z"
}
```

Common status values:

- `starting`
- `running`
- `stopping`
- `stopped`
- `error`

## Get Instance Logs

```bash
curl http://localhost:9867/instances/inst_ea2e747f/logs
# CLI Alternative
pinchtab instance logs inst_ea2e747f
```

Response is plain text.

## Stop An Instance

```bash
curl -X POST http://localhost:9867/instances/inst_ea2e747f/stop
# CLI Alternative
pinchtab instance stop inst_ea2e747f
# Response
{
  "id": "inst_ea2e747f",
  "status": "stopped"
}
```

Stopping an instance preserves the profile unless it was a temporary auto-generated profile.

## Start By Profile

You can also start an instance from a profile-oriented route:

```bash
curl -X POST http://localhost:9867/profiles/prof_278be873/start \
  -H "Content-Type: application/json" \
  -d '{"headless":false,"port":"9999"}'
# Response
{
  "id": "inst_ea2e747f",
  "profileId": "prof_278be873",
  "profileName": "work",
  "port": "9999",
  "headless": false,
  "status": "starting"
}
```

This route accepts a profile ID or profile name in the path. Its request body uses `headless` rather than `mode`.

## Open A Tab In An Instance

```bash
curl -X POST http://localhost:9867/instances/inst_ea2e747f/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'
# Response
{
  "tabId": "8f9c7d4e1234567890abcdef12345678",
  "url": "https://pinchtab.com",
  "title": "PinchTab"
}
```

There is no dedicated instance-scoped `tab open` CLI command today. The CLI shortcut is:

```bash
pinchtab instance navigate inst_ea2e747f https://pinchtab.com
```

That command opens a tab for the instance and then navigates it.

## List Tabs For One Instance

```bash
curl http://localhost:9867/instances/inst_ea2e747f/tabs
# Response
[
  {
    "id": "8f9c7d4e1234567890abcdef12345678",
    "instanceId": "inst_ea2e747f",
    "url": "https://pinchtab.com",
    "title": "PinchTab"
  }
]
```

## List All Tabs Across Running Instances

```bash
curl http://localhost:9867/instances/tabs
```

This is the fleet-wide tab listing endpoint. It is different from `GET /tabs`, which is shorthand/bridge-style and not a fleet-wide inventory.

## List Metrics Across Instances

```bash
curl http://localhost:9867/instances/metrics
```

Use this when you want per-instance memory metrics across all running instances.

## Attach An Existing Chrome

```bash
curl -X POST http://localhost:9867/instances/attach \
  -H "Content-Type: application/json" \
  -d '{"name":"shared-chrome","cdpUrl":"ws://127.0.0.1:9222/devtools/browser/..."}'
# Response
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "shared-chrome",
  "status": "running",
  "attached": true,
  "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/..."
}
```

Notes:

- there is no CLI attach command
- attach is allowed only when enabled in config under `security.attach`
