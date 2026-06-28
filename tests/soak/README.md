# PinchTab browser soak / stability lane

Docker-based stress tests for browser **stability, responsiveness, and recoverability**
under sustained load. Not a unit test — requires Docker, runs for minutes to an hour, and
is meant as a manual / nightly check. Drives the `chrome` provider (Debian Chromium) by
default.

## Quick start

```bash
# 1. Provision a container (builds the binary + image, starts a guards-down server)
tests/soak/setup.sh

# 2a. Stability soak — reuse ONE browser under mixed load for an hour
tests/soak/soak.sh 3600

# 2b. OR find the breaking point — unbounded tabs under a tight cap
MEM_CAP=1g SHM=256m MAX_TABS=200 tests/soak/setup.sh
tests/soak/burst.sh 1200

# 3. Tear down
docker rm -f pinchtab-soak
```

Results (CSV + summary) land in `tests/soak/results/` (git-ignored).

## What each measures

- **soak.sh** — reuses the *same* long-lived browser across cycles of randomized light /
  heavy / fixture loads, grows & churns tabs, periodically idles a few minutes then
  re-probes, and on unresponsiveness restarts + records recovery. Watches for responsiveness
  drift, memory growth/leaks, and crashes over time.
- **burst.sh** — opens a fresh heavy tab every ~1s with no eviction relief under a tight
  memory cap, to drive the browser to its breaking point (renderer OOM) and exercise
  recovery. Records the first breaking point (tabs/memory/OOM) and recovery success.

## Tuning (env vars on setup.sh)

| Var | Default | Use |
|-----|---------|-----|
| `MEM_CAP` | `2g` | lower (e.g. `1g`) to reach a breaking point sooner |
| `SHM` | `1g` | Chrome shared memory; lower (`256m`) stresses harder |
| `MAX_TABS` | `20` | raise (e.g. `200`) so tabs grow unbounded for burst.sh |
| `HEAVY_MB` | `5` | size of the generated `heavy.html` |
| `CPUS` | `2` | CPU limit |

## Reference baseline (what "healthy" looks like)

From a 1-hour soak at `maxTabs=20`, 2 GB cap, Chromium 149, arm64:
- responsiveness flat at **p50 ~11 ms** (no drift, even after multi-minute idles)
- memory self-bounds (LRU eviction at `maxTabs` caps tabs → caps memory); no leak
- **zero** unresponsive events / failed navs over 120 cycles

Breaking point only appears when `maxTabs` is raised past available RAM: ~**10 heavy tabs
per GB** → Chrome renderer OOM. The control plane + existing tabs stay responsive (~16 ms);
only *new-tab creation* fails. Recovery via `pinchtab server restart` is reliable (~6 s).
See the in-tree design note for the auto-recovery follow-up.
