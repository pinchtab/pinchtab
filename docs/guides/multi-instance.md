# Multi-Instance

PinchTab can run multiple isolated Chrome instances at the same time. Each running instance has its own browser process, port, tabs, and profile-backed state.

## Mental Model

- a profile is stored browser state on disk
- an instance is a running Chrome process
- one profile can have at most one active managed instance at a time
- tabs belong to an instance, and tab IDs should be treated as opaque values returned by the API

## Start The Orchestrator

```bash
pinchtab server
```

By default the orchestrator listens on `http://localhost:9867`.

## Start An Instance

Use the explicit instance API when you want predictable multi-instance behavior:

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headed","port":"9999"}'
# CLI Alternative
pinchtab instance start --mode headed --port 9999
# Response
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "instance-1741410000000",
  "port": "9999",
  "headless": false,
  "status": "starting"
}
```

Notes:

- `POST /instances/launch` still exists as a compatibility endpoint, but it now follows the same semantics as `POST /instances/start`.
- If you omit `profileId`, PinchTab creates a managed instance with an auto-generated profile name.
- Starting an instance is only optional in workflows that use shorthand routes with auto-launch behavior, such as the `simple` strategy. In `explicit`, you should assume you need to start one yourself.

## Open A Tab In A Specific Instance

```bash
curl -X POST http://localhost:9867/instances/inst_0a89a5bb/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'
# Response
{
  "tabId": "8f9c7d4e1234567890abcdef12345678",
  "url": "https://pinchtab.com",
  "title": "PinchTab"
}
```

For follow-up operations, keep using the returned `tabId`:

```bash
curl "http://localhost:9867/tabs/<tabId>/snapshot"
curl "http://localhost:9867/tabs/<tabId>/text"
curl "http://localhost:9867/tabs/<tabId>/metrics"
```

## Reuse A Persistent Profile

List existing profiles first:

```bash
curl http://localhost:9867/profiles
```

Then start an instance for a known profile:

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"278be873adeb","mode":"headless"}'
# CLI Alternative
pinchtab instance start --profile 278be873adeb --mode headless
```

Because a profile can have only one active managed instance, starting the same profile again while it is already active returns an error instead of creating a duplicate browser.

## Monitor Running Instances

```bash
curl http://localhost:9867/instances
curl http://localhost:9867/instances/inst_0a89a5bb
curl http://localhost:9867/instances/inst_0a89a5bb/tabs
curl http://localhost:9867/instances/metrics
```

Useful fields:

- `id`: stable instance identifier
- `profileId` and `profileName`: the profile backing that instance
- `port`: the instance's HTTP port
- `headless`: whether Chrome was launched headless
- `status`: usually `starting`, `running`, `stopping`, or `stopped`

## Stop An Instance

```bash
curl -X POST http://localhost:9867/instances/inst_0a89a5bb/stop
# CLI Alternative
pinchtab instance stop inst_0a89a5bb
# Response
{
  "id": "inst_0a89a5bb",
  "status": "stopped"
}
```

Stopping the instance frees its port. If the profile is persistent, its browser state remains on disk.

## Port Allocation

If you do not pass a port, PinchTab allocates one from the configured range:

```json
{
  "multiInstance": {
    "instancePortStart": 9868,
    "instancePortEnd": 9968
  }
}
```

When an instance stops, its port becomes available for reuse.

## When To Use Explicit Multi-Instance APIs

Prefer explicit instance APIs when:

- multiple browser sessions must stay isolated
- you want separate headed and headless browsers at the same time
- you need stable profile-to-instance ownership rules
- you are building tooling that should never depend on implicit auto-launch
