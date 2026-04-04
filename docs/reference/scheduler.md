# Scheduler And Tasks

The scheduler is an optional in-memory task queue for multi-agent coordination. It accepts tasks over `/tasks`, applies admission and fairness rules, then dispatches work to the same tab action executor used by the immediate browsing routes.

It does not replace the normal direct path. Routes such as `POST /tabs/{id}/action` still work independently.

There is no CLI scheduler command today.

## Enable The Scheduler

The scheduler is off by default. Dashboard mode registers the task routes only when `scheduler.enabled` is true.

```json
{
  "scheduler": {
    "enabled": true
  }
}
```

## Scheduler Config

```json
{
  "scheduler": {
    "enabled": true,
    "strategy": "fair-fifo",
    "maxQueueSize": 1000,
    "maxPerAgent": 100,
    "maxInflight": 20,
    "maxPerAgentInflight": 10,
    "resultTTLSec": 300,
    "workerCount": 4
  }
}
```

| Field | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | enables task routes in dashboard mode |
| `strategy` | `fair-fifo` | scheduler strategy label |
| `maxQueueSize` | `1000` | global queued task limit |
| `maxPerAgent` | `100` | queued task limit per agent |
| `maxInflight` | `20` | max concurrently executing tasks overall |
| `maxPerAgentInflight` | `10` | max concurrently executing tasks per agent |
| `resultTTLSec` | `300` | retention time for terminal task snapshots |
| `workerCount` | `4` | number of worker goroutines |

## Task Object

Tasks are scheduler-owned records with these main fields:

| Field | Meaning |
| --- | --- |
| `taskId` | generated task ID |
| `agentId` | submitting agent identifier |
| `action` | action kind to run |
| `tabId` | target tab ID |
| `ref` | optional element ref |
| `params` | optional action-specific request fields |
| `priority` | lower number means higher priority |
| `state` | current task state |
| `deadline` | execution deadline |
| `createdAt` | submission time |
| `startedAt` | first execution timestamp |
| `completedAt` | terminal timestamp |
| `latencyMs` | elapsed time from start to completion |
| `result` | executor response payload |
| `error` | terminal error message |
| `position` | queue position at submission time |
| `callbackUrl` | optional webhook URL for terminal state notification |

Task IDs are currently generated as `tsk_XXXXXXXX`, but callers should still treat them as opaque IDs.

## Submit A Task

```bash
curl -X POST http://localhost:9867/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "agentId": "agent-crawl-01",
    "action": "click",
    "tabId": "8f9c7d4e1234567890abcdef12345678",
    "ref": "e14",
    "priority": 5,
    "deadline": "2026-03-08T12:05:00Z"
  }'
# Response
{
  "taskId": "tsk_a1b2c3d4",
  "state": "queued",
  "position": 1,
  "createdAt": "2026-03-08T12:00:01Z"
}
```

This endpoint returns `202 Accepted` on successful queue submission.

Request fields:

| Field | Required | Notes |
| --- | --- | --- |
| `agentId` | yes | validated at request time |
| `action` | yes | becomes the executor `kind` |
| `tabId` | practically yes | required by the execution path |
| `ref` | no | top-level element ref for element-targeted actions |
| `params` | no | action-specific fields merged into the executor request body |
| `priority` | no | lower number means higher priority |
| `deadline` | no | RFC3339 timestamp; defaults to `now + 60s` |
| `callbackUrl` | no | webhook URL; receives POST with task snapshot on terminal state |

Important:

- request validation enforces only `agentId` and `action`
- missing `tabId` is rejected later during execution with `tabId is required for task execution`
- past deadlines are rejected at submission time
- `agentId` is also forwarded to the executor as `X-Agent-Id`, so the resulting browser action is attributed to the same agent in `/api/activity` and the dashboard Agents view

## Queue Full Response

If admission fails because the global queue or an agent queue is full, the scheduler returns `429 Too Many Requests`.

```bash
curl -X POST http://localhost:9867/tasks \
  -H "Content-Type: application/json" \
  -d '{"agentId":"agent-crawl-01","action":"click","tabId":"8f9c7d4e1234567890abcdef12345678"}'
# Response
{
  "code": "queue_full",
  "error": "rejected: global queue full",
  "retryable": true,
  "details": {
    "agentId": "agent-crawl-01",
    "queued": 1000,
    "maxQueue": 1000,
    "maxPerAgent": 100
  }
}
```

## List Tasks

`GET /tasks` returns the scheduler's in-memory task snapshots, including queued, running, and recently completed tasks that are still within the TTL window.

```bash
curl http://localhost:9867/tasks
# Response
{
  "tasks": [
    {
      "taskId": "tsk_a1b2c3d4",
      "state": "done",
      "agentId": "agent-crawl-01",
      "action": "click",
      "latencyMs": 842
    }
  ],
  "count": 1
}
```

Supported query filters:

- `agentId`
- `state`

Example:

```bash
curl 'http://localhost:9867/tasks?agentId=agent-crawl-01&state=done,failed'
```

## Get One Task

```bash
curl http://localhost:9867/tasks/tsk_a1b2c3d4
# Response
{
  "taskId": "tsk_a1b2c3d4",
  "agentId": "agent-crawl-01",
  "action": "click",
  "tabId": "8f9c7d4e1234567890abcdef12345678",
  "ref": "e14",
  "priority": 5,
  "state": "done",
  "createdAt": "2026-03-08T12:00:01Z",
  "startedAt": "2026-03-08T12:00:01Z",
  "completedAt": "2026-03-08T12:00:02Z",
  "latencyMs": 842,
  "result": {
    "success": true
  }
}
```

If the task is not found, the scheduler returns:

```json
{
  "code": "not_found",
  "error": "task not found"
}
```

## Cancel A Task

```bash
curl -X POST http://localhost:9867/tasks/tsk_a1b2c3d4/cancel
# Response
{
  "status": "cancelled",
  "taskId": "tsk_a1b2c3d4"
}
```

Behavior:

- queued tasks are removed from the queue
- running tasks have their execution context cancelled
- terminal tasks return `409 Conflict`

## Task States

Implemented states:

- `queued`
- `assigned`
- `running`
- `done`
- `failed`
- `cancelled`
- `rejected`

Terminal states:

- `done`
- `failed`
- `cancelled`
- `rejected`

## How Tasks Execute

The scheduler forwards each task to the normal tab action endpoint:

```text
POST /tabs/{tabId}/action
```

It builds the action body like this:

```json
{
  "kind": "<action>",
  "ref": "<ref>",
  "...params": "..."
}
```

That means:

- `action` becomes `kind`
- top-level `ref` is forwarded when present
- every key in `params` is merged into the top-level action body
- `agentId` is propagated as `X-Agent-Id`

Example:

```bash
curl -X POST http://localhost:9867/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "agentId": "my-agent",
    "action": "type",
    "tabId": "8f9c7d4e1234567890abcdef12345678",
    "ref": "e12",
    "params": {
      "text": "Alan Turing"
    }
  }'
```

In practice, task payloads should use the same action fields that the immediate `/tabs/{id}/action` route expects.

## Fairness, Deadlines, And Retention

- within one agent queue, lower `priority` values run first
- equal-priority tasks for the same agent fall back to FIFO order
- across agents, the scheduler prefers the agent with the fewest in-flight tasks
- if a queued task passes its deadline before execution starts, it is marked failed with `deadline exceeded while queued`
- terminal task snapshots are retained in memory for `resultTTLSec`

---

## Phase 2 -- Observability

### Scheduler Stats

`GET /scheduler/stats` returns a snapshot of queue state, runtime metrics, and configuration.

```bash
curl http://localhost:9867/scheduler/stats
# Response
{
  "queue": {
    "totalQueued": 5,
    "totalInflight": 2,
    "agentCounts": {
      "agent-crawl-01": 3,
      "agent-scrape-02": 2
    }
  },
  "metrics": {
    "tasksSubmitted": 42,
    "tasksCompleted": 35,
    "tasksFailed": 3,
    "tasksCancelled": 2,
    "tasksRejected": 1,
    "tasksExpired": 1,
    "dispatchCount": 38,
    "avgDispatchLatencyMs": 12.5,
    "agents": {
      "agent-crawl-01": {
        "submitted": 25,
        "completed": 22,
        "failed": 2,
        "cancelled": 1,
        "rejected": 0
      }
    }
  },
  "config": {
    "strategy": "fair-fifo",
    "maxQueueSize": 1000,
    "maxPerAgent": 100,
    "maxInflight": 20,
    "maxPerAgentFlight": 10,
    "workerCount": 4,
    "resultTTL": "5m0s"
  }
}
```

#### Metrics Fields

| Field | Type | Meaning |
| --- | --- | --- |
| `tasksSubmitted` | uint64 | total tasks accepted since startup |
| `tasksCompleted` | uint64 | tasks that finished successfully |
| `tasksFailed` | uint64 | tasks that finished with an error |
| `tasksCancelled` | uint64 | tasks cancelled via `POST /tasks/{id}/cancel` |
| `tasksRejected` | uint64 | tasks rejected at admission (queue full) |
| `tasksExpired` | uint64 | queued tasks that exceeded their deadline |
| `dispatchCount` | uint64 | number of tasks dispatched to workers |
| `avgDispatchLatencyMs` | float64 | average time from queue entry to dispatch start |
| `agents` | object | per-agent breakdown (submitted, completed, failed, cancelled, rejected) |

### Webhook Callbacks

Tasks can include a `callbackUrl` field. When the task reaches a terminal state (`done`, `failed`, or `cancelled`), the scheduler delivers a POST with the task snapshot to that URL.

```bash
curl -X POST http://localhost:9867/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "agentId": "my-agent",
    "action": "click",
    "tabId": "8f9c7d4e1234567890abcdef12345678",
    "callbackUrl": "https://pinchtab.com/hooks/task-done"
  }'
```

Webhook behavior:

- delivery is best-effort: failures are logged but do not affect task state
- only `http` and `https` schemes are allowed (SSRF protection)
- a dedicated HTTP client with a 10-second timeout is used
- custom headers are sent: `X-PinchTab-Event: task.completed` and `X-PinchTab-Task-ID: <taskId>`

The `callbackUrl` field is stored on the task and returned in `GET /tasks/{id}`.

---

## Phase 3 -- Hardening

### Batch Task Submission

`POST /tasks/batch` submits multiple tasks in a single request. All tasks in the batch share the same `agentId` and optional `callbackUrl`.

```bash
curl -X POST http://localhost:9867/tasks/batch \
  -H "Content-Type: application/json" \
  -d '{
    "agentId": "agent-crawl-01",
    "callbackUrl": "https://pinchtab.com/hooks/batch",
    "tasks": [
      { "action": "click", "tabId": "TAB_ID", "params": { "selector": "#btn" } },
      { "action": "scroll", "tabId": "TAB_ID", "params": { "scrollY": 400 } },
      { "action": "hover", "tabId": "TAB_ID", "params": { "selector": "h1" }, "priority": 1 }
    ]
  }'
# Response (202 Accepted)
{
  "tasks": [
    { "taskId": "tsk_aaaa1111", "state": "queued", "position": 1 },
    { "taskId": "tsk_bbbb2222", "state": "queued", "position": 2 },
    { "taskId": "tsk_cccc3333", "state": "queued", "position": 3 }
  ],
  "submitted": 3
}
```

#### Batch Request Fields

| Field | Required | Notes |
| --- | --- | --- |
| `agentId` | yes | shared across all tasks in the batch |
| `callbackUrl` | no | webhook URL applied to every task |
| `tasks` | yes | array of task definitions (1–50) |

Each task definition supports the same fields as a single task submit (`action`, `tabId`, `ref`, `params`, `priority`, `deadline`) except `agentId` and `callbackUrl` which are inherited from the batch.

#### Batch Validation

| Condition | Response |
| --- | --- |
| missing `agentId` | `400 Bad Request` |
| empty `tasks` array | `400 Bad Request` |
| more than 50 tasks | `400 Bad Request` with `batch_too_large` code |
| invalid JSON body | `400 Bad Request` |

Partial failure: if some tasks are rejected by admission (queue full), the accepted tasks are still submitted. The response includes each task's status individually.

### Config Hot-Reload

`ReloadConfig(cfg)` updates queue limits, inflight limits, and result TTL at runtime without restarting the scheduler.

Reloadable fields:

| Field | What Changes |
| --- | --- |
| `maxQueueSize`, `maxPerAgent` | queue admission limits via `SetLimits()` |
| `maxInflight`, `maxPerAgentFlight` | concurrency limits (protected by `cfgMu`) |
| `resultTTL` | result store eviction window via `SetTTL()` |

Zero values are ignored (the existing setting is preserved).

#### ConfigWatcher

`ConfigWatcher` runs a background goroutine that periodically re-reads the config and calls `ReloadConfig`. Create with:

```go
cw := scheduler.NewConfigWatcher(30*time.Second, loadFn, sched)
cw.Start()
defer cw.Stop()
```

`loadFn` is a `func() (Config, error)` that reads the current config from disk or environment.
