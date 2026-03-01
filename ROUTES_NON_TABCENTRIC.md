# Routes NOT Using /tabs/{id}/ Pattern

**Last Updated:** 2026-03-01  
**Status:** All 202 tests use non-tab-centric routes; dashboard uses mix of instance and bridge routes  
**Goal:** Document current state before tab-centric refactoring

---

## Dashboard Routes (internal/dashboard/dashboard/*.js)

### Profile Management
```
GET     /profiles
POST    /profiles/create
POST    /profiles/import
DELETE  /profiles/{name}
PATCH   /profiles/{name}
GET     /profiles/{name}/analytics
GET     /profiles/{name}/instance
POST    /profiles/{id}/stop
```

### Instance Management
```
POST    /instances/launch
GET     /instances
GET     /instances/{id}/logs
POST    /instances/{id}/stop
```

### Tab Aggregation (NOT tab-centric)
```
GET     /instances/tabs          # Instance port aggregation
GET     /tabs                    # Global tab list
GET     /screencast/tabs
```

### Utility
```
GET     /health
GET     /dashboard/agents
GET     /stealth/status
```

---

## Integration Tests (tests/integration/*.go)

### Instance/Orchestrator Tests
Files: `orchestrator_test.go`
```
POST    /instances/launch
POST    /instances/{id}/navigate
GET     /instances/{id}
GET     /instances/{id}/snapshot
POST    /instances/{id}/stop
GET     /instances/tabs
```

### Browser Operations (Bridge-scoped)
Files: `snapshot_test.go`, `screenshot_test.go`, `pdf_test.go`, `actions_test.go`, `tabs_test.go`

#### Navigation & Snapshot
```
POST    /navigate
GET     /snapshot
GET     /snapshot?filter=interactive
GET     /snapshot?format=text
GET     /snapshot?format=yaml
GET     /snapshot?depth=2
GET     /snapshot?maxTokens=500
GET     /snapshot?tabId={tabId}
GET     /snapshot?output=file
GET     /snapshot?diff=true
GET     /snapshot?selector={selector}
```

#### Screenshots
```
GET     /screenshot
GET     /screenshot?raw=true
```

#### PDF Export
```
GET     /pdf
GET     /pdf?raw=true
GET     /pdf?output=file
GET     /pdf?landscape=true
GET     /pdf?tabId={tabId}
GET     /pdf?displayHeaderFooter=true
GET     /pdf?scale={scale}
```

#### Tab Management
```
GET     /tabs
POST    /tab
POST    /tab/lock
POST    /tab/unlock
POST    /action
GET     /text
```

#### File Operations
```
POST    /navigate (with file:// URLs)
GET     /download
POST    /upload
```

### Stealth & Error Handling
Files: `stealth_test.go`, `error_handling_test.go`
```
GET     /tabs                    # Stealth mode verification
POST    /navigate                # Error handling tests
GET     /snapshot
```

---

## Route Categories

### 1. Instance-Scoped (/instances/{id}/*)
Used in **orchestrator** layer (dashboard, tests)
```
POST    /instances/launch
POST    /instances/{id}/navigate
GET     /instances/{id}
GET     /instances/{id}/snapshot
GET     /instances/{id}/logs
POST    /instances/{id}/stop
GET     /instances/tabs          # Aggregates tabs from all running instances
```

**Status:** ✅ These routes are intentionally instance-scoped (not tab-scoped)

### 2. Bridge-Scoped (no /instances/ prefix)
Used in **bridge** layer (direct browser control)
```
POST    /navigate
GET     /snapshot
GET     /screenshot
GET     /text
GET     /tabs
POST    /tab
POST    /tab/lock
POST    /tab/unlock
POST    /action
GET     /pdf
```

**Status:** ⚠️ These should evolve to `/tabs/{id}/` pattern

### 3. Tab-Centric (/tabs/{id}/*)
Expected routes (not yet implemented)
```
GET     /tabs/{id}/snapshot
GET     /tabs/{id}/screenshot
GET     /tabs/{id}/pdf
GET     /tabs/{id}/text
POST    /tabs/{id}/action
POST    /tabs/{id}/navigate
GET     /tabs/{id}/cookies
POST    /tabs/{id}/evaluate
```

**Status:** ❌ None currently in use

### 4. Profile-Scoped (/profiles/*)
```
GET     /profiles
POST    /profiles
POST    /profiles/create
POST    /profiles/import
DELETE  /profiles/{name}
GET     /profiles/{name}/analytics
GET     /profiles/{name}/instance
POST    /profiles/{id}/stop
```

**Status:** ✅ Profile-specific routes (orthogonal to tab-centric refactoring)

---

## Query Parameter Tab ID Usage

Current pattern: Tab ID passed as query parameter
```bash
# Current approach (bridge-scoped with ?tabId=)
GET /snapshot?tabId={tabId}&filter=interactive
GET /pdf?tabId={tabId}&landscape=true
GET /screenshot?tabId={tabId}

# Future approach (tab-centric path param)
GET /tabs/{tabId}/snapshot?filter=interactive
GET /tabs/{tabId}/pdf?landscape=true
GET /tabs/{tabId}/screenshot
```

**Impact:** All 202 tests use `?tabId=` pattern; migration requires:
1. Update bridge handlers to accept `/tabs/{id}/` routes
2. Update test files to use new route format
3. Update dashboard to use new route format
4. Optionally: Keep old routes for backward compatibility via shims

---

## Transition Impact Analysis

| Layer | Routes | Count | Impact | Effort |
|-------|--------|-------|--------|--------|
| Tests | Bridge-scoped | ~40+ | Highest | Medium |
| Dashboard | Instance + Bridge | ~30+ | High | Medium |
| Docs | All patterns | ~20+ | Medium | Low |
| **Total** | **~90+ routes** | | | |

### Files to Update for Tab-Centric Migration

1. **Test Files** (~15 files)
   - `snapshot_test.go`
   - `screenshot_test.go`
   - `pdf_test.go`
   - `actions_test.go`
   - `tabs_test.go`
   - Others

2. **Dashboard JS** (~5 files)
   - `profiles.js`
   - `instances.js`
   - `screencast.js`
   - `settings.js`
   - `app.js`

3. **Documentation** (~10 files)
   - `showcase.md`
   - `common-patterns.md`
   - API reference docs
   - Examples

4. **Handler Code**
   - `internal/handlers/` - Add `/tabs/{id}/` route handlers
   - `internal/orchestrator/` - Proxy tab routes if needed

---

## Decision Points

**Before migrating to `/tabs/{id}/` pattern:**

1. **Backward Compatibility:** Keep old routes as shims or remove?
2. **Orchestrator Layer:** Should `/instances/{id}/tabs/{tabId}/snapshot` exist?
3. **Routing:** How deep should proxy routing go?
4. **Query Params:** Keep optional `?tabId=` for fallback?

---

## Quick Reference

### Most Common Routes (Priority 1)
```
GET     /snapshot
GET     /screenshot
GET     /pdf
GET     /tabs
POST    /navigate
POST    /action
```

### Medium Priority (Priority 2)
```
GET     /text
POST    /tab
POST    /tab/lock
GET     /instances/tabs
```

### Lower Priority (Priority 3)
```
GET     /download
POST    /upload
POST    /evaluate
GET     /cookies
```

---

**Note:** This file documents the current state. Keep it updated as routes are refactored to the tab-centric `/tabs/{id}/` pattern.
