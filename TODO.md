# Pinchtab — TODO

**Philosophy**: 12MB binary. HTTP API. Minimal deps. Internal tool, not a product.

---

## DONE

Core HTTP API (18 endpoints), session persistence, ref caching, action registry,
smart diff, readability `/text`, config file, Dockerfile, YAML/file output,
stealth suite (light/full modes), human interaction (bezier mouse, typing sim),
fingerprint rotation, image/media blocking, stealth injection on all tabs,
K1-K11 all fixed, multi-agent concurrency (MA1-MA8), token optimization
(`maxTokens`, `selector`, `format=compact`), Dockerfile env vars consumed by Go,
tab locking (`/tab/lock`, `/tab/unlock`), CSS animation disabling, welcome page
(headed mode), stealth Date.getTimezoneOffset recursion fix, native Chrome UA,
tab limit (`BRIDGE_MAX_TABS`, default 20), tab close error on bogus IDs.
**120+ unit tests, ~100 integration, 36% coverage.**

---

## Open

### P4: Quality of Life
- [ ] **Headed mode testing** — Run Section 2 tests to validate non-headless.
- [ ] **Ad blocking** — Basic tracker blocking for cleaner snapshots.
- [ ] **Randomized window sizes** — Avoid automation fingerprint.

### Code Quality
- [ ] **installStableBinary streaming** — Use `io.Copy` with file streams instead of reading entire binary into memory.
- [ ] **proxy_ws.go proper HTTP** — Replace raw `backend.Write` of HTTP headers with proper request construction.

### Minor
- [ ] **humanType global rand** — Accept `*rand.Rand` for reproducible tests.
- [ ] **Canvas noise in headless** — `TestCanvasNoiseApplied` fails (headless Chrome limitation, `full` stealth only).
- [ ] **`hardwareConcurrency` redefine warning** — Suppress warning during fingerprint rotation.

### Release
- [ ] **Tag v0.5.0** — Pre-release tests pass (67/74, 90.5% on main). Ready to tag.

---

## Known Bugs

- **`hardwareConcurrency` redefine warning** — Console warning during fingerprint rotation (cosmetic).
- **Canvas noise in headless** — `toDataURL()` returns identical data in headless Chrome. Only affects `full` stealth mode.

---

## Not Doing
Desktop app, plugin system, proxy rotation, SaaS, Selenium compat, MCP protocol,
cloud anything, distributed clusters, workflow orchestration.
