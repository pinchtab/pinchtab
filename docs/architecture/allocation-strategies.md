# Allocation Strategies

## Quick Start

**Default strategy:** `simple` — single instance, no explicit management needed.

To switch to session-based allocation, add one line to your config:

```yaml
# pinchtab.yaml
strategy: session
```

Or via environment variable:
```bash
PINCHTAB_STRATEGY=session pinchtab
```

### Migration Note

If you're upgrading from a version without strategies:
- **No action needed** — the `simple` strategy behaves like the previous default
- Existing `/instances/*` and `/tabs/*` endpoints continue to work
- Add `strategy: session` only if you want automatic session management

---

## Overview

Pinchtab supports two usage modes:

1. **Power User Mode** — Agent explicitly manages instances, profiles, and tabs
2. **Casual Mode** — Agent just wants a browser; Pinchtab handles allocation

The **Strategy Pattern** allows swapping allocation logic without changing the core API.

```
┌─────────────────────────────────────────────────┐
│  Strategy Layer (swappable)                     │
│  - Registers HTTP endpoints                     │
│  - Controls allocation logic                    │
├─────────────────────────────────────────────────┤
│  Primitive Layer (fixed interface)              │
│  - InstanceManager: launch, stop, list          │
│  - TabManager: open, close, navigate, action    │
│  - ProfileManager: create, delete, list         │
├─────────────────────────────────────────────────┤
│  Bridge Layer (Chrome/CDP)                      │
└─────────────────────────────────────────────────┘
```

Strategies **compose** primitives — they don't reimplement them.

## Primitive Interfaces

Primitives are the internal building blocks that all strategies use.

```go
// internal/primitive/primitive.go

package primitive

import "context"

// InstanceManager controls browser instances
type InstanceManager interface {
    Launch(ctx context.Context, profile string, port int, headless bool) (*Instance, error)
    Stop(ctx context.Context, id string) error
    List() []*Instance
    Get(id string) (*Instance, bool)
    FirstRunning() *Instance
}

// TabManager controls tabs within instances
type TabManager interface {
    Open(ctx context.Context, instanceID string, url string) (tabID string, err error)
    Close(ctx context.Context, tabID string) error
    List(ctx context.Context, instanceID string) ([]*Tab, error)
    Get(tabID string) (*Tab, bool)
    
    // Tab operations
    Navigate(ctx context.Context, tabID string, url string, opts NavigateOpts) error
    Snapshot(ctx context.Context, tabID string, opts SnapshotOpts) (*Snapshot, error)
    Action(ctx context.Context, tabID string, action Action) (*ActionResult, error)
    Screenshot(ctx context.Context, tabID string, opts ScreenshotOpts) ([]byte, error)
    PDF(ctx context.Context, tabID string, opts PDFOpts) ([]byte, error)
    Text(ctx context.Context, tabID string, opts TextOpts) (*TextResult, error)
    Evaluate(ctx context.Context, tabID string, expr string) (any, error)
    Cookies(ctx context.Context, tabID string) ([]*Cookie, error)
    SetCookies(ctx context.Context, tabID string, cookies []*Cookie) error
}

// ProfileManager controls browser profiles
type ProfileManager interface {
    List() ([]*Profile, error)
    Create(name string) error
    Delete(name string) error
    Exists(name string) bool
    Reset(name string) error
}

// Primitives bundles all managers for injection
type Primitives struct {
    Instances InstanceManager
    Tabs      TabManager
    Profiles  ProfileManager
}
```

## Strategy Interface

Strategies define how tabs are allocated and what endpoints are exposed.

```go
// internal/strategy/strategy.go

package strategy

import (
    "context"
    "net/http"
    
    "github.com/pinchtab/pinchtab/internal/primitive"
)

// Strategy defines a browser allocation approach
type Strategy interface {
    // Name returns strategy identifier for config
    Name() string
    
    // Init receives primitives to compose
    Init(p *primitive.Primitives) error
    
    // RegisterRoutes adds strategy-specific HTTP endpoints
    RegisterRoutes(mux *http.ServeMux)
    
    // Start begins background tasks (pool warming, cleanup)
    Start(ctx context.Context) error
    
    // Stop gracefully shuts down
    Stop() error
}
```

## Strategy Registry

Strategies are registered by name for config-driven selection.

```go
// internal/strategy/registry.go

package strategy

import "github.com/pinchtab/pinchtab/internal/primitive"

type Factory func() Strategy

var registry = map[string]Factory{}

func Register(name string, factory Factory) {
    registry[name] = factory
}

func New(name string) (Strategy, error) {
    factory, ok := registry[name]
    if !ok {
        return nil, fmt.Errorf("unknown strategy: %s", name)
    }
    return factory(), nil
}

func init() {
    Register("explicit", func() Strategy { return &explicit.Strategy{} })
    Register("session", func() Strategy { return &session.Strategy{} })
    Register("pool", func() Strategy { return &pool.Strategy{} })
}
```

---

## Built-in Strategies

### 1. Explicit Strategy (default)

The current behavior. Agent explicitly manages everything.

**Endpoints:** All primitive endpoints exposed directly.

```
POST /instances/launch      → Launch instance
POST /instances/{id}/stop   → Stop instance
GET  /instances             → List instances

POST /instances/{id}/tabs/open  → Open tab
POST /tabs/{id}/close           → Close tab
POST /tabs/{id}/navigate        → Navigate
GET  /tabs/{id}/snapshot        → Snapshot
POST /tabs/{id}/action          → Action
...

GET  /profiles              → List profiles
POST /profiles              → Create profile
```

**Use case:** Power users, multi-instance setups, explicit control.

**Config:**
```yaml
strategy: explicit
```

---

### 2. Session Strategy

Agent gets a session; Pinchtab manages tab lifecycle.

**New endpoints:**

```
POST   /session              → Create session, returns session_xxx + tab_xxx
GET    /session/{id}         → Get session info
DELETE /session/{id}         → End session, release tab

POST   /browse               → Navigate (auto-allocates if no session)
```

**Behavior:**
- `POST /session` → Ensures instance running, opens tab, returns session ID
- `X-Session-ID` header → Sticky tab assignment
- Sessions expire after TTL (configurable)
- Primitive endpoints still available for power users

**Implementation:**

```go
// internal/strategy/session/session.go

type Strategy struct {
    p        *primitive.Primitives
    sessions sync.Map // sessionID → *Session
    config   Config
}

type Session struct {
    ID         string
    TabID      string
    InstanceID string
    CreatedAt  time.Time
    LastUsed   time.Time
}

type Config struct {
    DefaultProfile string        `yaml:"default_profile"`
    SessionTTL     time.Duration `yaml:"session_ttl"`
    AutoLaunch     bool          `yaml:"auto_launch"`
}

func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
    // Session endpoints
    mux.HandleFunc("POST /session", s.handleCreateSession)
    mux.HandleFunc("GET /session/{id}", s.handleGetSession)
    mux.HandleFunc("DELETE /session/{id}", s.handleDeleteSession)
    
    // Convenience endpoint
    mux.HandleFunc("POST /browse", s.handleBrowse)
    
    // Also expose primitive endpoints
    s.registerPrimitiveRoutes(mux)
}

func (s *Strategy) handleCreateSession(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    // Ensure instance running
    inst := s.p.Instances.FirstRunning()
    if inst == nil && s.config.AutoLaunch {
        var err error
        inst, err = s.p.Instances.Launch(ctx, s.config.DefaultProfile, 0, true)
        if err != nil {
            web.Error(w, 503, "launch_failed", err.Error())
            return
        }
    }
    
    // Open tab
    tabID, err := s.p.Tabs.Open(ctx, inst.ID, "about:blank")
    if err != nil {
        web.Error(w, 500, "tab_failed", err.Error())
        return
    }
    
    // Create session
    sess := &Session{
        ID:         "sess_" + generateID(),
        TabID:      tabID,
        InstanceID: inst.ID,
        CreatedAt:  time.Now(),
        LastUsed:   time.Now(),
    }
    s.sessions.Store(sess.ID, sess)
    
    web.JSON(w, 201, map[string]string{
        "session_id": sess.ID,
        "tab_id":     sess.TabID,
    })
}

func (s *Strategy) handleBrowse(w http.ResponseWriter, r *http.Request) {
    sessionID := r.Header.Get("X-Session-ID")
    
    // Find or create session
    sess := s.getOrCreateSession(r.Context(), sessionID)
    
    // Parse request
    var req struct{ URL string `json:"url"` }
    json.NewDecoder(r.Body).Decode(&req)
    
    // Navigate
    err := s.p.Tabs.Navigate(r.Context(), sess.TabID, req.URL, primitive.NavigateOpts{})
    if err != nil {
        web.Error(w, 500, "navigate_failed", err.Error())
        return
    }
    
    // Return snapshot for convenience
    snap, _ := s.p.Tabs.Snapshot(r.Context(), sess.TabID, primitive.SnapshotOpts{
        Interactive: true,
        Compact:     true,
    })
    
    web.JSON(w, 200, map[string]any{
        "session_id": sess.ID,
        "tab_id":     sess.TabID,
        "snapshot":   snap,
    })
}

func (s *Strategy) Start(ctx context.Context) error {
    go s.cleanupExpiredSessions(ctx)
    return nil
}
```

**Config:**
```yaml
strategy: session

strategy_options:
  default_profile: default
  session_ttl: 30m
  auto_launch: true
```

---

### 3. Pool Strategy

Pre-warmed pool of tabs. Agents acquire/release.

**New endpoints:**

```
POST /acquire              → Get tab from pool (blocks if exhausted)
POST /release/{id}         → Return tab to pool
GET  /pool/stats           → Pool metrics
```

**Behavior:**
- Pool is pre-warmed on startup
- Tabs are reset (cookies cleared, navigated to blank) on release
- Expired leases are automatically recycled
- Primitive endpoints still available

**Implementation:**

```go
// internal/strategy/pool/pool.go

type Strategy struct {
    p      *primitive.Primitives
    ready  chan *Tab           // Available tabs
    inUse  sync.Map            // tabID → *Lease
    config Config
}

type Tab struct {
    ID         string
    InstanceID string
}

type Lease struct {
    TabID   string
    AgentID string
    Start   time.Time
}

type Config struct {
    PoolSize       int           `yaml:"pool_size"`
    MaxLeaseTime   time.Duration `yaml:"max_lease_time"`
    WarmupURL      string        `yaml:"warmup_url"`
    DefaultProfile string        `yaml:"default_profile"`
}

func (s *Strategy) Start(ctx context.Context) error {
    s.ready = make(chan *Tab, s.config.PoolSize)
    
    // Pre-warm pool
    inst, err := s.p.Instances.Launch(ctx, s.config.DefaultProfile, 0, true)
    if err != nil {
        return fmt.Errorf("pool launch failed: %w", err)
    }
    
    for i := 0; i < s.config.PoolSize; i++ {
        tabID, err := s.p.Tabs.Open(ctx, inst.ID, s.config.WarmupURL)
        if err != nil {
            return fmt.Errorf("pool warmup failed: %w", err)
        }
        s.ready <- &Tab{ID: tabID, InstanceID: inst.ID}
    }
    
    // Background recycler
    go s.recycleExpiredLeases(ctx)
    
    return nil
}

func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("POST /acquire", s.handleAcquire)
    mux.HandleFunc("POST /release/{id}", s.handleRelease)
    mux.HandleFunc("GET /pool/stats", s.handleStats)
    
    // Tab operations on acquired tabs
    s.registerTabRoutes(mux)
}

func (s *Strategy) handleAcquire(w http.ResponseWriter, r *http.Request) {
    select {
    case tab := <-s.ready:
        s.inUse.Store(tab.ID, &Lease{
            TabID:   tab.ID,
            AgentID: r.Header.Get("X-Agent-ID"),
            Start:   time.Now(),
        })
        web.JSON(w, 200, map[string]string{
            "tab_id":      tab.ID,
            "instance_id": tab.InstanceID,
        })
        
    case <-time.After(10 * time.Second):
        web.Error(w, 503, "pool_exhausted", "no tabs available")
    }
}

func (s *Strategy) handleRelease(w http.ResponseWriter, r *http.Request) {
    tabID := r.PathValue("id")
    
    lease, ok := s.inUse.LoadAndDelete(tabID)
    if !ok {
        web.Error(w, 404, "not_leased", "tab not in use")
        return
    }
    
    // Reset tab state
    s.p.Tabs.Navigate(r.Context(), tabID, "about:blank", primitive.NavigateOpts{})
    // TODO: Clear cookies
    
    // Return to pool
    s.ready <- &Tab{ID: tabID, InstanceID: lease.(*Lease).TabID}
    
    web.JSON(w, 200, map[string]string{"status": "released"})
}

func (s *Strategy) handleStats(w http.ResponseWriter, r *http.Request) {
    inUseCount := 0
    s.inUse.Range(func(_, _ any) bool {
        inUseCount++
        return true
    })
    
    web.JSON(w, 200, map[string]int{
        "pool_size": s.config.PoolSize,
        "available": len(s.ready),
        "in_use":    inUseCount,
    })
}
```

**Config:**
```yaml
strategy: pool

strategy_options:
  pool_size: 10
  max_lease_time: 5m
  warmup_url: "about:blank"
  default_profile: default
```

---

## Orchestrator Integration

The orchestrator loads strategy from config and wires everything together.

```go
// internal/orchestrator/orchestrator.go

type Orchestrator struct {
    primitives *primitive.Primitives
    strategy   strategy.Strategy
    mux        *http.ServeMux
}

func New(cfg Config) (*Orchestrator, error) {
    // Build primitives from existing code
    p := &primitive.Primitives{
        Instances: newInstanceManager(cfg),
        Tabs:      newTabManager(cfg),
        Profiles:  profiles.NewManager(cfg.ProfileDir),
    }
    
    // Load strategy
    strat, err := strategy.New(cfg.Strategy)
    if err != nil {
        return nil, fmt.Errorf("strategy init: %w", err)
    }
    if err := strat.Init(p); err != nil {
        return nil, fmt.Errorf("strategy init: %w", err)
    }
    
    // Build mux
    mux := http.NewServeMux()
    
    // Core endpoints (always available)
    mux.HandleFunc("GET /health", handleHealth)
    mux.HandleFunc("GET /metrics", handleMetrics)
    
    // Strategy registers its routes
    strat.RegisterRoutes(mux)
    
    return &Orchestrator{
        primitives: p,
        strategy:   strat,
        mux:        mux,
    }, nil
}

func (o *Orchestrator) Start(ctx context.Context) error {
    return o.strategy.Start(ctx)
}

func (o *Orchestrator) Stop() error {
    return o.strategy.Stop()
}
```

---

## Configuration

```yaml
# pinchtab.yaml

# Strategy selection
strategy: session  # explicit | session | pool

# Strategy-specific options
strategy_options:
  # session strategy
  default_profile: default
  session_ttl: 30m
  auto_launch: true
  
  # pool strategy
  pool_size: 10
  max_lease_time: 5m
  warmup_url: "about:blank"

# Other config...
port: 9867
headless: true
```

---

## File Structure

```
internal/
├── primitive/
│   ├── primitive.go       # Interfaces: InstanceManager, TabManager, ProfileManager
│   ├── instance.go        # Instance type
│   ├── tab.go             # Tab type + options
│   └── profile.go         # Profile type
│
├── strategy/
│   ├── strategy.go        # Strategy interface
│   ├── registry.go        # Strategy registry + factory
│   │
│   ├── explicit/
│   │   └── explicit.go    # Current behavior (power user)
│   │
│   ├── session/
│   │   ├── session.go     # Session-based allocation
│   │   └── cleanup.go     # TTL cleanup loop
│   │
│   └── pool/
│       ├── pool.go        # Pre-warmed pool
│       └── recycler.go    # Lease expiry recycler
│
├── orchestrator/
│   ├── orchestrator.go    # Wires primitives + strategy
│   └── ...
```

---

## Comparison

| Feature | Explicit | Session | Pool |
|---------|----------|---------|------|
| Agent manages instances | ✅ | ❌ Auto | ❌ Auto |
| Agent manages tabs | ✅ | ❌ Auto | ✅ Acquire/Release |
| Sticky tabs | ❌ | ✅ Session ID | ❌ |
| Pre-warmed | ❌ | ❌ | ✅ |
| Best for | Power users | Casual agents | High throughput |
| Startup cost | None | On-demand | Upfront |

---

## Extending with Custom Strategies

Third parties can implement custom strategies:

```go
package custom

import (
    "github.com/pinchtab/pinchtab/internal/strategy"
    "github.com/pinchtab/pinchtab/internal/primitive"
)

func init() {
    strategy.Register("custom", func() strategy.Strategy {
        return &CustomStrategy{}
    })
}

type CustomStrategy struct {
    p *primitive.Primitives
}

func (s *CustomStrategy) Name() string { return "custom" }

func (s *CustomStrategy) Init(p *primitive.Primitives) error {
    s.p = p
    return nil
}

func (s *CustomStrategy) RegisterRoutes(mux *http.ServeMux) {
    // Custom endpoints...
}

func (s *CustomStrategy) Start(ctx context.Context) error { return nil }
func (s *CustomStrategy) Stop() error { return nil }
```

---

## Related

- [Issue #85: Allocation Strategy](https://github.com/pinchtab/pinchtab/issues/85)
- [Pinchtab Architecture](./pinchtab-architecture.md)
- [Chrome Lifecycle](./chrome-lifecycle-and-orchestration.md)
