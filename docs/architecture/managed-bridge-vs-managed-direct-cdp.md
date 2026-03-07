# Managed Bridge vs Managed Direct-CDP

This page compares two ways Pinchtab can manage a browser instance:

- `managed + bridge`
- `managed + direct-cdp`

Both are **managed** because Pinchtab owns the instance lifecycle.
The difference is where the browser control logic lives and how the server reaches Chrome.

## Short Version

```text
managed + bridge
  server -> bridge -> Chrome

managed + direct-cdp
  server -> Chrome
```

The bridge model adds one extra process and one extra hop.
The direct-CDP model removes that hop and keeps control in the main server.

## Chart 1: Runtime Shape

```text
Managed + bridge
  Pinchtab server
    └─ Pinchtab bridge child
         └─ Chrome
              └─ Tabs

Managed + direct-cdp
  Pinchtab server
    └─ Chrome
         └─ Tabs
```

## Managed + Bridge

### What it is

Pinchtab starts a child `pinchtab bridge` process for each managed instance.
That bridge owns one browser and exposes a single-instance HTTP API.
The main server routes instance and tab requests to that child.

### Communication Path

```text
agent -> server -> bridge -> Chrome
```

### Benefits

- strong per-instance isolation
- clearer process boundaries
- easier crash containment
- easier per-instance logs and health checks
- easier to reason about operationally as a worker model

### Costs

- one extra process per instance
- one extra HTTP hop before reaching Chrome
- more ports to allocate and monitor
- more startup overhead
- some configuration must be propagated to child runtimes

### Best fit

- multi-instance orchestration
- strong isolation between instances
- cases where instance failures should stay local
- systems that benefit from worker-style process supervision

## Managed + Direct-CDP

### What it is

Pinchtab starts Chrome itself and keeps the CDP session inside the main server process.
There is no bridge child and no extra per-instance HTTP server.

### Communication Path

```text
agent -> server -> Chrome
```

### Benefits

- fewer moving parts
- lower latency
- less process and port overhead
- simpler network model
- less duplicated HTTP handling

### Costs

- weaker process isolation by default
- more complexity inside the main server
- harder to contain instance-specific failures
- more shared memory and state inside one process
- main server becomes responsible for more lifecycle details directly

### Best fit

- low-overhead single-host deployments
- workloads where efficiency matters more than hard isolation
- environments where an extra worker process is unnecessary
- future architectures that want fewer internal hops

## Chart 2: Ownership And Transport

```text
managed + bridge
  ownership: pinchtab
  transport: http-bridge + cdp

managed + direct-cdp
  ownership: pinchtab
  transport: direct cdp
```

## Chart 3: Failure Boundary

```text
managed + bridge
  one instance crash
    -> bridge worker dies
    -> instance is affected
    -> server survives

managed + direct-cdp
  one instance failure
    -> handled inside server process
    -> isolation depends on server design
```

## Decision Frame

Use this rule:

- choose **managed + bridge** when isolation and operational clarity matter more
- choose **managed + direct-cdp** when simplicity of the runtime path and lower overhead matter more

Or even shorter:

```text
bridge      = better isolation
direct-cdp  = better efficiency
```

## Current Status

Today, the intended architecture is:

- `managed + bridge` for Pinchtab-launched instances
- `attached + direct-cdp` for externally managed browsers

`managed + direct-cdp` is a useful future model, but it is primarily an architectural option, not the default implementation.
