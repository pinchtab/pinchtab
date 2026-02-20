# Contributing to Pinchtab

## Setup

```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab

# Build
go build -o pinchtab ./cmd/pinchtab

# Run (headed — Chrome window opens)
./pinchtab

# Run headless
BRIDGE_HEADLESS=true ./pinchtab

# Enable pre-commit hook
git config core.hooksPath .githooks
```

Requires **Go 1.25+** and **Google Chrome**.

## Development Workflow

1. Make your changes
2. Format: `gofmt -w .`
3. Test: `go test ./... -count=1`
4. Lint: `golangci-lint run ./...`
5. Commit — pre-commit hook runs checks automatically
6. Push: `git pull --rebase && git push`

## Running Tests

```bash
# All tests
go test ./... -count=1 -v

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

Most tests use a `mockBridge` and do not require a running Chrome instance.

## Project Layout

The project follows the standard Go `internal/` pattern to ensure encapsulation and clean boundaries:

- `cmd/pinchtab/` — Application entry points and CLI commands.
- `internal/bridge/` — Core CDP logic, tab management, and state logic.
- `internal/handlers/` — HTTP API handlers and middleware.
- `internal/orchestrator/` — Multi-instance lifecycle and process management.
- `internal/profiles/` — Chrome profile management and user identity discovery.
- `internal/dashboard/` — Backend logic and static assets for the web UI.
- `internal/assets/` — Centralized embedded files (stealth scripts, CSS).
- `internal/human/` — Human-like interaction simulation (Bezier mouse paths, natural typing).
- `internal/web/` — Shared HTTP and JSON utilities.

## Style

- `gofmt` enforced (CI + pre-commit)
- Adhere to **SOLID** principles, specifically using interfaces for dependency inversion.
- Handle all error returns explicitly.
- Lowercase error strings, wrap with `%w`.
- Tests should live in the same package as the source code.
- No new dependencies without significant technical justification.

## For AI Agents

See [AGENTS.md](AGENTS.md) for detailed conventions and patterns when contributing via an agentic workflow.
