# Testing

## Quick Start with dev

The `dev` developer toolkit is the easiest way to run checks and tests:

```bash
./dev                    # Interactive picker
./dev test               # All tests (unit + E2E)
./dev test unit          # Unit tests only
./dev e2e                # Release suite (all extended tests)
./dev e2e pr             # PR suite (api + cli + infra basic)
./dev e2e api            # API basic tests
./dev e2e cli            # CLI basic tests
./dev e2e infra          # Infra basic tests
./dev e2e api-extended   # API extended, multi-instance
./dev e2e cli-extended   # CLI extended tests
./dev e2e infra-extended # Infra extended, multi-instance
./dev e2e api-extended auth  # Extended API suite filtered to "auth"

/bin/bash tests/e2e/run.sh api
/bin/bash tests/e2e/run.sh api extended=true
/bin/bash tests/e2e/run.sh api extended=true filter=auth
/bin/bash tests/e2e/run.sh cli
/bin/bash tests/e2e/run.sh cli extended=true
/bin/bash tests/e2e/run.sh infra
./dev check              # All checks (format, vet, build, lint)
./dev check go           # Go checks only
./dev check security     # Gosec security scan
./dev format dashboard   # Run Prettier on dashboard sources
./dev doctor             # Setup dev environment
```

E2E summaries and markdown reports prefix each test with its scenario filename, for example `[auth-extended] auth: login sets session cookie`, so it is easy to see which filename filter to use.

## Unit Tests

```bash
go test ./...
# or
./dev test unit
```

Unit tests are standard Go tests that validate individual packages and functions without launching a full server.

## E2E Tests

End-to-end tests launch a real pinchtab server with Chrome and run e2e-level tests against it. Tests are organized into three parallel groups:

- **api** — Browser control and page interaction (tabs, actions, files)
- **cli** — CLI command tests
- **infra** — System, network, security, stealth, orchestration

### PR Suites

```bash
./dev e2e pr
./dev e2e api
./dev e2e cli
./dev e2e infra
```

Use these on pull requests and during normal development:

- `pr` runs all three basic suites (same as CI PR workflow)
- `api` runs the API `*-basic.sh` groups on the single-instance stack
- `cli` runs the CLI `*-basic.sh` groups on the single-instance stack
- `infra` runs the Infra `*-basic.sh` groups on the single-instance stack

### Extended Suites

```bash
./dev e2e api-extended
./dev e2e cli-extended
./dev e2e infra-extended
```

Extended suites run both `*-basic.sh` and `*-extended.sh` scenarios plus standalone scripts. `api-extended` and `infra-extended` use the multi-instance stack for orchestration coverage.

### Release Meta-Suite

```bash
./dev e2e
./dev e2e release
```

Runs `api-extended`, `cli-extended`, and `infra-extended` in sequence.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `CI` | _(unset)_ | Set to `true` for longer health check timeouts (60s vs 30s) |

### Temp Directory Layout

Each E2E test run creates a single temp directory under `/tmp/pinchtab-test-*/`:

```
/tmp/pinchtab-test-123456789/
├── pinchtab          # Compiled test binary
├── state/            # Dashboard state (profiles, instances)
└── profiles/         # Chrome user-data directories
```

Everything is cleaned up automatically when tests finish.

## Test File Structure

E2E tests are organized by group and feature:

```
tests/e2e/scenarios/
├── api/           # Browser control, tabs, actions, files
│   ├── browser-basic.sh
│   ├── browser-extended.sh
│   ├── tabs-basic.sh
│   └── ...
├── cli/           # CLI command tests
│   ├── browser-basic.sh
│   ├── browser-extended.sh
│   └── ...
└── infra/         # System, network, security, stealth
    ├── system-basic.sh
    ├── system-extended.sh
    ├── stealth-basic.sh
    ├── orchestrator-extended.sh
    └── ...
```

- `*-basic.sh` is the PR happy-path layer
- `*-extended.sh` adds extra and edge-case coverage
- Standalone scripts (no suffix) run only in extended mode

Docker Compose files:
- `tests/e2e/docker-compose.yml` — single-instance stack for basic tests
- `tests/e2e/docker-compose-multi.yml` — multi-instance stack for extended tests

## E2E Results

Each suite writes its own summary and markdown report under `tests/e2e/results/`:

- `summary-api.txt` / `report-api.md`
- `summary-api-extended.txt` / `report-api-extended.md`
- `summary-cli.txt` / `report-cli.md`
- `summary-cli-extended.txt` / `report-cli-extended.md`
- `summary-infra.txt` / `report-infra.md`
- `summary-infra-extended.txt` / `report-infra-extended.md`

The runner clears the target suite files before each run so stale results do not survive into the next suite.

## Writing New E2E Tests

Add new coverage directly to a grouped entrypoint in `tests/e2e/scenarios/api/`, `tests/e2e/scenarios/cli/`, or `tests/e2e/scenarios/infra/`. Keep `*-basic.sh` focused on the happy path and put the extra and edge-case coverage in the matching `*-extended.sh`.

### Example: Grouped API Entrypoint

```bash
#!/bin/bash

# tests/e2e/scenarios/api/tabs-basic.sh
GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

start_test "tab-scoped snapshot"
# ...

start_test "tab focus"
# ...
end_test
```

## Coverage

Generate coverage for unit tests:

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Note: E2E tests are black-box tests and don't contribute to code coverage metrics directly.
