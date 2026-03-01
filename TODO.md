# PinchTab API Refactoring TODO

## Overview

Refactor from deeply nested instance-centric API to flatter, resource-centric API focused on **tabs** as the primary resource.

**Current (nested):** `POST /instances/{id}/navigate` + `POST /instances/{id}/tab` + `POST /instances/{id}/action`
**New (flat):** `POST /navigate {tabId, url}` + `POST /tabs/new {instanceId}` + `POST /action {tabId, ...}`

---

## New API Structure

### Phase 1: Profile Management (Already ~complete)
```
GET    /profiles              List profiles
POST   /profiles              Create profile
GET    /profiles/{id}         Get profile
DELETE /profiles/{id}         Delete profile
```

### Phase 2: Instance Management (Simplify current)
```
GET    /instances             List running instances
POST   /instances/start       Start instance with profile
  {profile: "name", mode: "headed|headless", port: optional}
  → {id: "inst_xyz", profileId, port, mode, status}

POST   /instances/{id}/stop   Stop instance
GET    /instances/{id}/logs   View instance logs
```

### Phase 3: Tab Management (NEW - Main focus)
```
POST   /tabs/new              Create new tab in instance
  {instanceId: "inst_xyz", url: optional}
  → {id: "tab_abc", instanceId, url, status}

GET    /tabs                  List all tabs (across all instances)
GET    /tabs?instanceId=...   List tabs in specific instance
GET    /tabs/{id}             Get tab info
POST   /tabs/{id}/close       Close tab
```

### Phase 4: Tab Operations (Flatten from /instances/{id}/...)
```
POST   /tabs/{id}/navigate    Navigate tab
  {url, timeout, blockImages, blockAds}

GET    /tabs/{id}/snapshot    Get page structure
  ?interactive, ?compact, ?depth, ?maxTokens

GET    /tabs/{id}/screenshot  Screenshot
  ?format=png|jpeg, ?quality=80

POST   /tabs/{id}/action      Single action
  {kind: "click"|"type"|"press"|"fill"|"hover"|"scroll"|"select"|"focus", ...}

POST   /tabs/{id}/actions     Multiple actions
  {actions: [{kind, ...}, ...]}

GET    /tabs/{id}/text        Extract visible text
POST   /tabs/{id}/evaluate    Run JavaScript
  {expression, await}

GET    /tabs/{id}/pdf         Export as PDF
  ?landscape, ?margins, ?scale, ?pages

POST   /tabs/{id}/cookies     Get/set cookies
GET    /tabs/{id}/cookies

POST   /tabs/{id}/lock        Lock tab (exclusive access)
  {owner: "agent-name", ttl: 60}

POST   /tabs/{id}/unlock      Unlock tab
  {owner: "agent-name"}

GET    /tabs/{id}/locks       Check lock status
POST   /tabs/{id}/fingerprint/rotate  Rotate fingerprint
```

---

## Current Endpoints (To Deprecate/Refactor)

### Instance-scoped endpoints (Remove after migration)
```
❌ POST   /instances/{id}/navigate        → POST /tabs/{id}/navigate
❌ GET    /instances/{id}/snapshot        → GET /tabs/{id}/snapshot
❌ GET    /instances/{id}/screenshot      → GET /tabs/{id}/screenshot
❌ POST   /instances/{id}/action          → POST /tabs/{id}/action
❌ POST   /instances/{id}/actions         → POST /tabs/{id}/actions
❌ GET    /instances/{id}/text            → GET /tabs/{id}/text
❌ POST   /instances/{id}/evaluate        → POST /tabs/{id}/evaluate
❌ GET    /instances/{id}/pdf             → GET /tabs/{id}/pdf
❌ POST   /instances/{id}/tab             → POST /tabs/new {instanceId}
❌ GET    /instances/{id}/tabs            → GET /tabs?instanceId={id}
❌ POST   /instances/{id}/tab/lock        → POST /tabs/{id}/lock
❌ POST   /instances/{id}/tab/unlock      → POST /tabs/{id}/unlock
❌ GET    /instances/{id}/cookies         → GET /tabs/{id}/cookies
❌ POST   /instances/{id}/cookies         → POST /tabs/{id}/cookies
❌ GET    /instances/{id}/download        → GET /tabs/{id}/download (future)
❌ POST   /instances/{id}/upload          → POST /tabs/{id}/upload (future)
❌ POST   /instances/{id}/ensure-chrome   (Keep for health check, but hidden)
```

### Old instance launch endpoint
```
❌ POST   /instances/launch {name, headless, port}
  → POST /instances/start {profile, mode, port}
```

---

## Implementation Checklist

### Phase 1: Add New Tab-focused Endpoints
- [ ] Create `POST /tabs/new {instanceId, url}` handler
  - Calls bridge.CreateTab(url)
  - Returns tab info with ID
  - Stores mapping of tabId → (instanceId, tabHandle)

- [ ] Create `GET /tabs` handler (list all)
  - Queries all instances for their tabs
  - Returns flattened list with tabId, instanceId, url, title

- [ ] Create `GET /tabs?instanceId=...` handler
  - Filters to specific instance
  - Reuses GetTabs logic

- [ ] Create `GET /tabs/{id}` handler
  - Returns single tab info

- [ ] Create tab operation handlers (refactored from instance handlers)
  - `/tabs/{id}/navigate` → Extract from `/instances/{id}/navigate`
  - `/tabs/{id}/snapshot` → Extract from `/instances/{id}/snapshot`
  - `/tabs/{id}/screenshot` → Extract from `/instances/{id}/screenshot`
  - `/tabs/{id}/action` → Extract from `/instances/{id}/action`
  - `/tabs/{id}/actions` → Extract from `/instances/{id}/actions`
  - `/tabs/{id}/text` → Extract from `/instances/{id}/text`
  - `/tabs/{id}/evaluate` → Extract from `/instances/{id}/evaluate`
  - `/tabs/{id}/pdf` → Extract from `/instances/{id}/pdf`
  - `/tabs/{id}/cookies` (GET/POST) → Extract from `/instances/{id}/cookies`
  - `/tabs/{id}/lock` → Extract from `/instances/{id}/tab/lock`
  - `/tabs/{id}/unlock` → Extract from `/instances/{id}/tab/unlock`

### Phase 2: Add Tab ID Resolution Layer
- [ ] Create `TabResolver` interface
  - Method: `ResolveTab(tabId) → (instanceId, tabHandle, error)`
  - Method: `ResolveInstance(tabId) → (instanceId, error)`

- [ ] Add to orchestrator
  - Maps tabId → instanceId globally
  - Routes tab operations to correct instance

- [ ] Update Bridge interface
  - Add method: `TabByID(tabId) → (tab.Info, error)`

### Phase 3: Refactor Handler Logic
- [ ] Extract common logic from instance handlers
  - TabResolver lookup
  - Parameter passing to instance
  - Response formatting

- [ ] Create tab-specific handler package
  - `handlers/tabs/navigate.go`
  - `handlers/tabs/snapshot.go`
  - `handlers/tabs/action.go`
  - etc.

- [ ] Update Handlers struct
  - Add `TabResolver` field
  - Keep Bridge for backward compat (bridge mode)

### Phase 4: Deprecate Old Endpoints
- [ ] Add deprecation warnings to old endpoints
  - Log "DEPRECATED: Use /tabs/... instead"
  - Return 307 Temporary Redirect or 308 Permanent Redirect

- [ ] Keep old endpoints working
  - Translate to new handlers internally
  - `POST /instances/{id}/navigate` → calls `/tabs/{id}/navigate` internally

- [ ] Document migration path
  - Example: old curl → new curl

### Phase 5: Update CLI Commands
- [ ] Update CLI to use new endpoints
  - `pinchtab nav <url>` → stays same (auto-detects tab)
  - `pinchtab --instance <id> nav <url>` → `pinchtab --tab <id> nav <url>`
  - `pinchtab snap` → `pinchtab --tab <id> snap`

- [ ] Add `--tab` flag support
  - Alternative to `--instance`
  - Auto-detects instance from tab

- [ ] Update docs/cli-*.md
  - Reflect new structure

### Phase 6: Update Documentation
- [ ] Update API reference (docs/references/api-reference.json)
  - Add all new endpoints
  - Mark old endpoints as deprecated

- [ ] Update architecture docs
  - Explain tab-centric design
  - Show tabId routing

- [ ] Update get-started guide
  - Profile → Instance → Tab workflow

- [ ] Update examples
  - Curl examples with new endpoints
  - CLI examples

---

## Endpoint Mapping (Migration Guide)

### Example: Click Element

**Old (instance-centric):**
```bash
curl -X POST http://localhost:9867/instances/inst_abc123/action \
  -H "Content-Type: application/json" \
  -d '{"kind": "click", "ref": "e5"}'
```

**New (tab-centric):**
```bash
# First, get tab ID
TAB=$(curl -s http://localhost:9867/tabs?instanceId=inst_abc123 | jq -r '.[0].id')

# Then operate on tab
curl -X POST http://localhost:9867/tabs/$TAB/action \
  -H "Content-Type: application/json" \
  -d '{"kind": "click", "ref": "e5"}'
```

Or just:
```bash
curl -X POST http://localhost:9867/tabs/tab_xyz789/action \
  -H "Content-Type: application/json" \
  -d '{"kind": "click", "ref": "e5"}'
```

**CLI (both work):**
```bash
# Old
pinchtab --instance inst_abc123 click e5

# New (cleaner - direct tab reference)
pinchtab --tab tab_xyz789 click e5

# Or still support old way internally
pinchtab --instance inst_abc123 click e5  # Auto-detects default tab
```

---

## Example: Full Workflow

### New (Flat, Tab-centric)

**Step 1: Create profile**
```bash
curl -X POST http://localhost:9867/profiles \
  -d '{"name": "my-profile"}'
# → {id: "prof_123", name: "my-profile"}
```

**Step 2: Start instance**
```bash
curl -X POST http://localhost:9867/instances/start \
  -d '{"profile": "my-profile", "mode": "headed", "port": 9868}'
# → {id: "inst_abc", profileId: "prof_123", port: 9868, status: "running"}
```

**Step 3: Create tab**
```bash
curl -X POST http://localhost:9867/tabs/new \
  -d '{"instanceId": "inst_abc"}'
# → {id: "tab_xyz", instanceId: "inst_abc", status: "ready"}
```

**Step 4: Navigate (direct to tab)**
```bash
curl -X POST http://localhost:9867/tabs/tab_xyz/navigate \
  -d '{"url": "https://example.com"}'
```

**Step 5: Get page structure**
```bash
curl -s http://localhost:9867/tabs/tab_xyz/snapshot?interactive=true | jq .
```

**Step 6: Click element**
```bash
curl -X POST http://localhost:9867/tabs/tab_xyz/action \
  -d '{"kind": "click", "ref": "e5"}'
```

**Step 7: Extract text**
```bash
curl -s http://localhost:9867/tabs/tab_xyz/text | jq .
```

**Step 8: Close tab**
```bash
curl -X POST http://localhost:9867/tabs/tab_xyz/close
```

**Step 9: Stop instance**
```bash
curl -X POST http://localhost:9867/instances/inst_abc/stop
```

---

## Benefits of New Structure

| Aspect | Old (Nested) | New (Flat) |
|--------|--------------|-----------|
| **Focus** | Instance-centric | Tab-centric |
| **Main resource** | Instance ID | Tab ID |
| **URL depth** | 3-4 levels | 2 levels |
| **Parameter passing** | Via path segments | Via body/query |
| **Scaling** | Hard (more nesting) | Easy (flat) |
| **Tab switching** | Change instance | Change tab |
| **Multi-tab workflows** | Complex (one instance) | Simple (many tabs) |
| **CLI complexity** | `--instance --tab` flags | Just `--tab` flag |
| **HTTP cacheability** | Low (deep paths) | Higher (flat paths) |

---

## Code Structure Changes

### Current
```
handlers/
  handlers.go (bridge-only, ~30 handlers)
    - HandleNavigate
    - HandleSnapshot
    - HandleAction
    - HandleTab
    - HandleTabs
    - etc.

orchestrator/
  handlers.go (~40 endpoints routed to instances)
    - handleLaunchByName
    - handleStopByInstanceID
    - handleNavigate (proxies to /instances/{id}/navigate)
    - etc.
```

### New
```
handlers/
  handlers.go (bridge-only, ~30 handlers) [UNCHANGED]
  tabs/
    resolver.go     (tab ID → instance mapping)
    navigate.go     (POST /tabs/{id}/navigate)
    snapshot.go     (GET /tabs/{id}/snapshot)
    action.go       (POST /tabs/{id}/action)
    actions.go      (POST /tabs/{id}/actions)
    text.go         (GET /tabs/{id}/text)
    evaluate.go     (POST /tabs/{id}/evaluate)
    pdf.go          (GET /tabs/{id}/pdf)
    screenshot.go   (GET /tabs/{id}/screenshot)
    cookies.go      (GET/POST /tabs/{id}/cookies)
    lock.go         (POST /tabs/{id}/lock)
    unlock.go       (POST /tabs/{id}/unlock)

orchestrator/
  handlers.go (refactored)
    - handleListTabs
    - handleNewTab
    - handleCloseTab
    - [Proxy to handlers/tabs/]

  tabs/
    handler.go      (routes to handlers/tabs/*)
    resolver.go     (tab ID lookup)
```

---

## Migration Path

### For Users
1. **No breaking changes initially** — Old endpoints still work
2. **Gradual migration** — Update to new endpoints at own pace
3. **CLI stays compatible** — `pinchtab --instance` still works

### For Maintainers
1. **Phase 1-3** — Add new endpoints alongside old
2. **Phase 4** — Mark old as deprecated (deprecation headers)
3. **Phase 5-6** — Update CLI & docs to prefer new
4. **Maintenance window** — After N months, remove old endpoints

---

## Testing Strategy

### Unit Tests
- [ ] Tab resolver tests
  - Resolve valid tabId → instanceId
  - Handle invalid tabId
  - Handle cross-instance tab access

- [ ] Tab operation tests
  - Navigate to URL
  - Get snapshot
  - Execute actions
  - Lock/unlock

### Integration Tests
- [ ] Full workflow (profile → instance → tab → actions)
- [ ] Multi-tab scenario
- [ ] Tab lifecycle (create, use, close)
- [ ] Cross-instance tab access (should fail appropriately)

### Backward compat tests
- [ ] Old endpoints still work
- [ ] Old endpoints return deprecation headers
- [ ] CLI with `--instance` flag still works

---

## Effort Estimation

| Phase | Task | Effort | Days |
|-------|------|--------|------|
| 1 | Add new tab endpoints | Medium | 2-3 |
| 2 | Tab resolver layer | Medium | 1-2 |
| 3 | Refactor handlers | Medium | 2-3 |
| 4 | Deprecation + compat | Small | 1 |
| 5 | Update CLI | Medium | 1-2 |
| 6 | Update docs | Small | 1 |
| | **Tests & QA** | Medium | 2-3 |
| | **TOTAL** | | ~11-17 days |

---

## Success Criteria

- [x] All new endpoints documented and in API reference
- [x] CLI commands work with `--tab` flag
- [x] Old endpoints still work (deprecated)
- [x] Documentation updated
- [x] Examples updated
- [x] All tests passing
- [x] No breaking changes for users

---

## Related Files to Update

1. **Endpoint docs**
   - [ ] docs/references/api-reference.json (add 15+ new endpoints)
   - [ ] docs/references/cli-design.md (update patterns)
   - [ ] docs/references/cli-implementation.md (update code examples)
   - [ ] docs/references/cli-quick-reference.md (update curl examples)

2. **Code**
   - [ ] internal/handlers/handlers.go (add tab operation handlers to bridge)
   - [ ] internal/orchestrator/handlers.go (add tab endpoints + routing)
   - [ ] cmd/pinchtab/cmd_cli.go (add --tab support)
   - [ ] internal/bridge/bridge.go (add TabByID method if needed)

3. **Tests**
   - [ ] internal/handlers/*_test.go (add tab tests)
   - [ ] internal/orchestrator/*_test.go (add tab routing tests)
   - [ ] tests/integration/*_test.go (add workflow tests)

4. **Architecture docs**
   - [ ] docs/architecture/architecture-overview.md (explain tab-centric model)
   - [ ] docs/core-concepts.md (update concepts with tabs as primary resource)
