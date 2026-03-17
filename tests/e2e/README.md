# E2E Test Suite

End-to-end tests for PinchTab that exercise the full stack including browser automation.

## Quick Start

### With Docker (recommended)

```bash
./dev e2e          # Run the release meta-suite
./dev e2e pr       # Run the PR meta-suite
./dev e2e recent   # Run only recently added/changed scenarios (fast feedback)
./dev e2e api-fast # Run the stable PR-fast API suite
./dev e2e cli-fast # Run the stable PR-fast CLI suite
./dev e2e full-api # Run the full API suite
./dev e2e full-cli # Run the full CLI suite
./dev e2e full-extended # Run the manual/pre-release extended suite
```

Or directly:
```bash
docker compose -f tests/e2e/docker-compose.yml up --build
docker compose -f tests/e2e/docker-compose-orchestrator.yml run --build --rm runner
```

## Architecture

```
tests/e2e/
├── docker-compose.yml      # Generic curl scenarios
├── docker-compose-orchestrator.yml # Orchestrator-specific services
├── config/                 # E2E-specific PinchTab configs
│   ├── pinchtab.json
│   ├── pinchtab-secure.json
│   └── pinchtab-bridge.json
├── fixtures/               # Static HTML test pages
│   ├── index.html
│   ├── form.html
│   ├── table.html
│   └── buttons.html
├── scenarios/              # Test scripts
│   ├── common.sh           # Shared utilities
│   ├── run-all.sh          # Generic curl scenarios
│   ├── run-fast.sh         # API fast suite runner
│   ├── 01-health.sh
│   ├── 02-navigate.sh
│   ├── 03-snapshot.sh
│   ├── 04-tabs-api.sh      # Regression test for #207
│   ├── 05-actions.sh
│   └── 06-screenshot-pdf.sh
├── scenarios-cli/          # CLI scenarios
│   ├── run-all.sh
│   ├── run-fast.sh         # CLI fast suite runner
│   └── ...
├── scenarios-recent/       # Recent edge-case coverage
│   ├── run.sh
│   └── 41-extensions.sh
├── scenarios-orchestrator/ # Multi-instance and attach flows
│   ├── run-all.sh
│   ├── 01-attach-bridge.sh
│   └── 31-multi-instance.sh
├── suites/                 # Curated PR-fast suite manifests
│   ├── api-fast.txt
│   └── cli-fast.txt
├── runner/                 # Test runner container
│   └── Dockerfile
└── results/                # Test output (gitignored)
```

The Docker stack reuses the repository root `Dockerfile` and mounts explicit config files with `PINCHTAB_CONFIG` instead of maintaining separate e2e-only images.

## Test Scenarios

| Script | Tests |
|--------|-------|
| 01-health | Basic connectivity, health endpoint |
| 02-navigate | Navigation, tab creation, tab listing |
| 03-snapshot | A11y tree extraction, text content |
| 04-tabs-api | Tab-scoped APIs (regression #207) |
| 05-actions | Click, type, press, check, and uncheck actions |
| 06-screenshot-pdf | Screenshot and PDF export |
| 40-activity | Activity API capture and filtering |
| scenarios-orchestrator/01-attach-bridge | Orchestrator attaches to the dedicated `pinchtab-bridge` container and proxies tab traffic |
| scenarios-orchestrator/31-multi-instance | Launch/list/stop and aggregate orchestration behavior |

## Adding Tests

1. Create a new script in `scenarios/` following the naming pattern `NN-name.sh`
2. Source `common.sh` for utilities
3. Use the assertion helpers:

```bash
#!/bin/bash
source "$(dirname "$0")/common.sh"

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
- `tests/e2e/scenarios/05-actions.sh` covers API actions including `check` and `uncheck`
- `tests/e2e/scenarios-cli/03-actions.sh` covers the matching CLI commands

## Adding Fixtures

Add HTML files to `fixtures/` for testing specific scenarios:

- Forms and inputs
- Tables and data
- Dynamic content
- iframes
- File upload/download

## CI Integration

The E2E tests run automatically:
- On PRs and pushes to `main`: `recent`, `api-fast`, and `cli-fast`
- Manually via workflow dispatch: `full-api`, `full-cli`, and `full-extended`

## Debugging

### View container logs
```bash
docker compose -f tests/e2e/docker-compose.yml logs pinchtab
```

### Interactive shell in runner
```bash
docker compose -f tests/e2e/docker-compose.yml run runner bash
```

### Run specific scenario
```bash
docker compose -f tests/e2e/docker-compose.yml run runner /scenarios/04-tabs-api.sh
```

### Run orchestrator scenarios
```bash
docker compose -f tests/e2e/docker-compose-orchestrator.yml run --build --rm runner
```

### Run remote bridge attach scenario
```bash
docker compose -f tests/e2e/docker-compose-orchestrator.yml run --build --rm runner /scenarios-orchestrator/01-attach-bridge.sh
```
