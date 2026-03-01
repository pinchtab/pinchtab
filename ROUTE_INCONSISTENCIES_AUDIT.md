# Routes & Documentation Inconsistencies Audit

**Date:** 2026-03-01  
**Status:** COMPREHENSIVE AUDIT COMPLETE  
**Current Commit:** `c27778f` (fix: integration tests and bug fixes)

---

## Executive Summary

**MAJOR INCONSISTENCIES FOUND:**

| Category | Issue | Severity | Count |
|----------|-------|----------|-------|
| Documentation | Routes shown NO LONGER EXIST | 🔴 CRITICAL | 6+ routes |
| Tests | Bridge tests don't match orchestrator implementation | 🟡 WARNING | 8+ test files |
| Dashboard | Uses old route patterns that might be broken | 🟡 WARNING | Needs verification |

---

## 1. CRITICAL: Documentation Shows Removed Routes

### showcase.md Examples Using REMOVED Routes

**File:** `docs/showcase.md`

The documentation shows these routes, but they DO NOT exist in the code:

```bash
❌ GET /instances/$INST/navigate          # REMOVED - use POST /tabs/{id}/navigate instead
❌ GET /instances/$INST/snapshot?tabId=$TAB  # REMOVED - use GET /tabs/{id}/snapshot
❌ GET /instances/$INST/text?tabId=$TAB  # REMOVED - use GET /tabs/{id}/text
❌ POST /instances/$INST/action          # REMOVED - use POST /tabs/{id}/action
```

### Lines in showcase.md with Bad Routes

| Line(s) | Route | Status | Fix |
|---------|-------|--------|-----|
| ~81, 99 | `/instances/$INST/navigate` | ❌ REMOVED | Use `/tabs/{id}/navigate` |
| ~99 | `/instances/$INST/text?tabId=$TAB` | ❌ REMOVED | Use `/tabs/{id}/text` |
| ~126, 166, 201 | `/instances/$INST/snapshot?tabId=$TAB` | ❌ REMOVED | Use `/tabs/{id}/snapshot` |
| ~194, 231, 294 | `/instances/$INST/action` | ❌ REMOVED | Use `/tabs/{id}/action` |
| ~270 | `/instances/$INST/snapshot?tabId=$TAB` | ❌ REMOVED | Use `/tabs/{id}/snapshot` |

### Evidence

**What code ACTUALLY has:**
```go
// orchestrator/handlers.go - Tab-centric routes that WORK:
mux.HandleFunc("POST /tabs/{id}/navigate", o.handleTabNavigate)
mux.HandleFunc("GET /tabs/{id}/snapshot", o.handleTabSnapshot)
mux.HandleFunc("GET /tabs/{id}/text", o.handleTabText)
mux.HandleFunc("POST /tabs/{id}/action", o.handleTabAction)

// orchestrator/handlers.go - Instance-scoped routes that DON'T EXIST for these:
// (no /instances/{id}/navigate)
// (no /instances/{id}/snapshot)
// (no /instances/{id}/text)
// (no /instances/{id}/action)
```

**What documentation claims:**
```bash
curl "http://localhost:9867/instances/inst_abc123/text?tabId=$TAB"
curl -X POST http://localhost:9867/instances/$INST/action
```

---

## 2. INCONSISTENCY: Test Files Use Different Route Patterns

### Bridge-Level Tests (snapshot_test.go, actions_test.go, navigate_test.go)
```go
// Use query parameter pattern
httpGet(t, "/snapshot?tabId=" + tabID)
httpPost(t, "/action", ...)
httpPost(t, "/navigate", ...)
```

✅ **These work** - They hit the bridge directly

### Orchestrator Tests (orchestrator_test.go)
```go
// Use path parameter pattern
httpGet(t, fmt.Sprintf("/tabs/%s/snapshot", tabID))
httpPost(t, fmt.Sprintf("/tabs/%s/action", tabID), ...)
httpPost(t, fmt.Sprintf("/tabs/%s/navigate", tabID), ...)
```

✅ **These work** - They hit the orchestrator

### PDF Tests (pdf_test.go)
```go
// Use path parameter pattern
httpGet(t, tabPDFPath(t, ""))  // Returns "/tabs/{id}/pdf"
```

✅ **These work** - They hit the orchestrator

### Problem
There's a **mismatch in test organization**:
- Some tests (snapshot, actions, navigate) run against **bridge directly** with `?tabId=` parameter
- Some tests (pdf, orchestrator) run against **orchestrator** with `/tabs/{id}/` path parameter
- This makes it unclear which is the "canonical" way to use the API

---

## 3. DASHBOARD Route Compatibility Check

### Dashboard Uses These Routes:
```bash
GET /profiles
POST /profiles
POST /profiles/create
DELETE /profiles/{name}
GET /profiles/{name}/analytics
GET /profiles/{name}/instance
POST /profiles/{id}/stop
GET /instances
POST /instances/launch
POST /instances/start
GET /instances/{id}/logs
POST /instances/{id}/stop
GET /instances/tabs
POST /instances/{id}/tabs/open
POST /instances/{id}/tab
GET /health
GET /stealth/status
GET /screencast/tabs
GET /tabs
```

### Verification
- ✅ All profile routes exist in `internal/profiles/handlers.go`
- ✅ All instance routes exist in `internal/orchestrator/handlers.go`
- ✅ All health/stealth routes exist in `internal/handlers/handlers.go`
- ✅ Dashboard appears functionally intact

**However:** Dashboard proxies to first running instance using old bridge-level routes like `/snapshot`, `/action`, etc. These still exist at bridge level, so it works.

---

## 4. ROUTE ORGANIZATION BY LAYER

### Bridge Level Routes (internal/handlers/handlers.go)
```
GET  /snapshot
GET  /screenshot
GET  /text
GET  /tabs
GET  /tabs/{id}/snapshot
GET  /tabs/{id}/screenshot
GET  /tabs/{id}/text
GET  /tabs/{id}/pdf
POST /navigate
POST /action
POST /actions
POST /evaluate
POST /tabs/{id}/navigate
POST /tabs/{id}/action
POST /tabs/{id}/actions
POST /tabs/{id}/evaluate
POST /tabs/{id}/pdf
... (and others)
```

**Status:** ✅ Both patterns exist at bridge level (with/without path params)

### Orchestrator Level Routes (internal/orchestrator/handlers.go)
```
GET  /tabs/{id}/snapshot
GET  /tabs/{id}/screenshot
GET  /tabs/{id}/text
GET  /tabs/{id}/pdf
GET  /tabs/{id}/download
GET  /tabs/{id}/cookies
POST /tabs/{id}/navigate
POST /tabs/{id}/action
POST /tabs/{id}/actions
POST /tabs/{id}/evaluate
POST /tabs/{id}/pdf
POST /tabs/{id}/lock
POST /tabs/{id}/unlock
POST /tabs/{id}/cookies
POST /tabs/{id}/upload
```

**Status:** ✅ Only path-param pattern exists (tab-centric)

### Instance-Level Routes (internal/orchestrator/handlers.go)
```
GET  /instances
GET  /instances/{id}
GET  /instances/{id}/logs
GET  /instances/{id}/tabs
GET  /instances/{id}/screencast
GET  /instances/{id}/proxy/screencast
POST /instances/launch
POST /instances/start
POST /instances/{id}/start
POST /instances/{id}/stop
POST /instances/{id}/tab
POST /instances/{id}/tabs/open
```

**Status:** ✅ These exist and work

---

## 5. Detailed Inconsistency List

### CRITICAL - Routes That Don't Exist But Are Documented

| Route Documented | Location | Actually Exists? | Should Use |
|------------------|----------|-----------------|------------|
| `GET /instances/{id}/navigate` | showcase.md lines 81 | ❌ NO | `POST /tabs/{id}/navigate` |
| `GET /instances/{id}/snapshot?tabId=` | showcase.md lines 99, 166, 201, 270 | ❌ NO | `GET /tabs/{id}/snapshot` |
| `GET /instances/{id}/text?tabId=` | showcase.md lines 99, 315 | ❌ NO | `GET /tabs/{id}/text` |
| `POST /instances/{id}/action` | showcase.md lines 194, 231, 294, 301, 308 | ❌ NO | `POST /tabs/{id}/action` |

### Routes That Work But Test Mix

| Test File | Uses | Bridge? | Orchestrator? | Status |
|-----------|------|--------|---------------|--------|
| `snapshot_test.go` | `/snapshot?tabId=` | ✅ Yes | ❌ No | Bridge-only tests |
| `actions_test.go` | `/action`, `/actions` | ✅ Yes | ❌ No | Bridge-only tests |
| `navigate_test.go` | `/navigate` | ✅ Yes | ❌ No | Bridge-only tests |
| `pdf_test.go` | `/tabs/{id}/pdf` | ❌ No | ✅ Yes | Orchestrator tests |
| `orchestrator_test.go` | `/tabs/{id}/*` | ❌ No | ✅ Yes | Orchestrator tests |

---

## 6. Test Coverage Gaps

### Tests That DON'T Exist for Tab-Centric Routes

| Endpoint | Bridge Test | Orchestrator Test | Gap |
|----------|------------|------------------|-----|
| `/tabs/{id}/navigate` | ❌ | ✅ orchestrator_test.go | ⚠️ Missing bridge test |
| `/tabs/{id}/snapshot` | ❌ | ✅ orchestrator_test.go | ⚠️ Missing bridge test |
| `/tabs/{id}/action` | ❌ | ✅ orchestrator_test.go | ⚠️ Missing bridge test |
| `/tabs/{id}/text` | ❌ | ✅ orchestrator_test.go | ⚠️ Missing bridge test |

### Tests Using Query Param Tab ID (OLD PATTERN)

| Endpoint | Query Param Pattern | Path Param Pattern | Status |
|----------|-------------------|-----------------|--------|
| Snapshot | ✅ `snapshot_test.go` | ❌ No | Old pattern tested, new not |
| Action | ✅ `actions_test.go` | ❌ No | Old pattern tested, new not |
| Navigate | ✅ `navigate_test.go` | ❌ No | Old pattern tested, new not |

---

## 7. Documentation Files to Review

### High Priority (Uses Removed Routes)
- ✅ `docs/showcase.md` - **HAS REMOVED ROUTES** - ~20+ bad examples

### Medium Priority (May Have Inconsistencies)
- `docs/get-started.md` - Should verify examples
- `docs/references/endpoints.md` - Should list actual routes
- `docs/references/api-structure.md` - Should reflect orchestrator/bridge split
- `docs/guides/common-patterns.md` - Should use correct routes
- `docs/guides/multi-instance.md` - Should use `/tabs/{id}/` pattern

### Low Priority (Less Critical)
- `docs/headless-vs-headed.md` - Conceptual, less code-specific
- `skill/pinchtab/references/api.md` - Mirror of main docs

---

## 8. Summary of Issues by Type

### Type A: Routes Don't Exist (CRITICAL)
- `/instances/{id}/navigate` - Used in showcase.md but doesn't exist
- `/instances/{id}/snapshot?tabId=` - Used in showcase.md but doesn't exist
- `/instances/{id}/text?tabId=` - Used in showcase.md but doesn't exist
- `/instances/{id}/action` - Used in showcase.md but doesn't exist

### Type B: Test Pattern Mismatch (WARNING)
- Bridge tests use `?tabId=` query param pattern
- Orchestrator tests use `/tabs/{id}/` path param pattern
- No bridge tests for `/tabs/{id}/` endpoints
- Unclear which is canonical

### Type C: Documentation Outdated (WARNING)
- showcase.md examples show removed routes
- Other guides may have inconsistent examples
- No clear statement about endpoint availability

### Type D: Dashboard Compatibility (INFO)
- Dashboard still works (uses proxying)
- But based on old bridge-level endpoints
- Could be refactored to use orchestrator endpoints

---

## 9. Recommended Actions (Priority Order)

### 🔴 URGENT (Breaks Docs)
1. **Fix showcase.md** - Replace all `/instances/{id}/...` routes with `/tabs/{id}/...`
   - Lines: ~81, 99, 126, 166, 194, 201, 231, 270, 294, 301, 308, 315

2. **Verify all other docs** - Check for same pattern
   - `docs/get-started.md`
   - `docs/guides/common-patterns.md`
   - `docs/guides/multi-instance.md`

### 🟡 IMPORTANT (Test Organization)
3. **Clarify test patterns** - Add comment explaining why tests split:
   - Bridge tests (query param) vs Orchestrator tests (path param)

4. **Add bridge tests for `/tabs/{id}/` endpoints** - Ensure both patterns tested

### 🟢 NICE-TO-HAVE (Polish)
5. **Add route availability matrix** to docs
   - Show which routes available where (bridge, orchestrator, both)

6. **Update API reference** - Document the two patterns explicitly

---

## 10. Proof of Issues

### Proof 1: showcase.md Line 81
```bash
# Current (BROKEN):
curl -X POST http://localhost:9867/instances/inst_abc123/navigate
# Route /instances/{id}/navigate does NOT exist in orchestrator/handlers.go

# Should Be:
curl -X POST http://localhost:9867/tabs/{tabId}/navigate
# Or to create new tab in instance:
curl -X POST http://localhost:9867/instances/{instId}/tabs/open
```

### Proof 2: Code Evidence
```go
// orchestrator/handlers.go - What actually exists:
mux.HandleFunc("POST /tabs/{id}/navigate", o.handleTabNavigate)

// NOT:
// mux.HandleFunc("POST /instances/{id}/navigate", ...)  // DOESN'T EXIST
```

### Proof 3: Test Evidence
```go
// snapshot_test.go uses:
httpGet(t, "/snapshot?tabId=" + currentTabID)  // Bridge route

// orchestrator_test.go uses:
httpGet(t, fmt.Sprintf("/tabs/%s/snapshot", tabID))  // Orchestrator route

// These are DIFFERENT patterns!
```

---

## Conclusion

| Issue | Count | Severity | Status |
|-------|-------|----------|--------|
| Removed routes in docs | 6+ | 🔴 CRITICAL | Needs fixing |
| Test pattern mismatch | 3 files | 🟡 WARNING | Needs clarification |
| Potential docs inconsistencies | 5 files | 🟡 WARNING | Needs verification |
| Dashboard issues | Unknown | 🟡 UNCERTAIN | Needs testing |

**Next Step:** Create fixes based on this audit before making changes.
