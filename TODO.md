# Pinchtab — TODO

## Completed (P0–P5)
Safety, file split, Go idioms, testability, features — all done.
38 tests passing, 0 lint issues. See git history for details.

Key deliverables: session persistence, stealth mode, ref caching, action registry,
hover/select/scroll actions, smart diff, text format, readability /text,
BridgeAPI interface, handler tests, nil guard, deprecated flag removal.

---

## Bugs & In Progress
- [x] **Navigate timeout on some SPAs** — Fixed: `navigatePage` now polls `document.readyState` until interactive/complete (10s timeout)
- [x] **Restore navigates all tabs at once** — Fixed: Added concurrency limiting (max 3 concurrent tab creations, max 2 concurrent navigations) 
- [x] **Screenshot base64 returns raw bytes** — Fixed: Already working correctly, returns proper base64 string encoding in JSON

## P6: Next Up
- [ ] **Action chaining** — `POST /actions` batch multiple actions in one call (big token saver for agents)
- [ ] **`/cookies` endpoint** — read/set cookies (auth debugging)
- [ ] **LaunchAgent/systemd** — auto-start on boot
- [ ] **Config file** — `~/.pinchtab/config.json` (alternative to env vars)

## P7: Nice to Have
- [ ] **File-based output** — `?output=file` saves snapshot to disk, returns path
- [ ] **Compact format** — YAML or indented text instead of JSON
- [ ] **Docker image** — `docker run pinchtab` with bundled Chromium

## P8: Stealth & Anti-Detection
- [ ] **Enhanced stealth mode** — Fix additional headless detection vectors (pointer type, viewport handling, content isolations)
- [ ] **Human-like interactions** — Natural mouse movement algorithm with timing variations for clicks/types
  - `POST /action {"kind": "humanClick", "ref": "e5", "delay": "random"}`
  - `POST /action {"kind": "humanType", "text": "hello", "typing_delay": "natural"}`
- [ ] **Fingerprint rotation** — Randomize navigator properties, screen sizes, WebGL parameters between sessions
  - `POST /fingerprint/rotate {"os": "random", "screen": "random", "webgl": "spoof"}`
- [ ] **Stealth profiles API** — Configurable stealth levels (basic, enhanced, maximum)
  - `POST /profile {"stealth": "maximum", "fingerprint": "rotate", "humanMouse": true}`
- [ ] **Anti-fingerprinting core** — Spoof navigator device/OS/hardware properties, screen resolution, WebGL context
- [ ] **Font spoofing** — System-appropriate fonts with randomized metrics to prevent font fingerprinting
- [ ] **Memory optimization** — Reduce Chrome memory footprint through selective feature disabling
- [ ] **Request header spoofing** — Match User-Agent with navigator properties, rotate Accept-Language headers
- [ ] **DNS leak prevention** — Fix networking leaks when using proxies
- [ ] **Stealth status endpoint** — `GET /stealth/status` to check current anti-detection configuration

## P9: Multi-Agent Coordination
**Current status**: Multiple agents can share one Pinchtab instance but may conflict on ref caches and navigation. Best practice: use separate tabs per agent (`newTab: true`) and always specify `tabId`.

- [ ] **Tab locking mechanism** — Prevent conflicts when multiple agents access same Pinchtab instance
  - `POST /tab/lock {"tabId": "tab1", "lockId": "agent-uuid", "timeoutMs": 30000}`
  - `POST /tab/unlock {"tabId": "tab1", "lockId": "agent-uuid"}`
  - All operations require lockId when tab is locked
- [ ] **Agent sessions (optional)** — Isolated browser contexts per agent for complex workflows
  - `POST /session/create {"agentId": "agent1", "profile": "isolated"}` 
  - `GET /session/list` — show active agent sessions
  - `DELETE /session/{sessionId}` — cleanup agent session
- [ ] **Ref cache versioning** — Prevent stale ref conflicts between agents taking overlapping snapshots
- [ ] **Tab ownership tracking** — Show which agent/session owns which tabs in `/tabs` response
- [ ] **Concurrent action queuing** — Queue conflicting actions on same tab instead of failing
- [ ] **Agent-aware cleanup** — Clean up tabs/caches when agent sessions expire

**Implementation priority**: Start with tab locking (solves 80% of conflicts with minimal complexity), then add optional sessions for advanced use cases.

## P10: Quality of Life  
- [ ] **Built-in ad blocking** — Integrate basic ad/tracker blocking for cleaner automation (less noise in snapshots)
- [ ] **CSS animation disabling** — Skip animations for faster page loads and more consistent snapshots
- [ ] **Debloated Chrome launch** — Strip unnecessary Chrome features for lower memory usage and faster startup
- [ ] **Non-default window sizes** — Randomize initial window dimensions to avoid common automation fingerprints
- [ ] **Custom user data directory management** — Better session isolation and cleanup options

## Future: Desktop App Restructure
When a second binary (desktop app via Wails) is needed, restructure to:
```
cmd/pinchtab/main.go        # CLI binary
cmd/pinchtab-app/main.go    # desktop binary
internal/server/             # current Go files move here
internal/config/
app/                         # Wails desktop layer
frontend/                    # dashboard HTML/JS
```
Until then, flat structure is correct. Don't premature-abstract.

## Not Doing
- Plugin system
- Proxy rotation (IP-level)
- Multi-tenant SaaS (different from multi-agent coordination)
- Selenium compatibility
- React UI
- Cloud anything
- MCP protocol (HTTP is the interface)
- Machine learning / AI integration
- External fingerprint services
- Distributed browser clusters
- Complex workflow orchestration (agents should handle their own logic)
