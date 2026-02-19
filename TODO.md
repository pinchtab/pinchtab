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
(headed mode), stealth Date.getTimezoneOffset recursion fix, native Chrome UA.
**100+ unit tests, ~100 integration, 36% coverage.**

---

## Open

### ~~P0-P2~~ — DONE
P0 (K10 profile hang), P1 (token optimization: maxTokens/selector/compact),
P2 (K11 file path, blockImages on CreateTab) — all resolved.

### ~~P3: Multi-Agent~~ — DONE
- [x] **Tab locking** — `POST /tab/lock`, `POST /tab/unlock` with timeout-based deadlock prevention (default 30s, max 5min). Same owner can re-lock (extend). 409 on conflict.
- [x] **Tab ownership tracking** — `/tabs` shows `owner` and `lockedUntil` on locked tabs.

### P4: Quality of Life
- [ ] **Headed mode testing** — Run Section 2 tests to validate non-headless.
- [ ] **Ad blocking** — Basic tracker blocking for cleaner snapshots.
- [x] **CSS animation disabling** — `BRIDGE_NO_ANIMATIONS` env + `?noAnimations=true` per-request.
- [ ] **Randomized window sizes** — Avoid automation fingerprint.

### Code Quality
- [x] **Extract TabManager from Bridge** — Tabs, snapshots, and ref cache in own struct with setup hook.
- [ ] **installStableBinary streaming** — Use `io.Copy` with file streams instead of reading entire binary into memory.
- [x] **Interfaces for ProfileManager/Orchestrator** — `ProfileService` and `OrchestratorService` interfaces with compile-time checks.
- [ ] **proxy_ws.go proper HTTP** — Replace raw `backend.Write` of HTTP headers with proper request construction.
- [x] **Dashboard SSE keepalive** — 30s keepalive comments on SSE connections.

### Minor
- [ ] **humanType global rand** — Accept `*rand.Rand` for reproducible tests.
- [ ] **Batch empty array** — Return specific error instead of generic decode error.
- [ ] **Canvas noise in headless** — `TestCanvasNoiseApplied` fails (headless Chrome limitation, `full` stealth only).
- [ ] **`hardwareConcurrency` redefine warning** — Suppress warning during fingerprint rotation.

### Release
- [ ] **Tag v0.4.0** — Pre-release tests pass (186/189, 98.4%). Ready to tag.

---

## Known Bugs

- **Batch empty array** — `POST /actions []` returns generic decode error instead of "empty batch" message.
- **`hardwareConcurrency` redefine warning** — Console warning during fingerprint rotation (cosmetic).
- **Canvas noise in headless** — `toDataURL()` returns identical data in headless Chrome. Only affects `full` stealth mode.
- **No tab limit** — Can open unlimited tabs with no cap enforced.
- **Headed mode** — Experimental, not fully tested. Profile management is manual and cumbersome.

---

## Not Doing
Desktop app, plugin system, proxy rotation, SaaS, Selenium compat, MCP protocol,
cloud anything, distributed clusters, workflow orchestration.
