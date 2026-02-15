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

## Done ✅ (P4)
- [x] Extract `Browser` interface (navigate, screenshot, evaluate) → `interfaces.go`
- [x] Extract `TabManager` interface (get, create, close, list) → `interfaces.go`
- [x] Extract `buildSnapshot` from handler → testable pure function in `snapshot.go`
- [x] Add snapshot unit tests — a11y tree filtering/parsing, depth, interactive filter
- [x] Add middleware tests — auth, CORS, logging, jsonResp, jsonErr (httptest)
- [x] Add bridge tests — ref cache concurrency, ref lookup
- [x] Add config tests — envOr, homeDir, constants
- [x] **18 tests passing** — `go test ./...`

## Done ✅ (P5)
- [x] **`withElement` helper** — generic `withElement(ctx, nodeID, jsFunc)` + `withElementArg` for all element actions
- [x] **`hover` action** — mouseover/mouseenter via ref, selector
- [x] **`select` action** — pick `<select>` option by value or text, fires change event
- [x] **`scroll` action** — scroll to element (ref), by pixel amount (`scrollX`/`scrollY`), or default 800px down
- [x] **Smart diff** — `?diff=true` returns added/changed/removed nodes since last snapshot
- [x] **Wait for navigation** — `"waitNav": true` on click action, 1s delay for page load
- [x] **Text format** — `?format=text` returns indented tree (~40-60% fewer tokens than JSON)
- [x] **21 tests passing** — diff, text format, all existing tests green

## Done ✅ (P5c)
- [x] **BridgeAPI interface** — `interfaces.go` with TabContext, ListTargets, CreateTab, CloseTab, Get/Set/DeleteRefCache
- [x] **Handlers use Bridge methods** — No direct `b.mu`/`b.snapshots`/`b.tabs` access in handlers
- [x] **CreateTab/CloseTab on Bridge** — Tab lifecycle extracted from handleTab
- [x] **Nil browserCtx guard** — ListTargets + TabContext return error instead of panic
- [x] **22 handler tests** — All validation/error paths tested via httptest without Chrome
- [x] **38 total tests passing**, 0 lint issues

## Done ✅ (P5b)
- [x] **Remove `--disable-blink-features=AutomationControlled` flag** — deprecated in Chrome 144+, stealth.js handles it
- [x] **Better /text** — Readability-style extraction (`readability.js`), strips nav/footer/aside/ads, prefers article/main. `?mode=raw` for old innerText.
- [x] **Split handlers.go** — snapshot handler extracted to `handler_snapshot.go`

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

## P7: Nice to Have
- [ ] **File-based output** — `?output=file` saves snapshot to disk, returns path (Playwright CLI approach)
- [ ] **Compact format** — YAML or indented text instead of JSON
- [ ] **Action chaining** — `POST /actions` batch multiple actions in one call
- [ ] **Docker image** — `docker run pinchtab` with bundled Chromium
- [ ] **Config file** — `~/.pinchtab/config.json`
- [ ] **LaunchAgent/systemd** — auto-start on boot
- [ ] **`/cookies` endpoint** — read/set cookies (useful for auth debugging)

## Not Doing
- Plugin system
- Proxy rotation / anti-detection
- Session isolation / multi-tenant
- Selenium compatibility
- React UI
- Cloud anything
- MCP protocol (HTTP is the interface)
