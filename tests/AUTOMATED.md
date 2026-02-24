# Automated Integration Tests

This document tracks which scenarios from the test plan are now covered by automated CI tests in `integration/`.

**CI Workflow:** `.github/workflows/integration.yml` — runs on PRs and main branch pushes.

**Run locally:** `go test -tags integration -v -timeout 10m -count=1 ./tests/integration/`

---

## Test Coverage (Automated)

### Health & Startup
- ✅ **H1** — Health check (`GET /health` returns 200 with status=ok)

### Navigation
- ✅ **N1** — Basic navigate to example.com
- ✅ **N2** — Navigate returns title
- ✅ **N3** — SPA title loading (httpbin.org/html)
- ✅ **N4** — Navigate with newTab flag
- ✅ **N5** — Navigate invalid URL returns error
- ✅ **N6** — Navigate missing URL returns 400
- ✅ **N7** — Navigate bad JSON returns 400
- ✅ **N8** — Navigation timeout behavior (reserved IP timeout)

### Snapshot (Accessibility Tree)
- ✅ **S1** — Basic snapshot returns nodes/tree
- ✅ **S2** — Interactive filter works
- ✅ **S3** — Depth filter works
- ✅ **S4** — Text format output
- ✅ **S5** — YAML format output
- ✅ **S5** (variant) — maxTokens parameter
- ✅ **S6** — Snapshot diff mode (optimized delta)
- ✅ **S7** — Snapshot diff first call (graceful fallback)
- ✅ **S8** — Snapshot file output (save to disk)
- ✅ **S9** — Snapshot with tabId parameter (specific tab extraction)
- ✅ **S10** — Snapshot no tab error (bad tabId returns error)
- ✅ **S11** — Large page snapshot (20K+ tokens, no timeout)
- ✅ **S12** — Ref stability across actions (refs unchanged after click)

### Text Extraction
- ✅ **T1** — Readability mode (`GET /text`)
- ✅ **T2** — Raw mode (`GET /text?mode=raw`)
- ✅ **T3** — Text with tabId parameter (specific tab extraction)
- ✅ **T4** — Text no tab error (bad tabId returns error)
- ✅ **T5** — Token efficiency (real-world content handling)

### Actions
- ✅ **A1** — Click by ref
- ✅ **A2** — Type by ref
- ✅ **A3** — Fill by ref
- ✅ **A4** — Press key
- ✅ **A5** — Focus element
- ✅ **A6** — Hover action
- ✅ **A7** — Select option
- ✅ **A8** — Scroll page
- ✅ **A9** — Unknown kind returns 400
- ✅ **A10** — Missing kind returns 400
- ✅ **A11** — Ref not found error
- ✅ **A12** — CSS selector click
- ✅ **A13** — Action no tab error (bad tabId)
- ✅ **A14** — Batch actions
- ✅ **A15** — Batch empty returns 400

### Tabs
- ✅ **TB1** — List tabs
- ✅ **TB2** — New tab
- ✅ **TB3** — Close tab
- ✅ **TB4** — Close without tabId returns 400
- ✅ **TB5** — Bad action returns 400
- ✅ **TB6** — Max tabs limit behavior

### Screenshots
- ✅ **SS1** — Basic screenshot (base64)
- ✅ **SS2** — Raw screenshot (JPEG bytes) — skips gracefully on headless limitations

### JavaScript Evaluation
- ✅ **E1** — Simple eval (1+1)
- ✅ **E2** — DOM eval (document.title)
- ✅ **E3** — Missing expression returns 400
- ✅ **E4** — Bad JSON returns 400

### PDF Export
- ✅ **PD1** — PDF base64 output
- ✅ **PD2** — PDF raw bytes
- ✅ **PD3** — PDF save to file
- ✅ **PD5** — PDF landscape mode
- ✅ **PD6** — PDF scale parameter

### File Upload
- ✅ **UP1** — Upload single file with selector (skips if file:// not supported)
- ✅ **UP4** — Upload multiple files (skips if file:// not supported)
- ✅ **UP6** — Upload with default selector (skips if file:// not supported)
- ✅ **UP7** — Upload invalid selector error (skips if file:// not supported)
- ✅ **UP8** — Upload missing files error
- ✅ **UP9** — Upload file not found error
- ✅ **UP11** — Upload bad JSON error

### Cookies
- ✅ **C1** — Get cookies
- ✅ **C2** — Set cookies
- ✅ **C3** — Get cookies no tab (error)
- ✅ **C4** — Set cookies bad JSON (400)
- ✅ **C5** — Set cookies empty (400)

### Stealth & Fingerprinting
- ✅ **ST1** — navigator.webdriver undefined
- ✅ **ST3** — navigator.plugins present
- ✅ **ST4** — window.chrome.runtime present
- ✅ **ST5** — Fingerprint rotation with OS specified
- ✅ **ST6** — Fingerprint rotation random (no OS)
- ✅ **ST8** — Stealth status endpoint

*Note: ST2 (canvas noise) skipped — unreliable in headless CI. ST7 replaced with specific tab rotation test.*

### Error Handling & Edge Cases
- ✅ **ER3** — Binary page (PDF URL) graceful handling
- ✅ **ER4** — Rapid navigate stress test (concurrent requests)
- ✅ **ER5** — Unicode content (CJK/emoji/RTL) handling in snapshot & text
- ✅ **ER6** — Empty page (about:blank) handling in snapshot & text

### Configuration
- ✅ **CF1** — Config file preference (config.json loading)
- ✅ **CF2** — Env overrides config (BRIDGE_PORT precedence)
- ✅ **CF3** — CDP_URL external Chrome (remote CDP connection)
- ✅ **CF4** — Custom profile directory (`BRIDGE_PROFILE` env var)
- ✅ **CF5** — No restore flag (`BRIDGE_NO_RESTORE=true`)
- ✅ **CF6** (variant) — Chrome version override via TEST_CHROME_VERSION
- ✅ **CF7** — Chrome version default in UA
- ✅ **CF8** — Chrome version persists after fingerprint rotate

---

## Manual Test Coverage

The following scenarios require manual testing or deployment-specific setups:

### Manual Verification (Fix Verified in Code)
- ✅ **CF3-Extended** — CDP_URL mode (fix verified, needs manual test to confirm: `manual/cf3-cdp-create-tab-repro.md`)

### Not Automating (Not Worth It)
- **ER1, ER2, ER7-ER8** — Chrome crash recovery, connection refused, port conflict (system-level, not practical)

### Manual Testing (Additional/Verification)
- 🔄 **UP1-UP11** (7 tests) — File upload also in CI but skips if `file://` fails. See `tests/manual/file-upload.md` for headed Chrome verification.
- 🔄 **SS2** (1 test) — Raw screenshot also in CI but skips on headless limitations. See `tests/manual/screenshot-raw.md` for headed Chrome verification.
- **A16-A17** — Human click/type (bezier movement, mouse trajectory)
- **SP1-SP3** — Session persistence (requires server restart sequencing)
- **HM1-HM3** — Headed mode (requires display server)
- **MA1-MA8** — Multi-agent scenarios (requires coordination)
- **Docker (D1-D7)** — Requires Docker, deployment testing
- **Dashboard (DA1-DA5)** — Requires manual profile management

See `manual/` directory for detailed test plans.

---

## Performance Testing

Token usage, speed benchmarks, and Chrome startup metrics tracked separately in `performance/`.

---

## Statistics

**Automated (in CI, may skip on headless limitations):** 84 scenarios
- Health: 1
- Navigation: 8 (N1-N8)
- Snapshot: 12 (S1-S12)
- Text: 5 (T1-T5)
- Actions: 15 (A1-A15)
- Tabs: 6 (TB1-TB6)
- Screenshots: 2 (SS1-SS2) — SS2 skips if headless
- Eval: 4 (E1-E4)
- PDF: 5 (PD1-PD3, PD5-PD6)
- File Upload: 7 (UP1, UP4, UP6-UP9, UP11) — skip if file:// not supported
- Cookies: 5 (C1-C5)
- Stealth: 6 (ST1, ST3-ST6, ST8)
- Error Handling: 4 (ER3-ER6)
- Configuration: 8 (CF1-CF8)

**Automated (always run):** ~76 tests (some UP/SS2 skip on headless)

**Manual Testing (Additional/Verification):** 16 scenarios
- 🔄 UP1-UP11 (7 tests) — Also in CI, verify with headed Chrome
- 🔄 SS2 (1 test) — Also in CI, verify with headed Chrome
- A16-A17, SP1-SP3, HM1-HM3, MA1-MA8, D1-D7, DA1-DA5 (8 tests)

**Not Automating:** 4 scenarios (ER1, ER2, ER7-ER8 — system-level)  
**Total Coverage:** 98 test scenarios

**Coverage achieved: 86% automated (84 of 98 test scenarios, 76+ always run)**

---

*Last updated: 2026-02-24 21:50 GMT — 84 automated (86%), 16 additional manual, 4 not doing*
