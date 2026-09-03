package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/ids"
	"github.com/pinchtab/pinchtab/internal/instance"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

type InstanceEvent struct {
	Type     string           `json:"type"` // "instance.started", "instance.stopped", "instance.error"
	Instance *bridge.Instance `json:"instance"`
}

type EventHandler func(InstanceEvent)

type Orchestrator struct {
	instances     map[string]*InstanceInternal
	baseDir       string
	binary        string
	profiles      *profiles.ProfileManager
	runner        HostRunner
	mu            sync.RWMutex
	client        *http.Client
	internalToken string
	bindings      *Bindings
	// detachedStops tracks async failed-attempt teardowns so tests (and
	// shutdown paths) can wait for them instead of leaking goroutines that
	// race with stubbed package vars.
	detachedStops sync.WaitGroup

	// monitors tracks the per-instance startup monitors for the same reason.
	// A monitor writes instance state for up to instanceStartupTimeout after
	// launch, so one that outlives its orchestrator keeps touching memory the
	// owner has finished with — and, in tests, package vars the test has already
	// restored. Shutdown waits on this.
	monitors sync.WaitGroup

	// shutdownCh is closed once, by Shutdown, to cut short in-flight startup
	// probes. Without it, waiting on monitors would mean waiting out the full
	// instanceStartupTimeout for any instance still starting.
	//
	// Nil when an Orchestrator is built as a bare struct literal rather than
	// through the constructor, which some tests do. A nil channel is never
	// ready, so every select below simply falls through to its other case.
	shutdownCh   chan struct{}
	shutdownOnce sync.Once

	// strictCrossInstanceTab toggles the cross-instance explicit-tab rule.
	// When false (default), a request that targets a tab on a different
	// instance than the caller's existing identity binding rebinds the
	// caller to the owner instance. When true, such requests return
	// 409 cross_instance_tab and the binding is left untouched.
	strictCrossInstanceTab bool

	// tabsCache stores per-instance snapshots of /tabs results to absorb
	// repeated dashboard visibility queries. Routing never reads it.
	tabsCache        *TabsCache
	portAllocator    *PortAllocator
	idMgr            *ids.Manager
	eventHandlers    []EventHandler
	instanceMgr      *instance.Manager
	live             config.Live
	fallbackLauncher Launcher

	// attachHealthCheckTimeout overrides the default health-check timeout in tests.
	attachHealthCheckTimeout time.Duration
}

func (o *Orchestrator) OnEvent(handler EventHandler) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.eventHandlers = append(o.eventHandlers, handler)
}

func (o *Orchestrator) emitEvent(eventType string, inst *bridge.Instance) {
	o.mu.RLock()
	handlers := make([]EventHandler, len(o.eventHandlers))
	copy(handlers, o.eventHandlers)
	o.mu.RUnlock()
	evt := InstanceEvent{Type: eventType, Instance: inst}
	for _, handler := range handlers {
		handler(evt)
	}
}

func (o *Orchestrator) EmitEvent(eventType string, inst *bridge.Instance) {
	o.emitEvent(eventType, inst)
}

type InstanceInternal struct {
	bridge.Instance
	URL   string
	Error string

	authToken string
	cdpPort   int
	cmd       Cmd
	logBuf    *ringBuffer

	requestedSecurityPolicy *bridge.SecurityPolicy

	requestedProvider string
	browser           string
	effectiveBinary   string

	lastFailureReason LaunchFailureReason
}

type LaunchOptions struct {
	ExtensionPaths []string
	SecurityPolicy *bridge.SecurityPolicy

	RequestedProvider string
	Browser           string
	// TargetName is the resolved browser target name. When set,
	// LaunchWithOptions uses this exact target's config instead of
	// re-deriving a target from Browser — with several targets sharing a
	// provider, re-derivation picks the wrong one.
	TargetName string
}

type AttachOptions struct {
	Browser string
}

var waitForChildBridgeHealthyFunc = func(o *Orchestrator, inst *InstanceInternal, timeout time.Duration) error {
	return o.waitForChildBridgeHealthy(inst, timeout)
}

// generateInternalToken returns a random hex string used as the shared
// secret between the orchestrator and its spawned instances. The token
// authorizes orchestrator → instance proxy hops as trusted-internal,
// allowing X-PinchTab-* identity headers to flow through.
func generateInternalToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Best effort: an empty token disables trusted-internal-proxy and
		// falls back to header stripping on the instance side.
		return ""
	}
	return hex.EncodeToString(b)
}

func NewOrchestrator(baseDir string) *Orchestrator {
	return NewOrchestratorWithRunner(baseDir, &LocalRunner{})
}

func NewOrchestratorWithRunner(baseDir string, runner HostRunner) *Orchestrator {
	orch := &Orchestrator{
		instances:     make(map[string]*InstanceInternal),
		baseDir:       baseDir,
		binary:        resolveStableBinary(baseDir),
		runner:        runner,
		client:        &http.Client{Timeout: httpx.MaxNavigationHTTPDuration},
		internalToken: generateInternalToken(),
		bindings:      NewBindings(nil),
		tabsCache:     NewTabsCache(0, nil),
		portAllocator: NewPortAllocator(9868, 9968),
		idMgr:         ids.NewManager(),
		shutdownCh:    make(chan struct{}),
	}

	orch.registerInstanceCleanupHook()
	orch.initInstanceManager()

	return orch
}

// registerInstanceCleanupHook drops identity → instance bindings and any cached
// tab snapshots when an instance stops or errors, so a restarted instance does
// not keep receiving routed traffic and dashboards do not show ghost tabs.
func (o *Orchestrator) registerInstanceCleanupHook() {
	o.OnEvent(func(evt InstanceEvent) {
		switch evt.Type {
		case "instance.stopped", "instance.error":
			if evt.Instance != nil {
				o.bindings.ClearInstance(evt.Instance.ID)
				o.tabsCache.Invalidate(evt.Instance.ID)
			}
		}
	})
}

func (o *Orchestrator) initInstanceManager() {
	bridgeClient := instance.NewBridgeClientWithAuth(o.authorizeInstanceRequest)
	o.instanceMgr = instance.NewManager(
		&orchestratorLauncher{orch: o},
		bridgeClient,
	)
}

// authorizeInstanceRequest stamps the target instance's own credential on a
// request the instance manager built for it. A spawned instance runs the same
// auth middleware as this process, so a bare call to its /tabs is answered 401 —
// which the locator's discovery swallowed at debug level, leaving the tab→instance
// cache to be filled only by lookups that had already succeeded some other way.
//
// Resolved per request rather than closed over one token because an attached
// external bridge authenticates with its own, which applyInstanceAuth knows and a
// captured server token would not.
func (o *Orchestrator) authorizeInstanceRequest(req *http.Request) {
	if o == nil || req == nil {
		return
	}
	o.applyInstanceAuth(req, o.proxyTargetInstance(req.URL))
}

func (o *Orchestrator) RunMaintenance(ctx context.Context) {
	if o == nil {
		return
	}
	const (
		tick     = 5 * time.Minute
		idleTTL  = 1 * time.Hour
		maxAgent = 10000
	)
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			o.runMaintenanceOnce(idleTTL, maxAgent)
		}
	}
}

// runMaintenanceOnce bounds the caches that nothing else prunes. Agent bindings
// have no lifecycle signal, and the tab→instance cache misses every tab that
// closes without passing through the proxy — bridge-side idle eviction and
// window.close() among them — so both need periodic reconciliation.
func (o *Orchestrator) runMaintenanceOnce(idleTTL time.Duration, maxAgent int) {
	o.bindings.PruneAgents(idleTTL, maxAgent)
	if o.instanceMgr != nil {
		o.instanceMgr.Locator.RefreshAll()
	}
}

func (o *Orchestrator) Bindings() *Bindings {
	if o == nil {
		return nil
	}
	return o.bindings
}

// SessionTabIDs reports tabs created for a session and still present in the
// orchestrator's ownership ledger.
func (o *Orchestrator) SessionTabIDs(sessionID string) []string {
	if o == nil || o.bindings == nil {
		return []string{}
	}
	return o.bindings.SessionTabIDs(sessionID)
}

func (o *Orchestrator) SetStrictCrossInstanceTab(strict bool) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.strictCrossInstanceTab = strict
	o.mu.Unlock()
}

func (o *Orchestrator) InstanceManager() *instance.Manager {
	return o.instanceMgr
}

func (o *Orchestrator) SetAllocationPolicy(name string) error {
	return o.instanceMgr.SetAllocationPolicy(name)
}

type orchestratorLauncher struct {
	orch *Orchestrator
}

func (l *orchestratorLauncher) Launch(name, port string, headless bool) (*bridge.Instance, error) {
	return l.orch.Launch(name, port, headless, nil)
}

func (l *orchestratorLauncher) Stop(id string) error {
	return l.orch.Stop(id)
}

func (o *Orchestrator) syncInstanceToManager(inst *bridge.Instance) {
	if o.instanceMgr == nil {
		return
	}
	o.instanceMgr.Repo.Add(inst)
}

func (o *Orchestrator) SetProfileManager(pm *profiles.ProfileManager) {
	o.profiles = pm
}

// ApplyRuntimeConfig publishes the value every reader will see from here on. The
// config is never written in place, so publication is a single pointer store and
// only the derived state that lives outside it needs the lock.
func (o *Orchestrator) ApplyRuntimeConfig(cfg *config.RuntimeConfig) {
	o.live.Publish(cfg)
	if cfg == nil {
		return
	}
	o.SetPortRange(cfg.InstancePortStart, cfg.InstancePortEnd)
	if cfg.AllocationPolicy != "" {
		if err := o.SetAllocationPolicy(cfg.AllocationPolicy); err != nil {
			slog.Warn("failed to apply allocation policy", "policy", cfg.AllocationPolicy, "err", err)
		}
	}
}

// LiveConfig hands out the publication point so every other holder of the runtime
// config in this process reads and writes the same one. A save through the
// dashboard is then visible to the orchestrator's goroutines by construction,
// rather than by two holders agreeing to mutate one shared object.
func (o *Orchestrator) LiveConfig() *config.Live {
	if o == nil {
		return nil
	}
	return &o.live
}

// cfg is the ONE accessor for the published runtime config. Nothing reads a bare
// field: a nil Live (a bare struct literal in a test) answers nil, which every
// caller already handles.
func (o *Orchestrator) cfg() *config.RuntimeConfig {
	if o == nil {
		return nil
	}
	return o.live.Get()
}

// ports is the same discipline for the allocator, which is a swap rather than a
// publication: SetPortRange replaces it while launches are reading it.
func (o *Orchestrator) ports() *PortAllocator {
	if o == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.portAllocator
}

// cfgToken is the child auth token: derived from the published config on every
// read rather than copied into a field, so a reader cannot observe a token that
// disagrees with the config in effect.
func (o *Orchestrator) cfgToken() string {
	cfg := o.cfg()
	if cfg == nil {
		return ""
	}
	return cfg.Token
}

func (o *Orchestrator) AllowsEvaluate() bool {
	cfg := o.cfg()
	return cfg != nil && cfg.AllowEvaluate
}

func (o *Orchestrator) AllowsMacro() bool {
	cfg := o.cfg()
	return cfg != nil && cfg.AllowMacro
}

func (o *Orchestrator) AllowsScreencast() bool {
	cfg := o.cfg()
	return cfg != nil && cfg.AllowScreencast
}

func (o *Orchestrator) AllowsDownload() bool {
	cfg := o.cfg()
	return cfg != nil && cfg.AllowDownload
}

func (o *Orchestrator) AllowsCookies() bool {
	cfg := o.cfg()
	return cfg != nil && cfg.AllowCookies
}

func (o *Orchestrator) AllowsUpload() bool {
	cfg := o.cfg()
	return cfg != nil && cfg.AllowUpload
}

func (o *Orchestrator) AllowsStateExport() bool {
	cfg := o.cfg()
	return cfg != nil && cfg.AllowStateExport
}

func (o *Orchestrator) AllowsNetworkIntercept() bool {
	cfg := o.cfg()
	return cfg != nil && cfg.AllowNetworkIntercept
}

func (o *Orchestrator) SetPortRange(start, end int) {
	allocator := NewPortAllocator(start, end)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.portAllocator = allocator
}
