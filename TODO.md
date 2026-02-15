# Pinchtab — TODO

## Done ✅
- [x] Session persistence — save/restore tabs on shutdown/startup
- [x] Graceful shutdown — save state on SIGTERM/SIGINT
- [x] Launch helper — self-launches Chrome, no manual CDP flags
- [x] Snapshot pruning — `?filter=interactive`, `?depth=N`
- [x] `/text` endpoint — body text extraction
- [x] Ref resolution via DOM.resolveNode + backendDOMNodeId
- [x] Stealth mode — webdriver hidden, UA spoofed, automation flags removed
- [x] Tab registry — contexts survive across requests
- [x] Ref stability — snapshot caches ref→nodeID mapping per tab, actions use cached refs
- [x] Action timeouts — 15s default, prevents hung pages blocking handlers
- [x] Tab cleanup — background goroutine removes stale entries every 30s
- [x] Tab restore on startup — loadState() called, tabs reopened

## Done ✅ (P0)
- [x] `http.MaxBytesReader` (1MB) on all POST handlers
- [x] `r.Context()` propagated — `cancelOnClientDone` cancels CDP on client disconnect
- [x] Graceful shutdown with `context.WithTimeout` (10s)
- [x] `cleanStaleTabs` accepts `context.Context` — no goroutine leak
- [x] `tabContext` lock — RLock fast path, Lock only on miss, double-check pattern
- [x] Errors wrapped with `%w` consistently
- [x] All ignored errors handled (`os.MkdirAll`, `json.Encode`, `os.WriteFile`, `json.MarshalIndent`)

## Done ✅ (P1)
Split into 8 files (single package):
- [x] `config.go` (33 lines) — env vars, constants
- [x] `bridge.go` (117 lines) — Bridge struct, tabContext, cleanStaleTabs
- [x] `snapshot.go` (60 lines) — A11yNode, raw a11y types, interactiveRoles
- [x] `cdp.go` (76 lines) — clickByNodeID, typeByNodeID, listTargets
- [x] `state.go` (112 lines) — save/restore, markCleanExit
- [x] `handlers.go` (545 lines) — all HTTP handlers
- [x] `middleware.go` (53 lines) — auth, CORS, jsonResp, cancelOnClientDone
- [x] `main.go` (152 lines) — Chrome launch, routes, signal handling

## Done ✅ (P2)
- [x] Handlers are Bridge methods (receiver pattern, no free functions)
- [x] Action registry `map[string]ActionFunc` replaces switch in handleAction
- [x] `scrollIntoViewIfNeeded` before click and type actions
- [x] Constants for magic strings (targetTypePage, filterInteractive, action/tab kinds)
- [x] `log/slog` structured logging throughout (replaces `log.Printf`)
- [x] Godoc comments on all exported types and methods
- [x] `//go:embed stealth.js` — stealth script in separate file
- [x] Chrome opts grouped by concern (profile, stealth, perf, UI, identity)

## Done ✅ (P3)
- [x] Navigate uses raw `Page.navigate` + 500ms sleep (not chromedp.Navigate full load)
- [x] Restore is non-blocking (fire-and-forget goroutines, server starts instantly)
- [x] Request logging middleware (method/path/status/ms via slog)
- [x] Port changed 18800 → 9867, state dir ~/.browser-bridge → ~/.pinchtab
- [x] ActionFunc takes full `actionRequest` struct (no more fragmented params)

## P4: Testability
- [ ] Extract `Browser` interface (navigate, screenshot, evaluate)
- [ ] Extract `TabManager` interface (get, create, close, list)
- [ ] Add handler tests using `httptest` + mock interfaces
- [ ] Add snapshot unit tests — a11y tree filtering/parsing

## P5: Features
- [ ] **`/scroll` endpoint** — scroll to element or by amount. Needed for infinite-scroll pages (X, Reddit).
- [ ] **`withElement` helper** — generic `withElement(ctx, nodeID, jsFunc)` for all element actions (click, type, scroll, hover, drag)
- [ ] **Smart diff** — `?diff=true` returns only changes since last snapshot. Massive token savings on multi-step tasks
- [ ] **Wait for navigation** — after click, wait for page load before returning
- [ ] **Better /text** — Readability-style extraction instead of raw innerText
- [ ] **Split handlers.go** — snapshot handler is complex enough for its own file

## P6: Nice to Have
- [ ] **File-based output** — `?output=file` saves snapshot to disk, returns path (Playwright CLI approach)
- [ ] **Compact format** — YAML or indented text instead of JSON
- [ ] **Action chaining** — `POST /actions` batch multiple actions in one call
- [ ] **Docker image** — `docker run pinchtab` with bundled Chromium
- [ ] **Config file** — `~/.pinchtab/config.json`
- [ ] **LaunchAgent/systemd** — auto-start on boot

## Not Doing
- Plugin system
- Proxy rotation / anti-detection
- Session isolation / multi-tenant
- Selenium compatibility
- React UI
- Cloud anything
- MCP protocol (HTTP is the interface)
