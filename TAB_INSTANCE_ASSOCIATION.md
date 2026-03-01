# Tab-Instance Association & `/tabs/{id}/` Routing

**Status:** ⚠️ **NOT IMPLEMENTED** - Global tab lookup needed for tab-centric routes  
**Impact:** Required before implementing `/tabs/{id}/snapshot`, `/tabs/{id}/pdf`, etc.

---

## Current Architecture

### How Tab IDs Are Scoped

**Tab IDs are INSTANCE-LOCAL, not globally unique:**

```
Instance A
├── tab_abc123  (local to Instance A)
├── tab_def456
└── tab_ghi789

Instance B
├── tab_abc123  (different tab, same ID prefix)
├── tab_xyz000
└── tab_jkl111
```

**This means:**
- Tab ID `tab_abc123` in Instance A ≠ Tab ID `tab_abc123` in Instance B
- Each bridge instance has its own TabManager with local tab registry
- There's no global mapping of tab IDs to instances

---

## Current Association Mechanism

### 1. When You Do `/instances/{id}/navigate`

```bash
POST /instances/{inst_abc}/navigate
→ Returns tabId: "tab_xyz123"
```

**Association is IMPLICIT:**
- Client knows instance ID from the request
- Client knows which tab was created
- No server-side mapping maintained

### 2. `GET /instances/tabs` Returns Full Association

```json
[
  {
    "id": "tab_abc123",      ← Tab ID (instance-local)
    "instanceId": "inst_xyz",  ← Instance ID (explicit)
    "url": "https://example.com",
    "title": "Example"
  }
]
```

**Association IS explicit here** - orchestrator queries all instances and builds the map

### 3. Current Operations Flow

```
Browser Operation (e.g., snapshot):
┌─────────────────────────────────────┐
│ GET /instances/{inst}/snapshot      │
│ ?tabId={tab}                        │
└──────────────────┬──────────────────┘
                   │
                   ├─ Instance ID: explicit in URL ✓
                   └─ Tab ID: query parameter ✓
                   
                   ↓
            Both provided by client
            Server doesn't need to resolve
```

---

## Problem: `/tabs/{id}/snapshot`

### What's Missing

```
GET /tabs/{tab_id}/snapshot
        ▲
        └─ Only have tab ID!
          Need to know: which instance owns this?
```

**The resolver problem:**

| Route | Tab ID | Instance ID | Need Resolver? |
|-------|--------|-------------|----------------|
| `GET /instances/{inst}/snapshot?tabId={tab}` | ✓ query param | ✓ path param | ❌ No |
| `GET /tabs/{id}/snapshot` | ✓ path param | ❌ MISSING | ✅ **Yes** |

---

## Solution: TabResolver

### Architecture

```
┌──────────────────────────────────────────────┐
│         Orchestrator (9867)                  │
│                                              │
│  ┌─────────────────────────────────────┐   │
│  │ TabResolver (NEW)                   │   │
│  │                                     │   │
│  │ tabs = GET /instances/tabs (cached) │   │
│  │ tabToInstance = {}                  │   │
│  │                                     │   │
│  │ ResolveTab(tabId) → instanceId      │   │
│  │ ResolveInstance(tabId) → instanceId │   │
│  └─────────────────────────────────────┘   │
│                                              │
│  Endpoints:                                 │
│  GET /tabs/{id}/snapshot                   │
│  GET /tabs/{id}/pdf                        │
│  GET /tabs/{id}/screenshot                 │
│         ↓                                   │
│    1. TabResolver.ResolveTab(id)           │
│    2. Get instance ID                      │
│    3. Proxy to instance                    │
└──────────────────────────────────────────────┘
         ↓
┌──────────────────────────────────────────────┐
│ Instance (9868+)                            │
│ - /snapshot                                 │
│ - /pdf                                      │
│ - /screenshot                               │
└──────────────────────────────────────────────┘
```

### Implementation Plan (from TODO.md)

```go
// Phase 2: Add Tab ID Resolution Layer

// Create TabResolver interface
interface TabResolver {
    ResolveTab(tabId string) (instanceId string, tabHandle string, error)
    ResolveInstance(tabId string) (instanceId string, error)
}

// Implement in orchestrator
type OrchestratorTabResolver struct {
    tabToInstance map[string]string  // tabId → instanceId
    mu            sync.RWMutex
}

func (r *OrchestratorTabResolver) ResolveTab(tabId string) (string, string, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    instanceId, ok := r.tabToInstance[tabId]
    if !ok {
        return "", "", fmt.Errorf("tab %q not found", tabId)
    }
    return instanceId, tabId, nil
}

// Refresh mapping periodically
func (o *Orchestrator) RefreshTabMapping() {
    all := o.AllTabs()
    r.mu.Lock()
    defer r.mu.Unlock()
    
    for _, tab := range all {
        r.tabToInstance[tab.ID] = tab.InstanceID
    }
}
```

---

## Current Data Structures

### InstanceTab (Orchestrator Level)

```go
type InstanceTab struct {
    ID         string  // tab_XXXXXXXX (instance-local)
    InstanceID string  // inst_XXXXXXXX (LINKS to instance)
    URL        string
    Title      string
}
```

✅ **Has the association** (InstanceID field)

### Tab Entry (Bridge Level)

```go
type TabEntry struct {
    CDPID string           // Chrome DevTools Protocol target ID
    Ctx   context.Context  // Tab context
    // No InstanceID field (not needed at bridge level)
}
```

❌ **No instance reference** (bridge only knows about its own tabs)

---

## How It Would Work

### Step 1: Build Mapping (Orchestrator Startup)

```go
// On /instances/tabs or periodically:
type tabToInstance = {
    "tab_abc123": "inst_xyz",
    "tab_def456": "inst_xyz",
    "tab_ghi789": "inst_uvw",
}
```

### Step 2: Handle `/tabs/{id}/snapshot`

```bash
GET /tabs/tab_abc123/snapshot

Orchestrator:
1. tabResolver.ResolveTab("tab_abc123")
   → returns ("inst_xyz", nil)
2. Get instance URL from map
   → "http://localhost:9868"
3. Proxy to instance:
   → GET http://localhost:9868/snapshot?tabId=tab_abc123
4. Return response
```

### Step 3: Handle Tab Closure (Keep Mapping Fresh)

```
When tab closes:
1. Bridge removes from local TabManager.tabs
2. Next /instances/tabs refresh removes from mapping
3. Requests to closed tabs return 404

Alternative: Use POST /tab/close to notify orchestrator
```

---

## Alternative Approaches

### Option A: Hash-Based Global Tab ID (Complex)
- Make tab IDs globally unique by including instance info
- Tab ID format: `tab_{instance_hash}_{local_id}`
- Pro: No central mapping needed
- Con: Leaks instance info, breaks client simplicity

### Option B: Caching + TTL (Current Plan)
- Cache tab→instance mapping at orchestrator
- Refresh from `/instances/tabs` periodically (e.g., every 5-10 seconds)
- Pro: Simple, doesn't require bridge changes
- Con: Short window of stale data (acceptable)

### Option C: Event-Driven Updates (Advanced)
- Bridge notifies orchestrator on tab open/close
- Orchestrator maintains real-time mapping
- Pro: Always accurate
- Con: Requires webhook/pub-sub architecture

---

## Implementation Steps

### Phase 1: Add TabResolver to Orchestrator
```go
type Orchestrator struct {
    // ... existing fields
    tabResolver *TabResolver
}

// In NewOrchestrator()
o.tabResolver = NewTabResolver(o.AllTabs)

// Refresh periodically
go func() {
    ticker := time.NewTicker(5 * time.Second)
    for range ticker.C {
        o.tabResolver.Refresh()
    }
}()
```

### Phase 2: Add `/tabs/{id}/` Handlers
```go
mux.HandleFunc("GET /tabs/{id}/snapshot", o.handleTabSnapshot)
mux.HandleFunc("GET /tabs/{id}/pdf", o.handleTabPDF)
mux.HandleFunc("GET /tabs/{id}/screenshot", o.handleTabScreenshot)
// etc.
```

### Phase 3: Implement Handlers
```go
func (o *Orchestrator) handleTabSnapshot(w http.ResponseWriter, r *http.Request) {
    tabId := r.PathValue("id")
    
    // Resolve: tabId → instanceId
    instanceId, err := o.tabResolver.ResolveInstance(tabId)
    if err != nil {
        web.Error(w, 404, err)
        return
    }
    
    // Get instance URL
    inst := o.instances[instanceId]
    if inst == nil {
        web.Error(w, 404, fmt.Errorf("instance not found"))
        return
    }
    
    // Proxy to instance
    proxyRequest(w, r, inst.URL + "/snapshot?tabId=" + tabId)
}
```

---

## Migration Path

### Current State
- Tests use: `GET /instances/{id}/snapshot?tabId={tab}`
- Dashboard uses: Instance-based navigation
- Tab ID association: Implicit (client-side knowledge)

### After Tab-Centric Implementation
- New route: `GET /tabs/{id}/snapshot`
- Old route: Still works (backward compat)
- Tab ID association: Maintained by TabResolver
- Tests can use either pattern

---

## Questions to Resolve

1. **Cache Refresh Rate:** 5s? 10s? Event-driven?
2. **TTL on Tab Closure:** How long to keep closed tabs in mapping?
3. **Instance Down Handling:** If instance crashes, what happens to its tabs in mapping?
4. **Multiple Instances Same Tab ID:** Already handled (ID is instance-local)
5. **Performance:** Scale to 100+ tabs? 1000+ tabs?

---

## Summary

| Aspect | Current | With `/tabs/{id}/` |
|--------|---------|-------------------|
| Tab→Instance | Implicit (client knows) | Explicit (TabResolver) |
| Data Stored | Per-instance TabManager | Orchestrator mapping |
| Update Frequency | Never (until needed) | Periodic refresh |
| Routing Complexity | Simple (ID explicit in URL) | Medium (requires lookup) |
| Backward Compat | N/A | Old routes still work |
| Implementation Effort | N/A | Medium (50-100 LOC) |

**Bottom Line:** Tab-to-instance association IS available (in `InstanceTab`), but we need an **orchestrator-level resolver** to enable `/tabs/{id}/` routing. This is documented in TODO.md as Phase 2.
