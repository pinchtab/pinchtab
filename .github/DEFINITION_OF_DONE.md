# Definition of Done (PR Checklist)

## Automated ✅ (CI/GitHub enforces these)
These run automatically via `ci.yml`. If your PR fails them, fix and re-push.
- [ ] Go formatting & linting passes (gofmt, vet, golangci-lint)
- [ ] Unit + E2E tests pass (`go test ./...` and `./dev e2e`)
- [ ] Build succeeds (`go build`)
- [ ] CodeQL security scan passes
- [ ] Branch naming follows convention

## Manual — Code Quality (Required)
- [ ] **Error handling explicit** — All errors wrapped with `%w`, no silent failures
- [ ] **No regressions** — Verify stealth, token efficiency, session persistence work (test locally if unsure)
- [ ] **SOLID principles** — Functions do one thing, testable, no unnecessary deps
- [ ] **No redundant comments** — Comments explain *why* or *context*, not *what* the code does
  - ❌ Bad: `// Loop through items` above `for _, item := range items`
  - ❌ Bad: `// Return error` above `return err`
  - ✅ Good: `// Prioritize chromium-browser on ARM64 (Raspberry Pi default)`
  - ✅ Good: `// Chrome may not be installed in CI, so empty result is valid`

## Manual — Testing (Required)
- [ ] **New/changed functionality has tests** — Same-package unit tests preferred
- [ ] **Docker E2E tests run locally** — If you modified handlers/bridge/tabs, run: `./dev e2e pr` and/or the narrower suite you changed, for example `./dev e2e api`, `./dev e2e cli`, or `./dev e2e infra`
- [ ] **npm commands work** (if npm wrapper touched):
  - `npm pack` in `/npm/` produces valid tarball
  - `npm install -g pinchtab` (or from local tarball) succeeds
  - `pinchtab --version` + basic commands work after install

## Manual — Documentation (Required)
- [ ] **README.md updated** — If user-facing changes (CLI, API, env vars, install)
- [ ] **/docs/ updated** — If API/architecture/perf changed (optional for small fixes)

## Manual — Review (Required)
- [ ] **PR description explains what + why** — Especially stealth/perf/compatibility impact
- [ ] **Commits are atomic** — Logical grouping, good messages
- [ ] **No breaking changes to npm** — Unless explicitly major version bump

## Conditional (Only if applicable)
- [ ] Headed-mode tested (if dashboard/UI changes)
- [ ] Breaking changes documented in PR description (if any)

---

## Quick Checklist (Copy/Paste for PRs)
```markdown
## Definition of Done
- [ ] Unit + Docker E2E tests added & passing
- [ ] Error handling explicit (wrapped with %w)
- [ ] No regressions in stealth/perf/persistence
- [ ] No redundant comments (explain why, not what)
- [ ] README/docs updated (if user-facing)
- [ ] npm install works (if npm changes)
```
