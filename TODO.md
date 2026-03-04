# TODO — CLI Restore + Smart Endpoints

## 1. Docs: CDP Bridge & Wait Strategies
New section in `docs/` explaining how pinchtab handles page readiness at the CDP level.

- [ ] `docs/cdp-bridge.md` — new document covering:
  - **Navigate + wait model**: `NavigatePage` polls `readyState` every 200ms
  - **Wait modes**: `none`, `dom`, `selector`, `networkidle` (already in `/navigate`)
  - **Smart defaults per operation**:
    - `snapshot`/`text`/`find` → `dom` (DOM parsed = a11y tree ready, don't need images)
    - `screenshot`/`pdf` → `complete` (need visual rendering, fonts, images)
    - `evaluate` → `dom` (JS can run once DOM is interactive)
  - **Why not always networkidle**: SPAs never go idle (analytics, websockets), `dom` is faster and sufficient for most operations
  - **`url` param pattern**: any read operation accepts optional `url` — navigates first with appropriate wait, then operates
  - **Retry-on-empty as fallback**: if snapshot returns < N nodes, short retry (page still hydrating)
  - CDP lifecycle events reference: `Page.lifecycleEvent`, `Page.loadEventFired`, `Page.domContentEventFired`
- [ ] Add to `docs/index.json` under architecture section
- [ ] Update `docs/cli-commands.md` (or create new) with all restored CLI commands
- [ ] Update `docs/api-reference.json` — add `url` param to snapshot/text/find/screenshot/pdf/eval endpoints

## 2. Bridge Handler: `url` param support
Add optional `url` field to bridge handlers. If present, navigate + smart-wait before the operation.

- [ ] `ensureNavigated(ctx, url, defaultWait)` helper in handlers package
  - If `url` == "" → no-op (use current page)
  - If `url` != "" → `NavigatePage(url)` + `waitForNavigationState(defaultWait)`
  - Each handler passes its own smart default wait mode
- [ ] `HandleFind` — add `url` to request, default wait: `dom`
- [ ] `HandleSnapshot` — accept `url` query param, default wait: `dom`
- [ ] `HandleText` — accept `url` query param, default wait: `dom`
- [ ] `HandleScreenshot` — accept `url` query param, default wait: `complete`
- [ ] `HandlePDF` — accept `url` query param, default wait: `complete`
- [ ] `HandleEvaluate` — accept `url` in body, default wait: `dom`
- [ ] All handlers also accept `waitFor` override (user can force `networkidle` if needed)
- [ ] Tests for each handler with and without `url`

## 3. Consistency: `/find` snapshot logic in bridge
Currently `/find` auto-snapshot is in the strategy layer. Move it down to the bridge handler.

- [ ] Bridge `HandleFind` always ensures fresh snapshot (auto-fetch if cache empty)
- [ ] Remove `SnapshotTab()` call from simple strategy `handleFind`
- [ ] Remove `SnapshotTab()` call from session strategy `handleFind`
- [ ] Simple/session strategy `/find` becomes plain proxy (no special handling)
- [ ] Remove `ProxyWithTabID` from BridgeClient if no longer needed
- [ ] Verify `/find` works in: bridge mode, simple strategy, session strategy, legacy proxy

## 4. CLI: Restore browser control commands
Thin HTTP wrappers in `cmd/pinchtab/cmd_cli.go`. Each hits `PINCHTAB_URL`.

**Navigation:**
- [ ] `pinchtab nav <url>` → `POST /navigate` (flags: `--wait dom|complete|networkidle|selector`, `--selector`)

**Read operations (accept optional URL):**
- [ ] `pinchtab snap [url]` → `GET /snapshot` (flags: `--interactive`, `--compact`, `--text`, `--depth`)
- [ ] `pinchtab find <query> [--url <url>]` → `POST /find` (flags: `--top`, `--threshold`)
- [ ] `pinchtab text [url]` → `GET /text`
- [ ] `pinchtab screenshot [url] [--out file]` → `GET /screenshot`
- [ ] `pinchtab pdf [url] [--out file]` → `GET /pdf`
- [ ] `pinchtab eval <js>` → `POST /evaluate`

**Actions (require prior nav):**
- [ ] `pinchtab click <ref>` → `POST /action`
- [ ] `pinchtab type <ref> <text>` → `POST /action`
- [ ] `pinchtab fill <ref> <text>` → `POST /action`
- [ ] `pinchtab press <key>` → `POST /action`
- [ ] `pinchtab hover <ref>` → `POST /action`
- [ ] `pinchtab scroll [ref]` → `POST /action`
- [ ] `pinchtab select <ref> <value>` → `POST /action`

**Meta:**
- [ ] Update `pinchtab help` with all commands
- [ ] Update `main.go` `isCLICommand()` list
- [ ] **Drop `quick`** — `snap <url>` replaces it

## 5. Tests
- [ ] Unit tests for `ensureNavigated` helper
- [ ] Unit tests for each CLI command (mock HTTP server)
- [ ] Unit tests for `url` param in bridge handlers (snapshot, text, find, screenshot, pdf)
- [ ] Integration: `POST /find` with `url` in bridge mode
- [ ] Integration: `POST /find` with `url` through simple strategy
- [ ] Integration: `pinchtab find "example" --url https://example.com` CLI end-to-end
- [ ] Integration: `pinchtab snap https://example.com` CLI end-to-end

## Implementation Order
1. Bridge handler `ensureNavigated` + `url` param on all read handlers
2. Move `/find` snapshot logic into bridge handler
3. Docs part 1: `cdp-bridge.md` — CDP wait strategies, smart defaults, `url` param pattern
4. CLI commands
5. Docs part 2: CLI reference — all commands, flags, examples
6. Tests

## Notes
- `quick` command dropped — `snap <url>` is the same thing
- Wait defaults are per-operation, not global — smarter than one-size-fits-all
- User can always override with `--wait` / `waitFor` param
- Actions (click/type/fill) don't accept `url` — you need snap first to get refs

## Future: Action commands without prior snapshot
Actions require refs from a snapshot. Current approach: explicit snap → act → snap loop.
Revisit later — options considered:
- A: Auto-snap before action if no cached snapshot
- B: Support `--selector` for CSS-based actions (no snapshot needed)
- C: Natural language actions (`pinchtab click "Sign In"`) — find + act combo
Decision: accept explicit snap→act for now, revisit when usage patterns are clearer.
