# Strategies And Allocation

PinchTab has two separate multi-instance controls:

- `multiInstance.strategy`
- `multiInstance.allocationPolicy`

They solve different problems:

```text
strategy          = what routes PinchTab exposes and how shorthand requests behave
allocationPolicy  = which running instance gets picked when PinchTab must choose one
```

## Strategy

Valid strategies in the current implementation:

- `always-on` - default
- `simple`
- `explicit`
- `simple-autorestart`

### `simple`

Behavior:

- registers the full orchestrator API
- keeps shorthand routes such as `/snapshot`, `/text`, `/navigate`, and `/tabs`
- if a shorthand request arrives and no instance is running, PinchTab auto-launches one managed instance and waits for it to become healthy

Best fit:

- local development
- single-user automation
- “just make the browser service available” setups

### `explicit`

`explicit` also exposes the orchestrator API and shorthand routes, but it does not auto-launch on shorthand requests.

Behavior:

- you start instances explicitly with `/instances/start`, `/instances/launch`, or `/profiles/{id}/start`
- shorthand routes proxy to the first running instance only if one already exists
- if nothing is running, shorthand routes return an error instead of launching a browser for you

Best fit:

- controlled multi-instance environments
- agents that should name instances deliberately
- deployments where hidden auto-launch would be surprising

### `always-on`

`always-on` behaves like a managed single-instance service that should stay up for the full lifetime of the PinchTab process.

Behavior:

- launches one managed instance when the strategy starts
- exposes the same shorthand routes as `simple`
- watches that managed instance and keeps restarting it after unexpected exits until the configured restart limit is reached
- exposes `GET /always-on/status` for current managed-instance state

Best fit:

- daemon-style local services
- agent hosts that expect one default browser to always be present
- setups where startup availability matters, but you still want a bounded failure policy

### `simple-autorestart`

`simple-autorestart` behaves like a managed single-instance service with recovery.

Behavior:

- launches one managed instance when the strategy starts
- exposes the same shorthand routes as `simple`
- watches that managed instance and tries to restart it after unexpected exits under the configured restart policy
- exposes `GET /autorestart/status` for restart state

Best fit:

- kiosk or appliance-style setups
- unattended local services
- environments where one browser should come back after a crash

## Allocation Policy

Valid policies in the current implementation:

- `fcfs`
- `round_robin`
- `random`

Allocation policy matters only when PinchTab has multiple eligible running instances and needs to choose one. If your request already targets `/instances/{id}/...`, no allocation policy is involved for that request.

### `fcfs`

First running candidate wins.

Best fit:

- predictable behavior
- simplest operational model
- “always use the earliest running instance” workflows

### `round_robin`

Candidates are selected in rotation.

Best fit:

- light balancing across a stable pool
- repeated shorthand-style traffic where you want even distribution over time

### `random`

PinchTab picks a random eligible candidate.

Best fit:

- looser balancing
- experiments where deterministic ordering is not important

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

## Recommended Defaults

### Always-On Service

```json
{
  "multiInstance": {
    "strategy": "always-on",
    "allocationPolicy": "fcfs",
    "restart": {
      "maxRestarts": 20,
      "initBackoffSec": 2,
      "maxBackoffSec": 60,
      "stableAfterSec": 300
    }
  }
}
```

Use this when the default managed browser should be started immediately and kept available with a bounded restart policy.

### Simple Local Service

```json
{
  "multiInstance": {
    "strategy": "simple",
    "allocationPolicy": "fcfs"
  }
}
```

Use this when you want shorthand routes to feel like a single local browser service.

### Explicit Orchestration

```json
{
  "multiInstance": {
    "strategy": "explicit",
    "allocationPolicy": "round_robin"
  }
}
```

Use this when your client is instance-aware and you want to control lifecycle directly.

### Self-Healing Single Service

```json
{
  "multiInstance": {
    "strategy": "simple-autorestart",
    "allocationPolicy": "fcfs",
    "restart": {
      "maxRestarts": 3,
      "initBackoffSec": 2,
      "maxBackoffSec": 60,
      "stableAfterSec": 300
    }
  }
}
```

Use this when one managed browser should stay available and recover after crashes.

## Decision Rule

```text
always-on           = default, launched at startup, restarted under restart policy
simple              = on-demand shorthand auto-launch
explicit            = most control, no shorthand auto-launch
simple-autorestart  = one managed browser with crash recovery

fcfs                = deterministic
round_robin         = balanced rotation
random              = loose distribution
```
