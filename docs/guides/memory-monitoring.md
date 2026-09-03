# Memory Monitoring

PinchTab exposes memory information for the Chrome processes it launches. Each instance reports two measurements side by side: the OS view of its process tree, and the page counters Chrome itself reports for the tabs that instance tracks.

## What PinchTab Measures

For the process tree, PinchTab walks the running instance's Chrome processes:

1. find the main browser PID
2. enumerate child processes
3. sum RSS memory across the browser and its children
4. count renderer processes

For the pages, PinchTab asks every tab the instance tracks for its own `Performance.getMetrics` reading over CDP and sums the answers.

## Memory Fields

| Field | Meaning |
| --- | --- |
| `memoryMB` | Real RSS memory across the browser process tree |
| `renderers` | Number of renderer processes in the browser process tree |
| `page.targets` | Tabs whose reading is included in the `page` sums |
| `page.jsHeapUsedMB` / `page.jsHeapTotalMB` | JavaScript heap used and reserved, summed over those tabs |
| `page.documents` / `page.frames` / `page.nodes` / `page.jsEventListeners` | DOM counters summed over those tabs |
| `unreadableTargets` | Tabs that did not answer within the read timeout; they contribute nothing |

Every field is measured; none is derived from another field in the payload.

## Aggregation Rule

- **Which targets contribute:** every tab the instance tracks with a live context. Each is read separately with `Performance.getMetrics`, and the `page` block is the sum of the readings that arrived.
- **Scope:** the instance. `memoryMB` is the whole process tree, which also holds the GPU and utility processes and the shared browser process; `page` covers only the tabs. The two describe different populations and are never combined or compared arithmetically.
- **Absent versus unreadable:** `page` is omitted when no tab contributed. `unreadableTargets` says how many tabs were asked and did not answer (closed or crashed mid-collection, or a read timeout). No tab is ever reported as `0` heap or `0` nodes because it could not be read: `{"unreadableTargets":0}` with no `page` means no tabs, `{"unreadableTargets":2}` with no `page` means two tabs that would not answer.
- **Cost (measured, five tabs open):** about 1 ms per tab read and about 30 ms for the process-tree walk, so a poll is a few tens of milliseconds per instance and grows by roughly a millisecond per open tab. The dashboard's **Memory metrics** toggle does not gate the CDP reads specifically: it gates whether the dashboard polls every running instance's `/metrics` on each monitoring tick at all. `GET /metrics` on an instance always collects both.

Important limitation:

- `GET /tabs/{id}/metrics` returns the owning browser instance's aggregate, including the `page` sum over all its tabs, not isolated per-tab figures

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
    "renderers": 11,
    "page": {
      "targets": 3,
      "jsHeapUsedMB": 41.2,
      "jsHeapTotalMB": 64.0,
      "documents": 4,
      "frames": 5,
      "nodes": 9120,
      "jsEventListeners": 212
    },
    "unreadableTargets": 0
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
  "renderers": 11,
  "page": { "targets": 3, "jsHeapUsedMB": 41.2, "jsHeapTotalMB": 64.0, "documents": 4, "frames": 5, "nodes": 9120, "jsEventListeners": 212 },
  "unreadableTargets": 0
}
```

Treat this as “memory for the browser instance that owns this tab”, not “memory for this tab alone”: the `page` block is summed over every tab of that instance.

## All Running Instances

In orchestrator mode:

```bash
curl http://localhost:9867/instances/metrics
```

This returns one metrics object per running instance — `instanceId`, `profileName`, `memoryMB`, `renderers`, `page` and `unreadableTargets` — which is the best API for comparing memory across a fleet.

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
