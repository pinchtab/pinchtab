# Strategies

`multiInstance.strategy` decides which routes PinchTab exposes and how shorthand requests behave.

Shorthand routes go to the earliest-started running instance of the requested (or default) browser target.
Requests that name an instance (`/instances/{id}/...`) go to that instance.

`multiInstance.allocationPolicy` is retired: config files that still set it load with an advisory, and the value is ignored.

## Strategy

Valid strategies in the current implementation:

- `always-on` - default
- `simple`
- `explicit`
- `simple-autorestart`
- `no-instance`

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
- shorthand routes proxy to the earliest-started running instance (of the default browser target) only if one already exists
- if nothing is running, shorthand routes return `503 no running instances — launch one from the Profiles tab` instead of launching a browser for you; `GET /tabs` returns an empty list

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
- exposes `GET /always-on/status` for current managed-instance state: `instanceId`, `restartCount`, `maxRestarts`,
  `lastCrash`, `lastStart`, and `status` (`running`, `restarting`, `crashed`, or `stopped`)

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
- exposes `GET /autorestart/status` for restart state (same shape as `/always-on/status`)

Both managed strategies take their limits from `multiInstance.restart`; `maxRestarts: -1` means unlimited.

Best fit:

- kiosk or appliance-style setups
- unattended local services
- environments where one browser should come back after a crash

### `no-instance`

`no-instance` runs PinchTab as a hub that does not launch any local Chrome processes. It only accepts remote bridges via `POST /instances/attach-bridge` and proxies shorthand requests to the first attached bridge.

Behavior:

- never launches a local instance
- registers the orchestrator API in no-launch mode
- shorthand routes (`/snapshot`, `/text`, `/navigate`, `/tabs`, etc.) proxy to the first attached remote bridge, or return `503 no remote instances connected — attach a bridge first` when none is attached
- `GET /tabs` returns an empty list when no bridge is attached, instead of erroring

Best fit:

- bridge-orchestrator deployments where browsers run on other hosts (Tailscale, remote workers)
- centralized routing tiers that should never spawn Chrome themselves
- environments where the local PinchTab process must remain a pure proxy

## Example Config

```json
{
  "multiInstance": {
    "strategy": "explicit",
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
    "strategy": "simple"
  }
}
```

Use this when you want shorthand routes to feel like a single local browser service.

### Explicit Orchestration

```json
{
  "multiInstance": {
    "strategy": "explicit"
  }
}
```

Use this when your client is instance-aware and you want to control lifecycle directly.

### Self-Healing Single Service

```json
{
  "multiInstance": {
    "strategy": "simple-autorestart",
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
no-instance         = pure proxy/hub for remote bridges, never launches Chrome
```
