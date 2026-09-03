# Memory Monitoring

PinchTab exposes memory information for the Chrome processes it launches. The current implementation measures browser memory at the process level and reports browser-wide aggregates for each instance.

## What PinchTab Measures

PinchTab walks the Chrome process tree for a running instance:

1. find the main browser PID
2. enumerate child processes
3. sum RSS memory across the browser and its children
4. count renderer processes

This gives you real OS-level memory usage for that instance's Chrome process tree.

## Memory Fields

| Field | Meaning |
| --- | --- |
| `memoryMB` | Real RSS memory across the browser process tree |
| `renderers` | Number of renderer processes in the browser process tree |

Both fields are measured. Neither is derived from the other, and PinchTab reports no JavaScript-heap or DOM-count figures: there is no CDP collection behind these endpoints.

Important limitation:

- `GET /tabs/{id}/metrics` returns the owning browser instance's aggregate memory, not isolated per-tab memory

## Instance Metrics

For a single running browser:

```bash
curl http://localhost:9867/metrics
```

Example shape:

```json
{
  "metrics": {
    "goHeapAllocMB": 12.5,
    "goHeapSysMB": 24.0,
    "goNumGoroutine": 15
  },
  "memory": {
    "memoryMB": 850.5,
    "renderers": 11
  }
}
```

## Per-Tab Metrics

```bash
curl http://localhost:9867/tabs/<tabId>/metrics
```

Example shape:

```json
{
  "memoryMB": 850.5,
  "renderers": 11
}
```

Treat this as “memory for the browser instance that owns this tab”, not “memory for this tab alone”.

## All Running Instances

In orchestrator mode:

```bash
curl http://localhost:9867/instances/metrics
```

This returns one metrics object per running instance — `instanceId`, `profileName`, `memoryMB` and `renderers` — which is the best API for comparing memory across a fleet.

## Dashboard Monitoring

The dashboard consumes monitoring snapshots from:

```bash
curl http://localhost:9867/api/events?memory=1
```

That stream includes:

- instance list
- tab list
- per-instance metrics when `memory=1`
- server metrics for the PinchTab process itself

The current SSE monitoring loop updates on a short interval, which is suitable for live dashboard views.

## Troubleshooting

### Memory Shows `0`

Likely causes:

- Chrome has not started yet
- the instance is stopped
- the browser context is not initialized

### Memory Looks Higher Than Expected

Remember that `memoryMB` includes:

- the browser process
- renderer processes
- GPU and utility children if present

This is usually closer to “what the OS sees” than to a narrow JavaScript heap figure.

### Numbers Do Not Match Activity Monitor Or Task Manager Exactly

Different tools report different memory definitions. PinchTab currently reports RSS-based totals for the Chrome process tree it owns.
