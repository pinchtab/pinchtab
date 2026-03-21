# Health

Check server status and availability.

## Bridge Mode

```bash
curl http://localhost:9867/health
# CLI Alternative
pinchtab health
# Response
{
  "status": "ok",
  "tabs": 1
}
```

Bridge-mode health may also include:

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

Notes:

- `defaultInstance` is present when at least one instance exists
- use `defaultInstance.status == "running"` when you want to confirm Chrome is ready
- strategies such as `always-on` can create an instance automatically at startup

## Related Pages

- [Tabs](./tabs.md)
- [Navigate](./navigate.md)
- [Strategies](./strategies.md)
