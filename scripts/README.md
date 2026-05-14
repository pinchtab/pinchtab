# Scripts

Development and CI scripts for PinchTab.

> **Tip:** Use `./dev` from the repo root for an interactive command picker, or `./dev <command>` to run directly.

## Quality

| Script | Purpose |
|--------|---------|
| `check.sh` | Go checks (format, vet, build, lint) |
| `check-dashboard.sh` | Dashboard checks (typecheck, eslint, prettier) |
| `check-npm.sh` | npm package checks (lint, format, typecheck, tests, pack validation) |
| `check-gosec.sh` | Security scan with gosec (reproduces CI security job) |
| `check-docs-json.sh` | Validate `docs/index.json` structure |
| `test.sh` | Go test runner with progress (unit, integration, system, or all) |
| `pre-commit` | Git pre-commit hook (format + lint) |

## Build & Run

| Script | Purpose |
|--------|---------|
| `build.sh` | Full build (dashboard + Go) without starting the server |
| `install.sh` | Full build + install to `~/.local/bin/pinchtab` (override with `PINCHTAB_INSTALL_DIR`); re-signs ad-hoc on macOS |
| `binary.sh` | Release-style stripped binary build into `dist/` for the current platform, or the full matrix with `all` |
| `build-dashboard.sh` | Generate TS types (tygo) + build React dashboard + copy to Go embed |
| `autosolver-realworld-smoke.sh` | Smoke-test real-world autosolver flow against an external detection page |
| `dev.sh` | Full build (dashboard + Go) and run |
| `dev-e2e.sh` | Lookup wrapper for `./dev e2e test`; delegates execution to the Go E2E runner |
| `docker-cloakbrowser-smoke.sh` | Docker CloakBrowser smoke using the dedicated local-only `tests/tools/docker/cloakbrowser-smoke.Dockerfile` image |
| `docker-smoke.sh` | Host Docker smoke helper invoked by the Go E2E runner |
| `npm-dev-binary.sh` | Build the canonical `./pinchtab-dev` binary for source-checkout npm-package testing |
| `run.sh` | Run the existing `./pinchtab` binary |

## Setup

| Script | Purpose |
|--------|---------|
| `doctor.sh` | Verify & setup dev environment (interactive — prompts before installing) |
| `install-hooks.sh` | Install git pre-commit hook |

## Testing

| Script | Purpose |
|--------|---------|
| `simulate-memory-load.sh` | Memory load testing |
| `simulate-ratelimit-leak.sh` | Rate limit leak testing |

## `./dev` shortcuts

- `./dev e2e smoke` runs the smoke tier through the Go e2e runner, including host Docker smoke checks.
- `PINCHTAB_DOCKER_SMOKE_RELEASE_IMAGE=<image> ./dev e2e smoke --filter release` skips the release-image build and runs the matching Docker smoke checks against the specified image tag.
- `./dev smoke cloakbrowser [image]` runs the manual Docker CloakBrowser smoke. By default it builds `pinchtab-cloakbrowser:test` from `tests/tools/docker/cloakbrowser-smoke.Dockerfile`, which installs the CloakBrowser binary into that local image. The smoke mounts `/data` and `/tmp` as tmpfs for Chromium profile and socket paths, serves `tests/e2e/fixtures`, checks a representative endpoint subset, and runs the Cloak-compatible API basic E2E scenario files in the API runner container attached to the Cloak container's network namespace. Set `SKIP_BUILD=1` to require a prebuilt image, or set `PINCHTAB_CLOAKBROWSER_E2E_SCENARIOS` to override the scenario list.
