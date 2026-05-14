# E2E Test Suite

End-to-end tests for PinchTab that exercise the full stack including browser automation.

## Quick Start

### With Docker (recommended)

```bash
./dev e2e          # Run the extended suite quietly by default
./dev e2e basic    # Run the basic suite (api + cli + infra basic tests)
./dev e2e extended # Run the extended suite
./dev e2e smoke    # Run smoke scenarios plus host Docker smoke checks
./dev smoke cloakbrowser # Run opt-in CloakBrowser Docker smoke image
./dev e2e api      # Run API basic tests
./dev e2e cli      # Run CLI basic tests
./dev e2e infra    # Run infra basic tests
./dev e2e api-extended   # Run API extended tests
./dev e2e cli-extended   # Run CLI extended tests
./dev e2e infra-extended # Run infra extended tests
./dev e2e api logs=show  # Opt back into full streaming logs
```

Or directly through the Go runner:
```bash
go run ./tests/tools/runner e2e --suite basic
go run ./tests/tools/runner e2e --suite extended
go run ./tests/tools/runner e2e --suite smoke
go run ./tests/tools/runner e2e --suite infra-extended --filter orchestrator
```

## E2E System Boundary

The E2E system has three layers with separate responsibilities. Keep this boundary explicit when adding scenarios or refactoring the runner.

### Go Runner

`go run ./tests/tools/runner e2e ...` is the host-side orchestrator. It owns:

- Suite normalization and expansion (`basic`, `extended`, `smoke`, group suites, and filtered smoke suites)
- Scenario discovery from `tests/e2e/scenarios/*/*.sh`
- Applying `tests/e2e/scenarios/manifest.json` metadata
- Selecting the compose stack, compose services, and readiness targets
- Passing explicit `scenario=<file>` arguments into the container executor
- Host Docker smoke checks and their required image build steps
- Log capture, result cleanup, summaries, markdown reports, failure accounting, GitHub Actions outputs, and GitHub Actions summaries

The runner does not own test assertions, endpoint request details, or CLI command behavior inside a scenario. Those stay in scenario files and helpers.

### Manifest

`tests/e2e/scenarios/manifest.json` is metadata, not the test list. Shell files are still discovered from disk. The manifest only controls per-scenario overrides:

- `tier`: `basic`, `extended`, or `smoke`
- `helper`: `api` or `cli`
- `services`: compose services required by the scenario
- `ready`: readiness targets required before the scenario runs
- `tags`: filter labels

By default, tier comes from the filename suffix: `*-basic.sh` is `basic`, `*-smoke.sh` is `smoke`, and every other scenario is `extended`. The default helper is `cli` for `scenarios/cli/` and `api` for the other groups. The default services are `pinchtab` and `fixtures`.

### Tiers

- `basic` is the PR happy path: fast, representative coverage with small setup. `./dev e2e basic` runs the API, CLI, and Infra basic suites.
- `extended` is deeper coverage: edge cases, detailed interaction checks, and tests that are useful before release. Extended suites include the matching `basic` scenarios plus `extended` scenarios for that group.
- `smoke` is independent high-setup coverage: lifecycle, multi-instance, host Docker, and production-like checks that do not belong in PR flow. `./dev e2e smoke` runs only `*-smoke.sh` scenarios plus host Docker smoke checks; it does not include `basic` or `extended`.

CloakBrowser smoke checks need a Linux CloakBrowser binary and a compatible
PinchTab image, so they are exposed as the manual Docker smoke command
`./dev smoke cloakbrowser` instead of being included in the CI-backed
`./dev e2e smoke` suite. By default, the command builds
`tests/tools/docker/cloakbrowser-smoke.Dockerfile` into
`pinchtab-cloakbrowser:test` and uses tmpfs mounts for browser profile and
socket paths. The smoke reuses `tests/e2e/fixtures`, checks a representative
endpoint subset, and then invokes the existing E2E scenario executor in the API
runner container attached to the Cloak-backed container's network namespace for
the Cloak-compatible API basic scenario set. This uses the same
`tests/e2e/run.sh` API helper and assertion base as the normal Chrome API
suite, while keeping CloakBrowser out of CI by default.

### Adding A Scenario

1. Add or update a grouped entrypoint under `tests/e2e/scenarios/api/`, `tests/e2e/scenarios/cli/`, `tests/e2e/scenarios/infra/`, or `tests/e2e/scenarios/plugin/`.
2. Pick the tier by filename: `feature-basic.sh`, `feature-extended.sh`, or `feature-smoke.sh`.
3. Source the local helper, usually `../../helpers/api.sh` or `../../helpers/cli.sh`.
4. Add a manifest entry only when the scenario needs non-default services, readiness targets, helper, tier, or tags.
5. Check the plan before running the full suite:

```bash
go run ./tests/tools/runner e2e --suite basic --dry-run
go run ./tests/tools/runner e2e --suite extended --filter feature --dry-run
go run ./tests/tools/runner e2e --suite smoke --filter feature --dry-run
```

### Filtering

`--filter TEXT` selects scenario files before compose planning. It is a case-sensitive substring match against the scenario file name, manifest key, group, tier, helper, and tags. Suites with no matching scenarios are skipped, and the runner only starts services required by the remaining scenarios.

For host Docker smoke checks, `--filter` matches the smoke step name or tags; required image build steps are included automatically when a filtered step depends on them.

`--test TEXT` is different: it does not select scenario files or compose services. It passes `E2E_TEST_FILTER` into the container so `run.sh` can run one matching `start_test` block inside the already-selected scenarios.

### CI Path

CI uses the Go runner directly:

- `.github/workflows/reusable-e2e.yml` runs `go run ./tests/tools/runner e2e --suite <suite> --logs hide`
- `.github/workflows/reusable-smoke.yml` runs `go run ./tests/tools/runner e2e --suite smoke --logs hide`
- `.github/workflows/ci-e2e.yml` chooses PR, extended-on-touch, manual, and smoke jobs, then delegates execution to the reusable workflows
- `.github/workflows/ci-smoke.yml` is the manual smoke entrypoint

Workflow YAML should stay thin: it decides when to run suites, while scenario selection, services, logs, reports, failure summaries, and GitHub Actions outputs stay in the Go layer.

## Architecture

```
tests/e2e/
├── docker-compose.yml      # Single-instance stack for basic suites
├── docker-compose-multi.yml # Multi-instance extended stack
├── config/                 # E2E-specific PinchTab configs
│   ├── pinchtab.json
│   ├── pinchtab-medium-permissive.json
│   ├── pinchtab-full-permissive.json
│   ├── pinchtab-secure.json
│   └── pinchtab-bridge.json
├── fixtures/               # Static HTML test pages
│   ├── index.html
│   ├── form.html
│   ├── table.html
│   └── buttons.html
├── helpers/                # Shared API/CLI E2E helpers
│   ├── api.sh
│   ├── api-http.sh
│   ├── api-assertions.sh
│   ├── api-actions.sh
│   ├── api-snapshot.sh
│   ├── cli.sh
│   └── base.sh
├── scenarios/              # Test scenarios organized by type
│   ├── api/                # Browser control and page interaction
│   │   ├── browser-basic.sh
│   │   ├── browser-extended.sh
│   │   ├── tabs-basic.sh
│   │   ├── tabs-extended.sh
│   │   ├── actions-basic.sh
│   │   ├── actions-extended.sh
│   │   ├── files-basic.sh
│   │   ├── files-extended.sh
│   │   ├── clipboard-basic.sh
│   │   └── console-basic.sh
│   ├── cli/                # CLI command tests
│   │   ├── browser-basic.sh
│   │   ├── browser-extended.sh
│   │   ├── tabs-basic.sh
│   │   ├── tabs-extended.sh
│   │   └── ...
│   └── infra/              # System, network, security, stealth
│       ├── system-basic.sh
│       ├── system-extended.sh
│       ├── network-basic.sh
│       ├── network-extended.sh
│       ├── security-basic.sh
│       ├── security-extended.sh
│       ├── stealth-basic.sh
│       ├── stealth-extended.sh
│       ├── orchestrator-extended.sh
│       ├── auth-extended.sh
│       └── ...
├── runner-api/             # API test runner container
│   └── Dockerfile
├── runner-cli/             # CLI test runner container
│   └── Dockerfile
└── results/                # Test output (gitignored)
```

The Docker stacks reuse the repository root `Dockerfile` and mount explicit config files with `PINCHTAB_CONFIG` instead of maintaining separate e2e-only images.

## Test Groups

Tests are organized into three parallel groups:

### API Group (`scenarios/api/`)
Browser control and page interaction tests:
- `browser-basic` / `browser-extended`
- `tabs-basic` / `tabs-extended`
- `actions-basic` / `actions-extended`
- `files-basic` / `files-extended`
- `clipboard-basic`
- `console-basic`

### CLI Group (`scenarios/cli/`)
CLI command tests:
- `browser-basic` / `browser-extended`
- `tabs-basic` / `tabs-extended`
- `actions-basic` / `actions-extended`
- `files-basic` / `files-extended`
- `system-basic` / `system-extended`
- And more...

### Infra Group (`scenarios/infra/`)
System, networking, security, and stealth tests:
- `system-basic` / `system-extended`
- `network-basic` / `network-extended`
- `security-basic` / `security-extended`
- `stealth-basic` / `stealth-extended`
- `orchestrator-extended`
- `auth-extended`
- `autosolver-smoke`
- `dashboard-smoke`
- manual autosolver check lives at `tests/manual/autosolver-check.sh`
- real-world autosolver smoke lives at `scripts/autosolver-realworld-smoke.sh`
- `idpi-extended`

The `basic` entrypoints are the PR happy path. The `extended` entrypoints add extra and edge-case coverage; extended suites include their matching basic scenarios. The `smoke` tier is separate and runs only `*-smoke.sh` scenarios plus host-level Docker smoke checks, not basic or extended scenarios. The Go runner selects the exact scenario files before entering the container; `run.sh` is only the in-container executor and requires explicit `scenario=<file>` arguments from the host runner.

Compose usage:
- `docker-compose.yml` powers `api`, `cli`, `infra`, and `cli-extended`
- `docker-compose-multi.yml` powers `api-extended` and `infra-extended`

## Adding Tests

1. Add or update a grouped entrypoint such as `tabs-basic.sh`, `tabs-extended.sh`, or `tabs-smoke.sh`
2. Source `../../helpers/api.sh` or `../../helpers/cli.sh`
3. Put PR happy-path coverage in `*-basic.sh`, deeper coverage in `*-extended.sh`, and slow/high-setup checks in `*-smoke.sh`
4. Add or update `scenarios/manifest.json` only when the scenario needs non-default services, readiness targets, helper, tier, or tags
5. Verify selection and service scope with `go run ./tests/tools/runner e2e --suite <suite> --filter <name> --dry-run`
6. Use the assertion helpers:

```bash
#!/bin/bash
GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

start_test "My test name"

# Assert HTTP status
assert_status 200 "${PINCHTAB_URL}/health"

# Assert JSON field equals value
RESULT=$(pt_get "/some/endpoint")
assert_json_eq "$RESULT" '.field' 'expected'

# Assert JSON contains substring
assert_json_contains "$RESULT" '.message' 'success'

# Assert array length
assert_json_length "$RESULT" '.items' 5

end_test
```

The action scenarios already cover common interaction regressions against the bundled fixtures:
- `tests/e2e/scenarios/api/actions-basic.sh` groups the API happy-path actions
- `tests/e2e/scenarios/cli/actions-basic.sh` groups the matching CLI commands

## Adding Fixtures

Add HTML files to `fixtures/` for testing specific scenarios:

- Forms and inputs
- Tables and data
- Dynamic content
- iframes
- File upload/download

## CI Integration

The E2E tests run automatically:
- On PRs: `api`, `cli`, and `infra` basic tests always run
- On PRs: touching any non-basic scenario also triggers the matching extended suite on its native compose stack
- On PRs: touching smoke scenarios or Docker smoke inputs triggers the smoke workflow
- Manually via workflow dispatch: Extended tests for all groups

## Result Files

The Go e2e runner captures runner output and writes each suite's result files in `tests/e2e/results/`. The container executor emits structured `E2E_RESULT` lines as tests finish, and the Go layer turns those into the final console summary, GitHub Actions summaries, durable summaries, and reports:

- `summary-api.txt` / `report-api.md`
- `summary-api-extended.txt` / `report-api-extended.md`
- `summary-cli.txt` / `report-cli.md`
- `summary-cli-extended.txt` / `report-cli-extended.md`
- `summary-infra.txt` / `report-infra.md`
- `summary-infra-extended.txt` / `report-infra-extended.md`
- `summary-api-smoke.txt` / `report-api-smoke.md`
- `summary-cli-smoke.txt` / `report-cli-smoke.md`
- `summary-infra-smoke.txt` / `report-infra-smoke.md`
- `summary-plugin-smoke.txt` / `report-plugin-smoke.md`
- `summary-docker-smoke.txt` / `report-docker-smoke.md`

The runner deletes the target suite files before each run to avoid stale output.
Saved and printed summaries include total test time and suite wall time.

The runner also saves the captured suite log under `output-*.log` and captures compose service logs on failure. When `logs=hide` is used and a suite fails, it prints the relevant failure summary and artifact paths.

## Debugging

### View container logs
```bash
docker compose -f tests/e2e/docker-compose.yml logs pinchtab
docker compose -f tests/e2e/docker-compose-multi.yml logs pinchtab
```

### Interactive shell in runner
```bash
docker compose -f tests/e2e/docker-compose.yml run runner-api bash
docker compose -f tests/e2e/docker-compose.yml run runner-cli bash
```

### Run specific scenario
```bash
go run ./tests/tools/runner e2e --suite api --filter tabs-basic
go run ./tests/tools/runner e2e --suite api-extended --filter tabs-extended
go run ./tests/tools/runner e2e --suite api-extended --filter tabs-extended --test "tab-scoped snapshot"
```

### Orchestrator Coverage
`infra-extended` uses `docker-compose-multi.yml` and includes the multi-instance and remote-bridge orchestrator scenarios through `orchestrator-extended.sh`.
