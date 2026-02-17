# Pinchtab — TODO

**Philosophy**: 12MB binary. HTTP API. Minimal deps (chromedp + stdlib + yaml.v3). Internal tool, not a product.
If a feature needs a GUI, complex config, or "target users" — it's probably wrong.

---

## Completed

### P0–P5: Core (54 tests, 0 lint issues)
Safety, file split, Go idioms, testability, session persistence, stealth basics,
ref caching, action registry, hover/select/scroll actions, smart diff, text format,
readability `/text`, BridgeAPI interface, handler tests, nil guard, deprecated flag removal.
Navigate timeout fix (readyState polling). Restore concurrency limiting (max 3 tabs, 2 navigations).

### P6: Productivity
Action chaining (`POST /actions`), `/cookies` endpoint, LaunchAgent/systemd auto-start,
config file (`~/.pinchtab/config.json`).

### P7: Output Formats
File-based output (`?output=file`), YAML format (`?format=yaml`), Dockerfile (Alpine + Chromium).

### P8: Stealth & Human Interaction
Human mouse movement (bezier curves in `human.go`), human typing (variable delays, typo simulation),
`GET /stealth/status`, `POST /fingerprint/rotate`, `stealth.js` injected via `AddScriptToEvaluateOnNewDocument`.
Covers: navigator overrides, WebGL vendor spoof, plugin emulation, canvas noise, font metrics noise,
WebRTC filtering (iceTransportPolicy relay), timezone/hardware spoofing, Chrome flag hardening (20+ flags).

### P8-FIX: Stealth correctness fixes (2026-02-16)
- ✅ 8F-1: Removed duplicate hardwareConcurrency/deviceMemory definitions
- ✅ 8F-2: Session-stable PRNG seed from Go (`__pinchtab_seed`), Mulberry32 with memoization
- ✅ 8F-3: measureText returns Proxy wrapping real TextMetrics (preserves instanceof)
- ✅ 8F-4: toDataURL uses offscreen canvas (no source mutation)
- ✅ 8F-5: toBlob routes through noised toDataURL
- ✅ 8F-6: Chrome UA configurable via `BRIDGE_CHROME_VERSION` (default 133.0.6943.98)
- ✅ 8F-8: WebRTC uses iceTransportPolicy relay (no error throwing)
- ✅ 8F-10: Stealth status probes browser when tab available (fallback to static)
- ✅ 8F-11: Removed addEventListener mousemove wrapper

### Refactoring (2026-02-16)
- handlers.go split: 1,290 → 313 LOC + handler_actions.go (400) + handler_cookies.go (175) + handler_stealth.go (354)
- AGENTS.md removed from git, added to .gitignore
- docs/ARCHITECTURE.md — full system architecture overview
- README: headed vs headless modes, BRIDGE_CHROME_VERSION env var

---

## Open: Stealth Nice-to-Haves

### 8F-7: Use CDP for fingerprint rotation [LOW]
`Object.defineProperty` on navigator is detectable via `getOwnPropertyDescriptor`.
Proper fix: use `Network.setUserAgentOverride` for UA/platform/language at CDP level.
JS overrides as backup for properties CDP doesn't cover.

### 8F-9: Use CDP for timezone [LOW]
`window.__pinchtab_timezone` works but `Intl.DateTimeFormat` still leaks real TZ.
Proper fix: `Emulation.setTimezoneOverride("America/New_York")` — one line of Go.

---

## Open: Testing

54 unit + 7 integration tests. Integration tests require Chrome (`go test -tags integration`).

### Integration tests ✅ (build tag: `integration`)
- ✅ TestStealthScriptInjected — navigator.webdriver === undefined
- ✅ TestCanvasNoiseApplied — toDataURL produces different outputs per call
- ✅ TestFontMetricsNoise — Proxy-wrapped TextMetrics, positive widths
- ⏭️ TestWebGLVendorSpoofed — skips in headless (no GPU); passes with headed Chrome
- ✅ TestPluginsPresent — navigator.plugins >= 3
- ⏭️ TestFingerprintRotation — skips (needs CDP-level overrides, 8F-7)
- ✅ TestStealthStatusEndpoint — score >= 50, level high/medium

### Manual validation (quarterly)
Run against detection sites, document in QA.md:
- https://bot.sannysoft.com/
- https://abrahamjuliot.github.io/creepjs/
- https://browserleaks.com/

---

## Open: Minor

- [ ] **Dockerfile env vars** — `CHROME_BINARY` and `CHROME_FLAGS` set but not consumed by Go
- [ ] **Dockerfile CI step** — Add `docker build` to CI workflow
- [ ] **humanType global rand** — Accept `*rand.Rand` for reproducible tests

---

## P9: Multi-Agent Coordination

Best practice today: separate tabs per agent with explicit `tabId`.

- [ ] **Tab locking** — `POST /tab/lock`, `POST /tab/unlock` with timeout-based deadlock prevention
- [ ] **Tab ownership tracking** — Show owner in `/tabs` response
- [ ] **Ref cache versioning** — Prevent stale ref conflicts between agents
- [ ] **Agent sessions (optional)** — Isolated browser contexts for advanced workflows

Start with tab locking (solves 80% of conflicts). Add sessions only if needed.

## P10: Quality of Life

- [ ] **Ad blocking** — Basic tracker blocking for cleaner snapshots
- [ ] **CSS animation disabling** — Faster page loads, more consistent snapshots
- [ ] **Debloated Chrome launch** — Strip unnecessary features
- [ ] **Randomized window sizes** — Avoid automation fingerprint

---

## Not Doing
- Desktop app / GUI — agents don't need GUIs
- Plugin system
- Proxy rotation (IP-level)
- Multi-tenant SaaS
- Selenium compatibility
- Cloud anything
- MCP protocol — HTTP is the interface
- Distributed browser clusters
- Complex workflow orchestration — agents handle their own logic
