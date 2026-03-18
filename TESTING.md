# Testing

## Quick Start with dev

The `dev` developer toolkit is the easiest way to run checks and tests:

```bash
./dev                    # Interactive picker
./dev test               # All tests (unit + E2E)
./dev test unit          # Unit tests only
./dev e2e                # Release suite (api-full + cli-full)
./dev e2e pr             # PR suite (api-fast + cli-fast)
./dev e2e api-fast       # API fast, single-instance
./dev e2e cli-fast       # CLI fast, single-instance
./dev e2e api-full       # API full, multi-instance
./dev e2e cli-full       # CLI full, single-instance

/bin/bash tests/e2e/run.sh api
/bin/bash tests/e2e/run.sh api all=true
/bin/bash tests/e2e/run.sh cli
/bin/bash tests/e2e/run.sh cli all=true
./dev check              # All checks (format, vet, build, lint)
./dev check go           # Go checks only
./dev check security     # Gosec security scan
./dev format dashboard   # Run Prettier on dashboard sources
./dev doctor             # Setup dev environment
```

## Unit Tests

```bash
go test ./...
# or
./dev test unit
```

Unit tests are standard Go tests that validate individual packages and functions without launching a full server.

## E2E Tests

End-to-end tests launch a real pinchtab server with Chrome and run e2e-level tests against it.

### PR Suites

```bash
./dev e2e pr
./dev e2e api-fast
./dev e2e cli-fast
```

Use these on pull requests and during normal development:

- `pr` runs the same E2E suite composition as the PR workflow
- `api-fast` runs the API `*-basic.sh` groups on the single-instance stack
- `cli-fast` runs the CLI `*-basic.sh` groups on the single-instance stack

### Full API Suite

```bash
./dev e2e api-full
```

Runs the grouped API `basic` and `full` scenarios on the multi-instance stack. `api-fast` is the `basic` layer only on the single-instance stack; `api-full` adds the extra and edge-case groups plus the multi-instance-only coverage.

### Full CLI Suite

```bash
./dev e2e cli-full
```

Runs the grouped CLI `basic` and `full` scenarios on the single-instance stack. `cli-fast` is the `basic` layer only; `cli-full` adds the extra and edge-case groups.

### Release Meta-Suite

```bash
./dev e2e
```

Runs `api-full` and `cli-full` in sequence.

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

E2E tests are organized by surface and feature group:

- **`tests/e2e/scenarios-api/*.sh`** — grouped API entrypoints such as `tabs-basic.sh` and `tabs-full.sh`
  - `*-basic.sh` is the PR-fast happy-path layer
  - `*-full.sh` adds the extra and edge-case coverage for the same feature
  - Use Docker Compose: `tests/e2e/docker-compose.yml` for `api-fast`, `tests/e2e/docker-compose-multi.yml` for `api-full`

- **`tests/e2e/scenarios-cli/*.sh`** — grouped CLI entrypoints such as `tabs-basic.sh` and `tabs-full.sh`
  - Follows the same `basic` vs `full` split as the API side
  - Use Docker Compose: `tests/e2e/docker-compose.yml` for both `cli-fast` and `cli-full`

## E2E Results

Each suite writes its own summary and markdown report under `tests/e2e/results/`:

- `summary-api-fast.txt` / `report-api-fast.md`
- `summary-api-full.txt` / `report-api-full.md`
- `summary-cli-fast.txt` / `report-cli-fast.md`
- `summary-cli-full.txt` / `report-cli-full.md`

The runner clears the target suite files before each run so stale results do not survive into the next suite.

## Writing New E2E Tests

Add new coverage directly to a grouped entrypoint in `tests/e2e/scenarios-api/` or `tests/e2e/scenarios-cli/`. Keep `*-basic.sh` focused on the happy path and put the extra and edge-case coverage in the matching `*-full.sh`.

### Example: Grouped API Entrypoint

```bash
#!/bin/bash

# tests/e2e/scenarios-api/tabs-basic.sh
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../helpers/api.sh"

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
