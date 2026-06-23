# Scheduler

This page describes how PinchTab's optional scheduler works internally.

## Scope

The scheduler is an opt-in dispatch layer for queued browser tasks. It sits in front of the existing tab action execution path and adds:

- queue admission limits
- per-agent fairness
- bounded concurrency
- cooperative cancellation
- result retention with TTL

It does not replace the direct action endpoints.

## Runtime Placement

In dashboard mode, the scheduler is created only when `scheduler.enabled` is true. It is registered directly on the main mux and exposes:

- `POST /tasks`
- `GET /tasks`
- `GET /tasks/{id}`
- `POST /tasks/{id}/cancel`
- `GET /scheduler/stats`
- `POST /tasks/batch`

## High-Level Flow

```text
client
  -> POST /tasks
  -> scheduler admission
  -> in-memory task queue
  -> worker dispatch
  -> resolve tab -> instance port
  -> POST /tabs/{tabId}/action on the owning instance
  -> result store
```

## Core Components

### Scheduler

The scheduler owns:

- config
- task queue
- result store
- task lookup for live tasks
- cancellation map
- worker lifecycle

### TaskQueue

The queue is:

- in-memory
- global-limit aware
- per-agent-limit aware
- priority ordered within an agent
- fairness aware across agents

Within an agent queue:

- lower `priority` value wins
- equal priority falls back to FIFO by `CreatedAt`

Across agents:

- the agent with the fewest in-flight tasks is chosen first

### ResultStore

The result store keeps snapshots of tasks and evicts terminal tasks after the configured TTL.

### ManagerResolver

The resolver maps a `tabId` to the owning instance port through `instance.Manager.FindInstanceByTabID`.

This is how the scheduler knows where to forward execution.

## Dispatch Lifecycle

A task moves through these internal states:

```text
queued -> assigned -> running -> done
                           -> failed
queued -> cancelled
queued -> rejected
```

Implementation details:

- `assigned` is set just before worker execution starts
- `running` is set immediately before the proxied action request
- `done`, `failed`, and `cancelled` are terminal
- rejected tasks are stored as terminal results even though they never enter active execution

## Admission And Limits

Admission checks include:

- global queue size
- per-agent queue size

Execution checks include:

- max global in-flight count
- max per-agent in-flight count

These are enforced in the queue and worker dispatch path rather than by external infrastructure.

## Cancellation Model

Each running task gets its own `context.WithDeadline(...)`.

The scheduler stores the corresponding cancel function so that:

- `POST /tasks/{id}/cancel` can stop a running task
- shutdown can cancel in-flight work
- deadlines propagate naturally to the proxied request

## Deadline Handling

Two deadline paths exist:

- queued task expiry: a background reaper scans queued tasks every second and marks expired ones as failed
- running task deadline: the per-task context deadline is enforced by the HTTP request to the executor

Queued expiry currently records:

```text
deadline exceeded while queued
```

## Execution Contract

The scheduler does not invent a separate execution protocol. It converts each task into the normal action request body:

```json
{
  "kind": "<action>",
  "ref": "<ref>",
  "...params": "..."
}
```

and forwards it to:

```text
POST /tabs/{tabId}/action
```

That keeps the immediate path and scheduled path aligned.

## Error Model

Common failure sources:

- admission rejection because the queue is full
- tab-to-instance resolution failure
- executor HTTP failure
- browser-side action failure
- deadline expiry

Scheduler task snapshots keep the final `error` string for these cases.

## Result Retention

Terminal task snapshots are stored in memory and evicted after the configured TTL. This makes `GET /tasks/{id}` and `GET /tasks` useful for short-lived inspection, not for long-term audit storage.

## Design Tradeoffs

The current scheduler favors:

- simple in-memory operation
- low dependency count
- reuse of the existing action executor

That means it does not currently provide:

- persistent queue storage
- durable recovery after process restart
- a separate task execution DSL

---

## Phase 2 -- Observability Layer

### Metrics

The scheduler maintains global and per-agent counters via the `Metrics` struct. Global counters use `atomic.Uint64` for lock-free increments. Per-agent counters use a sharded mutex pattern (`agentMetricEntry`) to avoid global lock contention.

Tracked counters:

- `TasksSubmitted`, `TasksCompleted`, `TasksFailed`
- `TasksCancelled`, `TasksRejected`, `TasksExpired`
- `DispatchTotal` and cumulative `DispatchLatency` (for average dispatch latency calculation)

`Metrics.Snapshot()` returns a `MetricsSnapshot` -- a point-in-time serializable copy that includes per-agent breakdowns.

### Stats Endpoint

`GET /scheduler/stats` exposes three sections:

- **queue** -- current queued/inflight counts from `QueueStats()`
- **metrics** -- snapshot from `Metrics.Snapshot()`
- **config** -- current scheduler configuration values

### Lifecycle Logging

Key scheduler events are logged via `slog`:

- task submission, dispatch, completion, failure, cancellation
- queue admission rejections
- config reload events
- webhook delivery outcomes

### Webhook Delivery

When a task has a `callbackUrl` and reaches a terminal state, the scheduler sends a POST with the task snapshot as JSON. This runs asynchronously in a goroutine from `finishTask()`.

Security constraints:

- only `http` and `https` schemes are accepted (SSRF mitigation)
- a dedicated `http.Client` with a 10-second timeout prevents hanging connections
- delivery failures are logged but do not affect task state

Custom headers are sent: `X-PinchTab-Event: task.completed` and `X-PinchTab-Task-ID`.

---

## Phase 3 -- Hardening Layer

### Batch Submission

`POST /tasks/batch` accepts an array of task definitions (up to 50) sharing a single `agentId` and optional `callbackUrl`. Each task is submitted individually through `Submit()`, so queue admission limits apply per-task.

The batch endpoint supports partial failure: if some tasks are rejected (queue full), the accepted tasks are still submitted and the response includes per-task status.

### Config Hot-Reload

`ReloadConfig(cfg Config)` updates tuning knobs at runtime:

- queue limits via `queue.SetLimits(maxQueue, maxPerAgent)`
- inflight limits via `cfgMu`-protected config fields
- result TTL via `results.SetTTL(ttl)`

Zero values in the reload config are ignored, preserving the existing setting.

`ConfigWatcher` is a background goroutine that periodically calls a user-supplied `loadFn() (Config, error)` and applies changes via `ReloadConfig`. It can be started and stopped cleanly.

### SetLimits and SetTTL

The queue and result store expose safe runtime mutators:

- `TaskQueue.SetLimits(maxQueue, maxPerAgent)` -- atomically updates admission thresholds
- `ResultStore.SetTTL(ttl)` -- updates the eviction window for terminal task snapshots
