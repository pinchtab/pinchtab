# Design: auto-recover from renderer-OOM degradation (follow-up)

Surfaced by `tests/soak/burst.sh`: under memory pressure Chrome OOM-kills a *renderer* but
the main browser process stays alive, so the instance is **functionally degraded** (new tabs
fail) yet the control plane never recovers it. In the burst run, 7/7 OOMs required an external
`pinchtab server restart`. (The 60s new-tab hang itself is already fixed — new-tab creation
now fails fast with a `503` naming OOM.)

## Root cause
The orchestrator emits crash events only at startup and process-exit, never continuously at
runtime:
- startup probe → `instance.error` (`internal/orchestrator/health.go`)
- process exit → `instance.stopped` (`finalizeInstanceExit`)

A running-but-degraded child (process alive, renderer dead) matches neither, so the autorestart
strategy — which acts only on `instance.error`/`instance.stopped`
(`internal/strategy/autorestart/restart.go`) — never triggers. Instances are child processes
(`orchestrator/runner.go`), so the degraded signal must cross the process boundary via the
child's `/health`.

## Plan (Option A — signal degraded → reuse the existing restart path)
1. **Child reports degraded** (low risk): a consecutive new-tab-OOM-failure counter on
   `Handlers`, incremented on the new-tab OOM 503 path in `runNavigate` (and the `CreateTab`
   OOM path), reset on any successful tab creation; exposed as `degraded` in the instance
   `/health`. Useful on its own (health reflects *functional capability*, not just liveness).
2. **Orchestrator runtime monitor** (sensitive): a goroutine that polls each running managed
   instance's `/health`; if `degraded` for K consecutive polls, emits `instance.error`.
   Guard against restart storms: K-consecutive requirement + the existing `MaxRestarts`/backoff
   in `handleCrash`; only the managed instance; skip while already restarting.
3. **Autorestart**: no change — `instance.error` already routes to `handleCrash` → restart.

## Verification
Reuse `burst.sh` but remove its own `server restart`; assert the instance self-recovers
(instance id changes once per OOM, `degraded` clears) without external intervention, and does
NOT restart-storm under steady light load.
