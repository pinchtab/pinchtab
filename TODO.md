# Pinchtab — TODO

**Philosophy**: 12MB binary. HTTP API. Minimal deps. Internal tool, not a product.

---

## Done (P0–P8)

Core HTTP API, 16 endpoints, session persistence, ref caching, action registry,
smart diff, readability `/text`, config file, Dockerfile, YAML/file output,
stealth suite (navigator, WebGL, canvas noise, font metrics, WebRTC, timezone,
plugins, Chrome flags), human interaction (bezier mouse, typing simulation),
fingerprint rotation via CDP (`SetUserAgentOverride`, `SetTimezoneOverride`),
handlers.go split (4 files), architecture docs. **62 tests** (54 unit + 8 integration).

---

## Open

### Minor
- [ ] **Dockerfile env vars** — `CHROME_BINARY`/`CHROME_FLAGS` set but not consumed by Go
- [ ] **humanType global rand** — Accept `*rand.Rand` for reproducible tests

### P9: Multi-Agent Coordination
- [ ] **Tab locking** — `POST /tab/lock`, `POST /tab/unlock` with timeout-based deadlock prevention
- [ ] **Tab ownership tracking** — Show owner in `/tabs` response

### P10: Quality of Life
- [ ] **Ad blocking** — Basic tracker blocking for cleaner snapshots
- [ ] **CSS animation disabling** — Faster page loads, more consistent snapshots
- [ ] **Randomized window sizes** — Avoid automation fingerprint

---

## Not Doing
Desktop app, plugin system, proxy rotation, SaaS, Selenium compat, MCP protocol,
cloud anything, distributed clusters, workflow orchestration.
