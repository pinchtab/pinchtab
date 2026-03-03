# Allocation Strategies Implementation Plan

**Status:** Architecture finalized 2026-03-03  
**Related:** PR #91 (feat/allocation-strategies)  
**Author:** Mario (implementation), Bosch (architecture)

---

## Executive Summary

This plan implements **two independent systems** that work together:

1. **Allocation Strategy** (API style): Simple/Session/Explicit
2. **Instance Allocation Policy** (instance selection): FCFS/RoundRobin/LoadBased

Currently PR #91 has the Strategy framework but is missing:
- InstanceManager (adapter layer between Orchestrator ↔ Bridge)
- AllocationPolicy (swappable instance selection)
- Primitives implementation
- Integration with Orchestrator
- Proper Instance + Profile + Tab relationships

---

## Architecture Overview (DECOMPOSED - Facade Pattern)

```
Agent Request
  ↓
Orchestrator (coordinator)
  ├─ Picks AllocationStrategy based on config
  └─ Routes to strategy
  ↓
AllocationStrategy (Simple/Session/Explicit)
  ├─ Handles API endpoints
  └─ Uses InstanceManager facade methods
  ↓
InstanceManager (thin facade - ZERO business logic, just orchestrates)
  │
  ├─ TabService (all tab operations)
  │   ├─ CreateTab(url) → calls allocator → repository → bridge
  │   ├─ CloseTab(tabID)
  │   └─ ListTabs()
  │
  ├─ InstanceAllocator (applies AllocationPolicy)
  │   ├─ AllocateInstance() → policy.SelectInstance(instances)
  │   └─ SelectForNewTab()
  │
  ├─ TabLocator (discovery + cache)
  │   ├─ FindInstanceByTabID(tabID) → cache OR query bridges
  │   ├─ Invalidate(tabID)
  │   └─ RefreshAll()
  │
  ├─ InstanceRepository (instance lifecycle)
  │   ├─ Launch(profileID, port) → new Instance + Bridge
  │   ├─ Stop(instanceID)
  │   ├─ List() → []*Instance
  │   └─ Get(instanceID) → *Instance
  │
  └─ RequestRouter (HTTP proxying only)
      └─ ProxyTabRequest(w, r, tabID) → locator → proxy
  ↓
Bridge (thin wrapper - one per instance)
  ├─ TabRegistry (local ownership)
  │   └─ GET /tab-registry endpoint
  │   └─ {tabId → {url, createdAt}}
  ├─ TabManager (wraps CDP)
  └─ chromedp.Run(ctx, ...) → CDP operations

AllocationPolicy (INJECTED - SWAPPABLE)
  ├─ FCFS: first available
  ├─ RoundRobin: rotate through instances
  ├─ LoadBased: least loaded (tabs + memory weighted)
  └─ Random: random selection
```

---

## Component Responsibilities

### Orchestrator
**Owns:** Strategy lifecycle, high-level routing  
**Delegates to:** InstanceManager for all instance/tab operations  
**Does NOT own:** Instance allocation decisions, tab discovery

```go
type Orchestrator struct {
  instanceMgr InstanceManager
  strategy    strategy.Strategy
  config      *Config
}

func (o *Orchestrator) proxyTabRequest(w, r) {
  tabID := r.PathValue("id")
  o.instanceMgr.ProxyTabRequest(w, r, tabID)  // Delegate!
}
```

### InstanceManager (Facade Pattern)
**Owns:** NOTHING - pure orchestration only  
**Delegates to:** 5 focused components (see below)  
**Used by:** Strategies, Orchestrator

**Public interface (what Orchestrator + Strategies see):**
```go
type InstanceManager interface {
  // Lifecycle (delegates to Repository)
  Launch(profileId string, port int) (*Instance, error)
  Stop(instanceId string) error
  List() []*Instance
  Get(instanceId string) (*Instance, error)
  
  // Tab discovery (delegates to Locator)
  FindInstanceByTabID(tabID string) (*Instance, error)
  
  // Allocation (delegates to Allocator)
  AllocateInstance() (*Instance, error)
  
  // Operations (delegates to TabService)
  CreateTab(url string) (string, error)
  CloseTab(tabID string) error
  ProxyTabRequest(w, r, tabID string) error
  
  // Caching (delegates to Locator)
  InvalidateTabCache(tabID string)
}
```

**Implementation is pure delegation:**
```go
type InstanceManager struct {
  repo      InstanceRepository
  allocator InstanceAllocator
  locator   TabLocator
  tabs      TabService
  router    RequestRouter
}

func (m *InstanceManager) CreateTab(url string) (string, error) {
  return m.tabs.CreateTab(url)  // delegates
}

func (m *InstanceManager) FindInstanceByTabID(tabID string) (*Instance, error) {
  return m.locator.FindInstanceByTabID(tabID)  // delegates
}

func (m *InstanceManager) ProxyTabRequest(w, r, tabID) error {
  return m.router.ProxyTabRequest(w, r, tabID)  // delegates
}

// ... all methods just forward to appropriate component
```

### InstanceManager Components (Decomposed)

#### InstanceRepository
**Owns:** Instance lifecycle, in-memory store  
**Responsible for:** Launch, Stop, List, Get, Add, Remove  
**Dependencies:** None
```go
type InstanceRepository interface {
  Launch(profileID string, port int) (*Instance, error)
  Stop(instanceID string) error
  List() []*Instance
  Get(instanceID string) (*Instance, error)
  Add(instance *Instance) error
  Remove(instanceID string) error
}
```

#### InstanceAllocator
**Owns:** Applying AllocationPolicy to select an instance  
**Responsible for:** AllocateInstance, SelectForNewTab  
**Dependencies:** InstanceRepository, AllocationPolicy (injected)
```go
type InstanceAllocator interface {
  AllocateInstance() (*Instance, error)
  SelectForNewTab() (*Instance, error)
}
```

#### TabLocator
**Owns:** Discovering which instance owns a tab + caching + invalidation  
**Responsible for:** FindInstanceByTabID (O(1) cache, O(n) fallback), Invalidate, RefreshAll  
**Dependencies:** InstanceRepository
```go
type TabLocator interface {
  FindInstanceByTabID(tabID string) (*Instance, error)  // Cache OR query bridges
  Invalidate(tabID string)
  RefreshAll()
}
```

#### TabService
**Owns:** All tab-level operations  
**Responsible for:** CreateTab, CloseTab, ListTabs  
**Dependencies:** Allocator, Locator, Repository (indirectly via allocator)
```go
type TabService interface {
  CreateTab(url string) (string, error)
  CloseTab(tabID string) error
  ListTabs(instanceID string) ([]*Tab, error)
}
```

#### RequestRouter
**Owns:** HTTP proxying logic only  
**Responsible for:** ProxyTabRequest  
**Dependencies:** TabLocator
```go
type RequestRouter interface {
  ProxyTabRequest(w http.ResponseWriter, r *http.Request, tabID string) error
}
```

### Bridge
**Owns:** Tabs (creates, manages, closes)  
**Exposes:** TabRegistry via `GET /tab-registry` endpoint  
**Does NOT own:** Which instance a tab belongs to (discovered by others)

```go
type Bridge struct {
  tabManager *TabManager
}

func (b *Bridge) GetTabRegistry() *TabRegistry {
  return b.tabManager.registry
}

// HTTP endpoint for Orchestrator discovery
func (b *Bridge) handleTabRegistry(w, r) {
  registry := b.GetTabRegistry()
  web.JSON(w, 200, registry.ExportJSON())
}

// On create/close: registry is updated automatically
func (tm *TabManager) CreateTab(url string) (string, error) {
  tabID := tm.generateID()
  // ... CDP ops ...
  tm.registry.Register(TabID: tabID, URL: url, CreatedAt: now)
  return tabID, nil
}
```

### AllocationPolicy (SWAPPABLE)
**Owns:** Logic for selecting which instance gets the next tab  
**Used by:** InstanceManager.AllocateInstance()  
**Config-driven:** Can be swapped via pinchtab.yaml

```go
type AllocationPolicy interface {
  SelectInstance(instances []*Instance) (*Instance, error)
}

// Built-in implementations
- FCFSPolicy: first running
- RoundRobinPolicy: cycle through (maintains index)
- LoadBasedPolicy: least loaded (tabs + memory)
- RandomPolicy: random selection
```

### Strategies (Simple, Session, Explicit)
**Own:** API endpoint definitions, request handling  
**Use:** InstanceManager for all operations  
**DO NOT own:** Instance selection, tab discovery

```go
type SimpleStrategy struct {
  instanceMgr    InstanceManager
  currentTabID   string  // Just remember the last one
}

func (s *SimpleStrategy) handleNavigate(w, r) {
  var req struct{ URL string }
  json.Decode(r.Body, &req)
  
  // Allocate instance (policy decides)
  tabID, _ := s.instanceMgr.CreateTab(req.URL)
  s.currentTabID = tabID
  
  web.JSON(w, 200, map[string]string{"tabId": tabID})
}

func (s *SimpleStrategy) handleSnapshot(w, r) {
  // Use remembered current tab
  s.instanceMgr.ProxyTabRequest(w, r, s.currentTabID)
}
```

---

## Tab → Instance Discovery Flow

**Problem:** Orchestrator needs to know which instance owns a tab for routing

**Current (inefficient):**
```
proxyTabRequest(tabID)
  ├─ Query Bridge 1: GET /instances/1/tabs → no match
  ├─ Query Bridge 2: GET /instances/2/tabs → FOUND!
  └─ O(n×m) complexity
```

**New (efficient):**
```
proxyTabRequest(tabID)
  ↓
InstanceManager.FindInstanceByTabID(tabID)
  ├─ Check tabCache[tabID] → O(1) hit
  ├─ If miss:
  │   ├─ Query Bridge 1: GET /tab-registry → {tab_abc, tab_def}
  │   ├─ Query Bridge 2: GET /tab-registry → {tab_xyz} ← FOUND!
  │   └─ Cache: tabCache[tabID] = instanceId
  └─ Return instance
  ↓
proxy to instance.bridge
```

**Cache invalidation:** Automatic on tab close

---

## Configuration

```yaml
# pinchtab.yaml
orchestrator:
  # Allocation strategy (API style)
  strategy: simple  # simple | session | explicit
  
  # Instance allocation policy (instance selection)
  allocation_policy: round_robin  # fcfs | round_robin | load_based | random
  
  # Optional policy settings
  load_based:
    weight_tabs: 0.6      # 60% weight on tab count
    weight_memory: 0.4    # 40% weight on memory

# Environment variable override
export PINCHTAB_STRATEGY=simple
export PINCHTAB_ALLOCATION_POLICY=load_based
```

---

## Current State vs Target State

### Current (PR #91 + main)
```
✅ Strategy framework exists
✅ Explicit strategy implemented
✅ Session strategy scaffolded
❌ NO InstanceManager
❌ NO AllocationPolicy
❌ NO primitives implementation
❌ NO Bridge.TabRegistry exposure
❌ NO integration with Orchestrator
❌ Simple strategy NOT implemented
❌ Dashboard deleted (needs rebase)
```

### Target State (After Implementation)
```
✅ Strategy framework (existing)
✅ InstanceManager (new adapter layer)
✅ AllocationPolicy (swappable policies)
✅ Primitives implementation
✅ Bridge.TabRegistry exposed
✅ Orchestrator uses InstanceManager
✅ All 3 strategies working (Simple, Session, Explicit)
✅ Config-driven allocation policy
✅ Tab cache optimization
```

---

## Implementation Steps

### Phase 1: Rebase & Setup (1-2 hours)

**1.1 Rebase PR #91 onto main**
```bash
git fetch origin
git rebase origin/main feat/allocation-strategies
# Resolve dashboard conflicts (restore from main)
git push origin feat/allocation-strategies --force-with-lease
```

**Status:** CI passes, no conflicts

### Phase 2: InstanceManager (Decomposed) (5-7 hours)

**2.1 Create Bridge.TabRegistry exposure**
- File: `internal/bridge/tab_registry.go` (new)
- Define `TabRegistry` struct: `map[string]*TabEntry{tabID → {url, createdAt}}`
- Add to Bridge: `func (b *Bridge) GetTabRegistry() *TabRegistry`
- Add HTTP endpoint: `GET /tab-registry` in handlers

**2.2 Create InstanceRepository (lifecycle)**
- File: `internal/instance/repository.go`
- Manages instances in-memory map
- Implement: Launch, Stop, List, Get, Add, Remove
- Thread-safe (sync.RWMutex)

**2.3 Create TabLocator (discovery + cache)**
- File: `internal/instance/locator.go`
- Owns `map[string]string` cache: tabID → instanceID
- Implement: FindInstanceByTabID (cache OR query bridges)
- Implement: Invalidate, RefreshAll
- Thread-safe caching

**2.4 Create InstanceAllocator (policy application)**
- File: `internal/instance/allocator.go`
- Holds AllocationPolicy (injected)
- Implement: AllocateInstance(), SelectForNewTab()
- Delegates to policy.SelectInstance(instances)

**2.5 Create TabService (tab operations)**
- File: `internal/instance/tabservice.go`
- Implement: CreateTab(url), CloseTab(tabID), ListTabs(instanceID)
- CreateTab: allocate → repository.Get → bridge.CreateTab
- CloseTab: locator.Invalidate + bridge.CloseTab

**2.6 Create RequestRouter (proxying)**
- File: `internal/instance/router.go`
- Implement: ProxyTabRequest(w, r, tabID)
- Uses TabLocator.FindInstanceByTabID → proxy HTTP

**2.7 Create InstanceManager facade**
- File: `internal/instance/manager.go`
- Struct holds: repo, allocator, locator, tabs, router
- All public methods just delegate to components
- `func (m *InstanceManager) CreateTab(url) = m.tabs.CreateTab(url)`
- `func (m *InstanceManager) FindInstanceByTabID(id) = m.locator.FindInstanceByTabID(id)`

**2.8 Create types**
- File: `internal/instance/types.go`
- Instance, TabEntry, TabRegistry

**2.9 Wire into Orchestrator**
- File: `internal/orchestrator/orchestrator.go`
- Create InstanceManager in `NewOrchestrator()`
- Update `proxyTabRequest` to use InstanceManager
- Test: old flow should still work

**Tests:**
- `internal/instance/instance_test.go` (integration)
- `TestInstanceRepository_Launch`
- `TestTabLocator_FindsInstanceByTabID_CacheHit`
- `TestTabLocator_FindsInstanceByTabID_CacheMiss`
- `TestInstanceAllocator_AppliesPolicy`
- `TestTabService_CreateTab_AllocatesViaPolicy`
- `TestRequestRouter_ProxiesTabRequest`
- `TestInstanceManager_Facade_DelegatesToComponents`

### Phase 3: AllocationPolicy (2-3 hours)

**3.1 Create AllocationPolicy interface**
- File: `internal/allocation/policy.go`
- Define interface: `SelectInstance(instances) *Instance`

**3.2 Implement built-in policies**
- File: `internal/allocation/fcfs.go` - first available
- File: `internal/allocation/round_robin.go` - cycle
- File: `internal/allocation/load_based.go` - least loaded
- File: `internal/allocation/random.go` - random

**3.3 Policy factory**
- File: `internal/allocation/factory.go`
- `NewPolicy(name string) AllocationPolicy`
- Support config-driven selection

**3.4 Integrate policy into InstanceManager**
- Add `allocationPolicy AllocationPolicy` field
- Use in `AllocateInstance()`
- Pass to `CreateTab()`

**Tests:**
- `TestFCFS_SelectsFirstRunning`
- `TestRoundRobin_Cycles`
- `TestLoadBased_SelectsLeast`
- `TestPolicyFactory_CreatesCorrectPolicy`

### Phase 4: Simple Strategy (3-4 hours)

**4.1 Create Simple strategy**
- File: `internal/strategy/simple/simple.go`
- Has InstanceManager reference
- Tracks currentTabID
- Implements strategy.Strategy interface

**4.2 Implement shorthand endpoints**
- `POST /navigate` - create new tab
- `GET /snapshot` - get current tab's snapshot
- `POST /action` - action on current tab
- `POST /evaluate` - eval on current tab
- `GET /text` - text from current tab
- `GET /screenshot` - screenshot current tab
- `POST /tab/lock`, `POST /tab/unlock`
- `GET /cookies`, `POST /cookies`
- etc.

**4.3 Register strategy**
- In `init()`: `strategy.MustRegister("simple", ...)`

**Tests:**
- `TestSimple_Navigate_CreatesNewTab`
- `TestSimple_Snapshot_UsesCurrentTab`
- `TestSimple_RespaysAllocationPolicy`
- `TestSimple_RemembersCurrentTab`

### Phase 5: Session Strategy Refinement (2-3 hours)

**5.1 Update Session strategy to use InstanceManager**
- Replace any direct orchestrator calls
- Use `instanceMgr.CreateTab()` with policy
- Use `instanceMgr.ProxyTabRequest()`

**5.2 Session-specific allocation**
- Optionally override policy per session
- E.g., session1 uses FCFS, session2 uses RoundRobin

**Tests:**
- `TestSession_UsesAllocationPolicy`
- `TestSession_ManagesLifecycle`

### Phase 6: Orchestrator Integration (2-3 hours)

**6.1 Update proxyTabRequest**
- Already started in Phase 2.5
- Ensure all proxying goes through InstanceManager
- Remove `findRunningInstanceByTabID` (replaced by cache)

**6.2 Test orchestrator routing**
- Existing integration tests should pass
- Add: `TestOrchestrator_ProxyTabRequest_UsesCache`
- Add: `TestOrchestrator_ProxyTabRequest_QueuesRegistry`

**6.3 Profile integration**
- Ensure profiles work with strategies
- Test: launch profile → creates instance → applies strategy

### Phase 7: Config & CLI (2-3 hours)

**7.1 Add config fields**
- File: `internal/config/config.go`
- Add: `Strategy string`
- Add: `AllocationPolicy string`
- Add: `AllocationPolicyOptions map[string]any`

**7.2 Parse from YAML + env vars**
- `strategy: simple` from config
- `PINCHTAB_STRATEGY=session` env var override
- Same for allocation_policy

**7.3 Initialize strategy in orchestrator**
```go
policy := allocation.NewPolicy(cfg.AllocationPolicy)
strategy := strategy.New(cfg.Strategy, instanceMgr, policy)
```

**7.4 CLI support (optional)**
- `pinchtab --strategy simple`
- `pinchtab --allocation-policy round_robin`

### Phase 8: Testing & Polish (3-4 hours)

**8.1 Integration tests**
- Full workflow: `launch profile → create instance → apply strategy → allocate tab → proxy request`
- All 3 strategies with each policy
- Error cases

**8.2 Benchmarks**
- Cache hit/miss performance
- Policy selection overhead
- Registry lookup time

**8.3 Documentation**
- Update API docs for shorthand endpoints
- Document allocation policies
- Add examples

**8.4 Cleanup**
- Remove old `findRunningInstanceByTabID` entirely
- Remove dead code from old search flow
- Update comments

---

## Testing Strategy

### Unit Tests
- AllocationPolicy: each policy selects correctly
- InstanceManager: lifecycle, caching, discovery
- Bridge.TabRegistry: register/unregister/find
- Strategies: endpoint routing, current tab tracking

### Integration Tests
- Full workflow: Orchestrator → InstanceManager → Bridge → CDP
- proxyTabRequest with cache
- All strategies with all policies
- Error handling (instance down, tab not found, etc.)

### End-to-End
- Launch instance via dashboard
- Use Simple strategy (shorthand API)
- Create tabs, navigate, snapshot
- Verify allocation policy is respected

---

## Files to Create/Modify

### Create
```
internal/instance/manager.go              # Facade (pure delegation)
internal/instance/repository.go           # Lifecycle (Launch, Stop, List, Get)
internal/instance/locator.go              # Discovery + cache (FindInstanceByTabID)
internal/instance/allocator.go            # Policy application (AllocateInstance)
internal/instance/tabservice.go           # Tab operations (CreateTab, CloseTab)
internal/instance/router.go               # HTTP proxying (ProxyTabRequest)
internal/instance/types.go                # Instance, TabEntry, TabRegistry

internal/allocation/policy.go             # Interface
internal/allocation/fcfs.go               # FCFS policy
internal/allocation/round_robin.go        # RoundRobin policy
internal/allocation/load_based.go         # LoadBased policy
internal/allocation/random.go             # Random policy
internal/allocation/factory.go            # NewPolicy()

internal/strategy/simple/simple.go        # Simple strategy impl
internal/strategy/simple/simple_test.go   # Tests

internal/bridge/tab_registry.go           # TabRegistry + HTTP endpoint
```

### Modify
```
internal/orchestrator/handlers.go         # Use InstanceManager in proxyTabRequest
internal/orchestrator/orchestrator.go     # Create + wire InstanceManager
internal/orchestrator/proxy.go            # Remove findRunningInstanceByTabID

internal/bridge/bridge.go                 # Add GetTabRegistry()
internal/bridge/tab_manager.go            # Register/unregister on create/close

internal/config/config.go                 # Add strategy + allocation_policy fields
cmd/pinchtab/cmd_orchestrator.go          # Initialize strategy + policy

internal/strategy/strategy.go             # Already exists, reuse
internal/strategy/session/session.go      # Update to use InstanceManager
```

---

## Success Criteria

✅ **Phase 1:** No merge conflicts, CI passes  
✅ **Phase 2:** InstanceManager works, proxyTabRequest uses cache  
✅ **Phase 3:** All 4 policies implemented and testable  
✅ **Phase 4:** Simple strategy /navigate and /snapshot working  
✅ **Phase 5:** Session strategy updated  
✅ **Phase 6:** Orchestrator fully integrated  
✅ **Phase 7:** Config-driven strategy + policy selection  
✅ **Phase 8:** 100% integration test coverage, docs complete  

**Final:** PR ready to merge with clean commit history

---

## Risk Mitigation

**Risk:** Rebase conflicts  
**Mitigation:** Do Phase 1 first, resolve conflicts carefully, test CI

**Risk:** InstanceManager becomes bottleneck  
**Mitigation:** Cache extensively, use sync.RWMutex, benchmark early

**Risk:** Policies are too simple  
**Mitigation:** Start with 4 basics, extend later; interface is pluggable

**Risk:** Breaking existing flows  
**Mitigation:** Orchestrator.proxyTabRequest must remain backward compatible

---

## Timeline Estimate

- **Phase 1:** 1-2 hours
- **Phase 2:** 5-7 hours (decomposed components)
- **Phase 3:** 2-3 hours
- **Phase 4:** 3-4 hours
- **Phase 5:** 2-3 hours
- **Phase 6:** 2-3 hours
- **Phase 7:** 2-3 hours
- **Phase 8:** 3-4 hours

**Total:** ~23-32 hours (3-4 days with normal breaks)

---

## Next Steps

1. **Mario:** Start Phase 1 (rebase)
2. **Bosch:** Review for architectural correctness
3. **Mario:** Continue phases 2-8 sequentially
4. **Review:** Each phase before moving to next
5. **Test:** Full integration suite before final PR

---

**Last Updated:** 2026-03-03 17:23 GMT+1
