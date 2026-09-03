# Metrics

Runtime counters, recent failures and crash diagnostics.

```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:9867/metrics
```

## Two layers, never summed

Server mode runs **two processes**, and each keeps its own counters:

| Layer | Process | Sees |
|-------|---------|------|
| `frontDoor` | the orchestrator that owns the port | auth rejections, unrouted paths, everything before a request is forwarded |
| `instance` | a browser-control child (also what `pinchtab bridge` runs standalone) | every request that reached a browser |

Every response says which layer its numbers describe:

```json
{
  "layer": "frontDoor",
  "metrics": { "requestsTotal": 41, "requestsFailed": 7, "avgLatencyMs": 3.1, "...": "..." },
  "failures": {
    "layer": "frontDoor",
    "requestsFailed": 7,
    "recent": [
      { "time": "...", "requestId": "a1b2c3", "method": "GET", "path": "/tabs",
        "status": 401, "type": "http_error", "layer": "frontDoor" }
    ]
  },
  "crashes": { "total": 0, "recent": [] }
}
```

A failure is not always a 4xx. `POST /actions` and `POST /macro` answer 200 with per-item
results by design, so a run whose steps failed publishes a failure reason instead of a
status: it counts in `requestsFailed`, appears in `failures.recent` with the run's path,
`status: 200`, and a message of the form `N of M steps failed: <first step's error>`, and
logs at `WARN`. Watching `requestsFailed` therefore sees an agent whose batches are all
failing, which is the shape of traffic these endpoints exist to serve.

This holds at **both** layers. The reason travels the proxy hop on its own response
headers, so a batch served by an instance and proxied by the front door moves the front
door's `requestsFailed` too — which is the number an operator running `pinchtab server`
curls, since `GET /metrics` there reports the front door's own counters. A proxied
response carrying no reason records nothing, whatever its status.

**The two layers are never added together.** A single `requestsTotal` covering both
would mean two things at once, and an operator cannot act on that number: a spike
would not say whether clients are being turned away at the door or the browser is
failing. Read each layer at its own endpoint instead:

| Endpoint | Answered by | Reports |
|----------|-------------|---------|
| `GET /metrics` | front door (server mode) / the bridge itself (bridge mode) | that process's own counters |
| `GET /instances/{id}/metrics` | proxied to that instance | that instance's counters |
| `GET /instances/metrics` | front door | per-instance **browser memory**, not request counters |
| `GET /tabs/{id}/metrics` | proxied to the owning instance | that instance's **browser memory** — the whole process tree, not the tab |

The tab id in the last row selects the instance to ask, never the measurement. There
is no per-tab reading behind it: two tabs of one instance answer identically, and the
figure is the same one that instance reports for itself. See
[Memory monitoring](../guides/memory-monitoring.md).

`failures.recent` repeats `layer` on each event as well as on the block, so an
event pasted into a bug report still says where it came from.

`failures` and `crashes` are always present, empty when there is nothing to
report. A key that appeared only after the first failure could not be relied on by
anything watching for one.

## What the two modes share, and what they do not

Shared, by construction — both modes serve the same diagnostics payload:

- `metrics` (request counters, latency, Go runtime figures)
- `failures` with `requestsFailed` and `recent`
- `crashes` with `total` and `recent`
- `version` on `/health`

Deliberately different:

- **`/metrics` is answered locally in both modes; `/health` is not.** In server mode
  `/health` is the orchestrator's own envelope (`instances`, `profiles`,
  `defaultInstance`, `restartRequired`), because those facts exist only at the front
  door. A bridge has no instances to report. The two `/health` bodies therefore
  differ on purpose, and `/metrics` is the endpoint to compare across modes.
- **`memory` appears only on an instance's `/metrics`**, since only a process
  holding a browser can measure one.
- **Browser crash events are recorded by whichever process owns the browser**, so in
  server mode a crash shows up under the instance layer, not the front door — the
  front door's `crashes` block stays empty by design.

## Related Pages

- [Health](./health.md)
- [Instances](./instances.md)
