---
name: pinchtab-dev
description: Develop and contribute to the PinchTab project. Use when working on PinchTab source code, adding features, fixing bugs, running tests, or preparing PRs. Triggers on "work on pinchtab", "pinchtab development", "contribute to pinchtab", "fix pinchtab bug", "add pinchtab feature".
---

# PinchTab Development

PinchTab is a browser control server for AI agents — 12MB Go binary with HTTP API.

## Project Location

```bash
cd ~/dev/pinchtab
```

## Dev Commands

All development commands run via `./dev`:

| Command | Description |
|---------|-------------|
| `./dev build` | Build the application |
| `./dev dev` | Build & run |
| `./dev dashboard` | Hot-reload dashboard development (Vite + Go) |
| `./dev run` | Run the application |
| `./dev check` | All checks (Go + Dashboard) |
| `./dev check go` | Go checks only |
| `./dev check dashboard` | Dashboard checks only |
| `./dev test unit` | Go unit tests |
| `./dev test dashboard` | Dashboard unit tests |
| `./dev e2e pr` | PR suite (api-fast + cli-fast) |
| `./dev e2e release` | Release suite (api-full + cli-full) |
| `./dev e2e docker` | Build local image and Docker smoke test |
| `./dev doctor` | Setup dev environment |

## Architecture

```
cmd/pinchtab/     CLI entry point
internal/
  bridge/         Chrome CDP communication
  handlers/       HTTP API handlers
  server/         HTTP server
  dashboard/      Embedded React dashboard
  config/         Configuration
  assets/         Embedded assets (stealth.js)
dashboard/        React dashboard source (Vite + TypeScript)
tests/e2e/        E2E test suites
```

## Workflow: New Feature or Bug Fix

1. **Create branch** from `main`:
   ```bash
   git checkout main && git pull
   git checkout -b feat/my-feature  # or fix/my-bug
   ```

2. **Make changes** — follow code patterns in existing files

3. **Run checks locally**:
   ```bash
   ./dev check        # Lint + format + typecheck
   ./dev test unit    # Go unit tests
   ./dev e2e pr       # E2E tests (Docker required)
   ```

4. **Commit** with conventional commits:
   - `feat:` new feature
   - `fix:` bug fix
   - `refactor:` code change without behavior change
   - `test:` adding tests
   - `docs:` documentation
   - `chore:` maintenance

5. **Push and create PR**

## Definition of Done (PR Checklist)

### Required — Code Quality
- Error handling explicit — all errors wrapped with `%w`, no silent failures
- No regressions — verify stealth, token efficiency, session persistence
- SOLID principles — functions do one thing, testable
- No redundant comments — explain *why*, not *what*

### Required — Testing
- New/changed functionality has tests
- Docker E2E tests pass locally: `./dev e2e pr`
- If npm wrapper touched: `npm pack` and `npm install` work

### Required — Documentation
- README.md updated if user-facing changes
- /docs/ updated if API/architecture changed

### Required — Review
- PR description explains what + why
- Commits are atomic with good messages

## Key Files

| File | Purpose |
|------|---------|
| `internal/assets/stealth.js` | Bot detection evasion (light/medium/full levels) |
| `internal/bridge/bridge.go` | Chrome CDP bridge |
| `internal/handlers/*.go` | HTTP API endpoints |
| `dashboard/src/` | React dashboard source |
| `tests/e2e/scenarios-api/` | API E2E tests |
| `tests/e2e/scenarios-cli/` | CLI E2E tests |

## Testing

### Unit Tests
```bash
./dev test unit              # All Go tests
go test ./internal/handlers  # Specific package
```

### E2E Tests (requires Docker)
```bash
./dev e2e pr                    # Fast suite for PRs (api-fast + cli-fast)
./dev e2e api-fast              # API tests only (fast)
./dev e2e cli-fast              # CLI tests only (fast)
./dev e2e api-full              # API tests (full, with multiple configs)
./dev e2e cli-full              # CLI tests (full)
./dev e2e release               # Full release suite (api-full + cli-full)
./dev e2e docker                # Docker smoke test only

# Run specific test file(s) with filter (second argument)
./dev e2e api-fast clipboard              # Run only clipboard-basic.sh
./dev e2e api-full "clipboard|console"    # Run clipboard and console tests
./dev e2e cli-fast browser                # Run browser-basic.sh in CLI suite
```

The filter is a regex pattern matched against scenario filenames. Requires Docker daemon running.

### Dashboard Tests
```bash
./dev test dashboard  # Vitest
cd dashboard && npm test
```

## Dashboard Development

### Setup

Start hot-reload development:
```bash
./dev dashboard
```

This runs:
- Backend on `:9867`
- Vite dev server on `:5173` with hot-reload
- Dashboard at `http://localhost:5173/dashboard/`

### Development Workflow (Use PinchTab to Develop PinchTab)

**Do not assume changes worked.** Use pinchtab itself to verify changes visually:

1. **Start dev mode**:
   ```bash
   ./dev dashboard
   ```

2. **Make changes** to files in `dashboard/src/`

3. **Verify with pinchtab** — use the pinchtab skill to inspect the dashboard:
   ```bash
   # Navigate to the page under development
   curl -X POST http://localhost:9867/navigate \
     -d '{"url":"http://localhost:5173/dashboard/settings"}'
   
   # Take a screenshot to verify the change
   curl -X POST http://localhost:9867/screenshot \
     -d '{"path":"/tmp/dashboard-check.png"}'
   
   # Or get a snapshot to inspect elements
   curl -s http://localhost:9867/snapshot | jq .
   ```

4. **Provide evidence** — when reporting changes, include:
   - Link to the page: `http://localhost:5173/dashboard/{page}`
   - Screenshot of the result
   - Relevant snapshot data if inspecting specific elements

### Example: Verifying a Settings Page Change

```bash
# Navigate to settings
curl -X POST http://localhost:9867/navigate \
  -d '{"url":"http://localhost:5173/dashboard/settings"}'

# Screenshot the result
curl -X POST http://localhost:9867/screenshot \
  -d '{"path":"./dashboard-settings.png","fullPage":true}'

# Find specific element
curl -X POST http://localhost:9867/find \
  -d '{"selector":"[data-testid=stealth-level]"}'
```

### Key Dashboard Pages

| Page | URL | Purpose |
|------|-----|---------|
| Home | `/dashboard/` | Instance overview |
| Settings | `/dashboard/settings` | Configuration |
| Profiles | `/dashboard/profiles` | Browser profiles |
| Tabs | `/dashboard/tabs` | Active tabs |

### Dashboard Tech Stack

- React 19 + TypeScript
- Vite (build/dev)
- Tailwind CSS
- Zustand (state)
- Vitest (tests)

## Stealth Module

The stealth module (`internal/assets/stealth.js`) has three levels:

| Level | Features | Trade-offs |
|-------|----------|------------|
| `light` | webdriver, CDP markers, plugins, hardware | None — safe |
| `medium` | + userAgentData, chrome.runtime.connect, csi/loadTimes | May affect error monitoring |
| `full` | + WebGL/canvas noise, WebRTC relay | May break WebRTC, canvas apps |

Configure in `~/.pinchtab/config.json`:
```json
{
  "instanceDefaults": {
    "stealthLevel": "medium"
  }
}
```

## Common Tasks

### Add new API endpoint
1. Create handler in `internal/handlers/`
2. Register route in `internal/server/routes.go`
3. Add tests in same package
4. Add E2E test in `tests/e2e/scenarios-api/`

### Modify stealth behavior
1. Edit `internal/assets/stealth.js`
2. Run `./dev build` (embeds via go:embed)
3. Test with `./dev e2e api-fast` (includes stealth tests)

### Update dashboard
1. Run `./dev dashboard` for hot-reload
2. Edit files in `dashboard/src/`
3. Run `./dev check dashboard` before commit
