# Definition of Done (PR Checklist)

## Automated ✅ (CI/GitHub enforces these)
These run automatically via `ci.yml`. If your PR fails them, fix and re-push.
- [ ] Go formatting & linting passes (gofmt, vet, golangci-lint)
- [ ] Unit + integration tests pass (`go test ./...`)
- [ ] Build succeeds (`go build`)
- [ ] CodeQL security scan passes
- [ ] Branch naming follows convention

## Manual — Code Quality (Required)
- [ ] **Error handling explicit** — All errors wrapped with `%w`, no silent failures
- [ ] **No regressions** — Verify stealth, token efficiency, session persistence work (test locally if unsure)
- [ ] **SOLID principles** — Functions do one thing, testable, no unnecessary deps

## Manual — Testing (Required)
- [ ] **New/changed functionality has tests** — Same-package unit tests preferred; use mockBridge for integration
- [ ] **Integration tests run locally** — If you modified handlers/bridge/tabs, run: `go test -tags integration ./tests/integration`
- [ ] **npm commands work** (if npm wrapper touched):
  - `npm pack` in `/npm/` produces valid tarball
  - `npm install -g pinchtab` (or from local tarball) succeeds
  - `pinchtab --version` + basic commands work after install

## Manual — Documentation (Required)
- [ ] **README.md updated** — If user-facing changes (CLI, API, env vars, install)
- [ ] **CHANGELOG.md entry added** — Under `## Unreleased` with PR link
- [ ] **/docs/ updated** — If API/architecture/perf changed (optional for small fixes)

## Manual — Review (Required)
- [ ] **PR description explains what + why** — Especially stealth/perf/compatibility impact
- [ ] **Commits are atomic** — Logical grouping, good messages
- [ ] **No breaking changes to npm** — Unless explicitly major version bump

## Conditional (Only if applicable)
- [ ] Headed-mode tested (if dashboard/UI changes)
- [ ] CHANGELOG mentions breaking changes (if any)

---

## Quick Checklist (Copy/Paste for PRs)
```markdown
## Definition of Done
- [ ] Unit/integration tests added & passing
- [ ] Error handling explicit (wrapped with %w)
- [ ] No regressions in stealth/perf/persistence
- [ ] README/CHANGELOG updated (if user-facing)
- [ ] npm install works (if npm changes)
```
