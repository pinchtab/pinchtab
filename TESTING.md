# Testing

## Quick Start with dev

The `dev` developer toolkit is the easiest way to run checks and tests:

```bash
./dev                    # Interactive picker
./dev test               # All tests (unit + E2E)
./dev test unit          # Unit tests only
./dev e2e                # E2E tests (both curl and CLI)
./dev e2e orchestrator   # Orchestrator-heavy E2E tests only
./dev e2e curl           # E2E curl tests only
./dev e2e cli            # E2E CLI tests only
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

### Curl Tests (HTTP API)

```bash
./dev e2e curl
```

Runs 183 HTTP-level tests using curl against the server. Tests the REST API, navigation, snapshots, and other HTTP endpoints.

### CLI Tests

```bash
./dev e2e cli
```

Runs CLI e2e tests. Tests the command-line interface directly.

### Both E2E Test Suites

```bash
./dev e2e
```

Runs all E2E tests (curl + CLI, 224 tests total).

### Orchestrator E2E Suite

```bash
./dev e2e orchestrator
```

Runs the orchestration-focused curl scenarios, including multi-instance flows and remote bridge attachment against the dedicated `pinchtab-bridge` Compose service.

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

E2E tests are organized in two directories:

- **`tests/e2e/scenarios/*.sh`** — HTTP curl-based tests (183 tests)
  - Test the REST API directly
  - Use Docker Compose: `tests/e2e/docker-compose.yml`

- **`tests/e2e/scenarios-orchestrator/*.sh`** — orchestration-heavy curl tests
  - Test multi-instance flows and remote bridge attachment
  - Use Docker Compose: `tests/e2e/docker-compose-orchestrator.yml`

- **`tests/e2e/scenarios-cli/*.sh`** — CLI e2e tests (41 tests)
  - Test the command-line interface
  - Use Docker Compose: `tests/e2e/docker-compose.cli.yml`

Each test is a standalone bash script that:
1. Starts the test server (or uses existing)
2. Runs curl or CLI commands
3. Asserts expected output or exit codes
4. Cleans up

## Writing New E2E Tests

Create a new bash script in `tests/e2e/scenarios/` (for curl tests) or `tests/e2e/scenarios-cli/` (for CLI tests):

### Example: Simple Curl Test

```bash
#!/bin/bash

# tests/e2e/scenarios/test-my-feature.sh

set -e  # Exit on error

# Source helpers
. "$(dirname "$0")/../helpers.sh"

# Test setup
SERVER_URL="http://localhost:9867"

# Start server if needed
start_test_server

# Run test
echo "Testing my feature..."
RESPONSE=$(curl -s "$SERVER_URL/health")

if [ "$(echo "$RESPONSE" | jq -r '.status')" != "ok" ]; then
    echo "❌ Health check failed"
    exit 1
fi

echo "✅ Test passed"
```

### Example: CLI Test

```bash
#!/bin/bash

# tests/e2e/scenarios-cli/test-my-cli.sh

set -e

# Source helpers
. "$(dirname "$0")/../helpers.sh"

# Test the CLI
echo "Testing pinchtab CLI..."
OUTPUT=$($PINCHTAB_BIN --version)

if [[ ! "$OUTPUT" =~ pinchtab ]]; then
    echo "❌ Version output incorrect"
    exit 1
fi

echo "✅ CLI test passed"
```

## Coverage

Generate coverage for unit tests:

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Note: E2E tests are black-box tests and don't contribute to code coverage metrics directly.
