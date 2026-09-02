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
	instances      map[string]*InstanceInternal
	baseDir        string
	binary         string
	profiles       *profiles.ProfileManager
	runner         HostRunner
	mu             sync.RWMutex
	client         *http.Client
	childAuthToken string
	allowEvaluate  bool
	internalToken  string
	bindings       *Bindings
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
	runtimeCfg       *config.RuntimeConfig
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
		instances:      make(map[string]*InstanceInternal),
		baseDir:        baseDir,
		binary:         resolveStableBinary(baseDir),
		runner:         runner,
		client:         &http.Client{Timeout: httpx.MaxNavigationHTTPDuration},
		childAuthToken: "",
		allowEvaluate:  false,
		internalToken:  generateInternalToken(),
		bindings:       NewBindings(nil),
		tabsCache:      NewTabsCache(0, nil),
		portAllocator:  NewPortAllocator(9868, 9968),
		idMgr:          ids.NewManager(),
		shutdownCh:     make(chan struct{}),
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
	bridgeClient := instance.NewBridgeClient()
	o.instanceMgr = instance.NewManager(
		&orchestratorLauncher{orch: o},
		bridgeClient,
	)
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

func (o *Orchestrator) ApplyRuntimeConfig(cfg *config.RuntimeConfig) {
	o.runtimeCfg = cfg
	if cfg == nil {
		o.childAuthToken = ""
		o.allowEvaluate = false
		return
	}
	o.childAuthToken = cfg.Token
	o.allowEvaluate = cfg.AllowEvaluate
	o.SetPortRange(cfg.InstancePortStart, cfg.InstancePortEnd)
	if cfg.AllocationPolicy != "" {
		if err := o.SetAllocationPolicy(cfg.AllocationPolicy); err != nil {
			slog.Warn("failed to apply allocation policy", "policy", cfg.AllocationPolicy, "err", err)
		}
	}
}

func (o *Orchestrator) AllowsEvaluate() bool {
	return o != nil && o.allowEvaluate
}

func (o *Orchestrator) AllowsMacro() bool {
	return o != nil && o.runtimeCfg != nil && o.runtimeCfg.AllowMacro
}

func (o *Orchestrator) AllowsScreencast() bool {
	return o != nil && o.runtimeCfg != nil && o.runtimeCfg.AllowScreencast
}

func (o *Orchestrator) AllowsDownload() bool {
	return o != nil && o.runtimeCfg != nil && o.runtimeCfg.AllowDownload
}

func (o *Orchestrator) AllowsCookies() bool {
	return o != nil && o.runtimeCfg != nil && o.runtimeCfg.AllowCookies
}

func (o *Orchestrator) AllowsUpload() bool {
	return o != nil && o.runtimeCfg != nil && o.runtimeCfg.AllowUpload
}

func (o *Orchestrator) AllowsStateExport() bool {
	return o != nil && o.runtimeCfg != nil && o.runtimeCfg.AllowStateExport
}

func (o *Orchestrator) AllowsNetworkIntercept() bool {
	return o != nil && o.runtimeCfg != nil && o.runtimeCfg.AllowNetworkIntercept
}

func (o *Orchestrator) SetPortRange(start, end int) {
	o.portAllocator = NewPortAllocator(start, end)
}
