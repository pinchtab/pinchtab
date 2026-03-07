# Expert Guide: Multi-Instance Strategies And Allocation

This guide covers advanced server behavior for users running more than the default simple local setup.

## Two Separate Concepts

Pinchtab has two knobs for advanced multi-instance behavior:

- `multiInstance.strategy`
- `multiInstance.allocationPolicy`

These are different.

Mental model:

```text
strategy          = what routing model the server exposes
allocationPolicy  = which running instance gets selected
```

## Strategy

Valid strategies on this branch:

- `simple`
- `explicit`
- `simple-autorestart`

### `simple`

`simple` is the default.

It behaves like:
- shorthand routes proxy to the first running instance
- if needed, the strategy may ensure one instance exists
- good default for single-user, single-active-instance workflows

Best fit:
- local development
- “just give me one browser service”
- simplest replacement for an embedded browser runtime

### `explicit`

`explicit` exposes the orchestrator model directly.

It is best when you want to think in terms of:
- profiles
- instances
- tab IDs
- explicit routing

Best fit:
- systems that manage several instances deliberately
- users who want predictable, explicit control over which instance handles work
- environments where shorthand auto-routing is too implicit

### `simple-autorestart`

`simple-autorestart` behaves like `simple`, but actively tries to keep a managed instance alive.

Best fit:
- kiosk-style or always-on setups
- local browser service behavior where one managed instance should come back after a crash
- unattended environments

## Allocation Policy

Valid allocation policies on this branch:

- `fcfs`
- `round_robin`
- `random`

### `fcfs`

First suitable instance wins.

Best fit:
- predictable behavior
- simple deployments
- minimal surprise

### `round_robin`

Requests rotate across eligible instances.

Best fit:
- distributing work across a stable set of running instances
- light balancing without randomness

### `random`

Requests pick a random eligible instance.

Best fit:
- spreading work without a fixed order
- experiments or looser balancing behavior

## Example Config

```json
{
  "multiInstance": {
    "strategy": "explicit",
    "allocationPolicy": "round_robin",
    "instancePortStart": 9868,
    "instancePortEnd": 9968
  }
}
```

## Recommended Usage Patterns

### Primary User Path

For the normal “local browser service” workflow:

```json
{
  "multiInstance": {
    "strategy": "simple",
    "allocationPolicy": "fcfs"
  }
}
```

This matches:
- install Pinchtab
- run `pinchtab`
- point the client to `http://localhost:9867`

### Explicit Multi-Instance Control

Use:

```json
{
  "multiInstance": {
    "strategy": "explicit",
    "allocationPolicy": "round_robin"
  }
}
```

when you want:
- clear instance boundaries
- explicit instance API usage
- predictable multi-instance orchestration

### Self-Healing Single-Service Setup

Use:

```json
{
  "multiInstance": {
    "strategy": "simple-autorestart",
    "allocationPolicy": "fcfs"
  }
}
```

when you want one logical browser service that tries to recover automatically.

## Decision Rule

Use this rule:

```text
simple              = easiest default
explicit            = most control
simple-autorestart  = single-service resilience

fcfs                = most predictable
round_robin         = balanced rotation
random              = loose distribution
```

## Important Boundary

These settings are expert-level because they change control-plane behavior.

They are not required for the primary Pinchtab experience.
Most users should stay on:

- `strategy = simple`
- `allocationPolicy = fcfs`
