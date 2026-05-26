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
| `docker-smoke.sh` | Host Docker smoke helper invoked by the Go E2E runner |
| `npm-dev-binary.sh` | Build the canonical `./pinchtab-dev` binary for source-checkout npm-package testing |
| `run.sh` | Run the existing `./pinchtab` binary |

## Setup

| Script | Purpose |
|--------|---------|
| `doctor.sh` | Verify & setup dev environment (interactive — prompts before installing) |
| `install-hooks.sh` | Install git pre-commit hook |
| `install-skills.sh` | Symlink repo skills into `./.claude/skills` by default; set `CLAUDE_SKILLS_DIR=~/.claude/skills` for a global install |

## Testing

| Script | Purpose |
|--------|---------|
| `simulate-memory-load.sh` | Memory load testing |
| `simulate-ratelimit-leak.sh` | Rate limit leak testing |

## `./dev` shortcuts

- `./dev smoke` runs Docker smoke harnesses: browser parity, CDP attach, and live detection. The host shell remains the harness, but PinchTab and the browser under test run in containers.
- `./dev e2e [suite]` runs E2E suites through the Go runner. The default provider is Chrome; pass `--browser=cloak` to run against CloakBrowser. The runner builds `pinchtab-cloakbrowser:test` from `tests/tools/docker/cloakbrowser-smoke.Dockerfile` unless `SKIP_BUILD=1` is set.
- `PINCHTAB_DOCKER_SMOKE_RELEASE_IMAGE=<image> ./dev e2e smoke --filter release` skips the release-image build and runs the matching Docker smoke checks against the specified image tag.
- `./dev smoke cloakbrowser [--browser=chrome|cloak|all] [--multi-target|--profile-persistence|--profile-lock-recovery]` runs the specialized Chrome/CloakBrowser parity smoke via `scripts/docker-browser-parity-smoke.sh`. It builds the provider images from `tests/tools/docker/chrome-smoke.Dockerfile` and `tests/tools/docker/cloakbrowser-smoke.Dockerfile`; set `SKIP_BUILD=1` to require prebuilt images. The Cloak leg also honors `PINCHTAB_CLOAKBROWSER_E2E_SCENARIOS` for the scenario list.
