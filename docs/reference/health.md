# Health

Check server status and availability.

`/health` is deliberately **not** the endpoint to compare across modes: in server
mode it is the orchestrator's own envelope, and an orchestrator has facts
(`instances`, `profiles`, `defaultInstance`) a bridge does not. Failure and crash
telemetry is served in the same shape by both modes, but not by the same endpoint:
`/metrics` reports the counters of the process answering it, so in server mode it is
the front door's own layer and a browser crash never appears there, while in bridge
mode it is the instance layer. To compare a bridge repro against a server-mode
problem, read the server-mode instance at `/instances/{id}/metrics`, which answers
from the same layer a bridge's `/metrics` does. Every metrics response names its
`layer` (`frontDoor` or `instance`); two readings are comparable only when that field
agrees. [`/metrics`](./metrics.md) has the layer table.

## Bridge Mode

```bash
curl http://localhost:9867/health
# Response: {"status":"ok","tabs":1}

# CLI Alternative (human-readable by default)
pinchtab health
# Output: ok

pinchtab health --json              # Full JSON response
```

Bridge-mode health also reports `version`, and may include:

- `crashLogs`
- `failures`
- `crashes`

In error cases it returns `503` with `status: "error"` and a `reason`.

## Server Mode (Dashboard)

In full server mode, `/health` returns the dashboard health envelope:

```bash
curl http://localhost:9867/health
# Response
{
  "status": "ok",
  "mode": "dashboard",
  "version": "0.8.0",
  "uptime": 12345,
  "authRequired": true,
  "profiles": 1,
  "instances": 1,
  "defaultInstance": {
    "id": "inst_abc12345",
    "status": "running"
  },
  "agents": 0,
  "restartRequired": false
}
```

| Field | Description |
| --- | --- |
| `status` | `ok` when server is healthy |
| `mode` | `dashboard` in server mode |
| `version` | PinchTab version |
| `uptime` | Milliseconds since server start |
| `authRequired` | `true` when a server token is configured |
| `profiles` | Number of configured profiles |
| `instances` | Number of managed instances |
| `defaultInstance` | First managed instance info, when present |
| `agents` | Connected agent count |
| `restartRequired` | `true` when file-based config changes need restart |
| `restartReasons` | Restart reason list when required |
| `crashes` | Present once any instance's browser has crashed: `total` and `recent`, the same block bridge `/health` carries, each event naming its `instanceId` |
| `security` | Present only for a bearer-token or cookie-authenticated caller. The front-door process's own configured policy, nothing more: `scope` carries the literal `frontDoorConfiguration` so a client can key on it, then `level`, `bind`, `allowedDomains`, `idpiEnabled`, `enabledSensitiveEndpoints` and `guardsDown`. What the instance processes enforce is `enforcedSecurity` |
| `enforcedSecurity` | Present beside `security`. What the instance processes are enforcing, which is the policy each snapshotted at its own boot. `instances[]` lists RUNNING instances only, so an absent id is an instance that was not running, never one that matched. Each entry carries `id`, `queried`, `comparison` and `policy`. `divergent` is `true` whenever any listed instance is not `match`, so it is raised by `unknown` as well as by a real difference and does not on its own prove the policies differ |
| `enforcedSecurity.instances[].comparison` | Three states. `match`: the instance's enforced policy equals the front door's current configuration. `diverges`: it differs from the front door's CURRENT configuration, which is not by itself a misconfiguration: a security change saved while the instance kept running reports `diverges` until the instance is replaced, and an instance launched with a per-instance `securityPolicy.allowedDomains` reports `diverges` for its whole life because the extra domains are merged into the child's policy. `unknown`: the instance did not answer, so `queried` is `false`; it is not an instance enforcing nothing |
| `enforcedSecurity.instances[].policy` | The enforced `idpiEnabled`, `allowedDomains`, `enabledSensitiveEndpoints` and `guardsDown`. Absent, not empty, when `queried` is `false` |

Notes:

- `defaultInstance` is present when at least one instance exists
- use `defaultInstance.status == "running"` when you want to confirm Chrome is ready
- strategies such as `always-on` can create an instance automatically at startup
- `status` does not degrade on a browser crash: the instance is relaunched and is serving again. The crash is history, so it rides beside `status` as `crashes`; a crashed-then-relaunched instance differs from one that never crashed by that key alone. Every tab the dead browser held is gone, and a call to one answers `404` with code `browser_crashed` and a `hint` saying so

## Related Pages

- [Tabs](./tabs.md)
- [Navigate](./navigate.md)
- [Strategies](./strategies.md)
