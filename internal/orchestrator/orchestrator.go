package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/ids"
	"github.com/pinchtab/pinchtab/internal/instance"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

// InstanceEvent is emitted when instance state changes.
type InstanceEvent struct {
	Type     string           `json:"type"` // "instance.started", "instance.stopped", "instance.error"
	Instance *bridge.Instance `json:"instance"`
}

// EventHandler receives instance lifecycle events.
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

// OnEvent adds an event handler for instance lifecycle events.
// Multiple handlers can be registered; all will be called in order.
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

// EmitEvent allows external components (e.g. strategies) to broadcast
// lifecycle events through the orchestrator's event system.
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
	binDir := filepath.Join(filepath.Dir(baseDir), "bin")
	stableBin := filepath.Join(binDir, "pinchtab")
	exe, _ := os.Executable()
	binary := exe
	if binary == "" {
		binary = os.Args[0]
	}

	if err := os.MkdirAll(binDir, 0755); err != nil {
		slog.Warn("failed to create bin directory", "path", binDir, "err", err)
	}

	if exe != "" {
		if err := installStableBinary(exe, stableBin); err != nil {
			slog.Warn("failed to install pinchtab binary", "path", stableBin, "err", err)
		} else {
			slog.Debug("installed pinchtab binary", "path", stableBin)
		}
	}

	if _, err := os.Stat(binary); err != nil {
		if _, stableErr := os.Stat(stableBin); stableErr == nil {
			binary = stableBin
		}
	}

	orch := &Orchestrator{
		instances: make(map[string]*InstanceInternal),
		baseDir:   baseDir,
		binary:    binary,
		runner:    runner,
		// Client timeout for proxying to instances: 60 seconds
		// Why so high?
		// - First request to an instance triggers lazy Chrome initialization (8-20+ seconds)
		// - Navigation can take up to 60s (NavigateTimeout in bridge config)
		// - Proxied requests (e.g., POST /tabs/{tabId}/navigate) must wait for:
		//   1. Instance /health handler to initialize Chrome (via ensureChrome())
		//   2. Tab operations to complete (navigate, snapshot, actions, etc.)
		// - Short timeout (<5s) would break first-request scenarios
		// See: internal/orchestrator/health.go (monitor), internal/bridge/init.go (InitChrome)
		client:         &http.Client{Timeout: 60 * time.Second},
		childAuthToken: "",
		allowEvaluate:  false,
		internalToken:  generateInternalToken(),
		bindings:       NewBindings(nil),
		tabsCache:      NewTabsCache(0, nil),
		portAllocator:  NewPortAllocator(9868, 9968),
		idMgr:          ids.NewManager(),
	}

	// Drop identity → instance bindings and any cached tab snapshots when
	// an instance stops or errors so a restarted instance does not keep
	// receiving routed traffic and dashboards do not show ghost tabs.
	orch.OnEvent(func(evt InstanceEvent) {
		switch evt.Type {
		case "instance.stopped", "instance.error":
			if evt.Instance != nil {
				orch.bindings.ClearInstance(evt.Instance.ID)
				orch.tabsCache.Invalidate(evt.Instance.ID)
			}
		}
	})

	bridgeClient := instance.NewBridgeClient()
	orch.instanceMgr = instance.NewManager(
		&orchestratorLauncher{orch: orch},
		bridgeClient,
	)

	return orch
}

// RunMaintenance runs periodic background maintenance tasks for the
// orchestrator (currently: pruning idle agent bindings). Returns when ctx
// is cancelled.
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
			o.bindings.PruneAgents(idleTTL, maxAgent)
		}
	}
}

// Bindings returns the identity → instance binding map. Exposed for tests
// and for handlers that need to inspect routing state.
func (o *Orchestrator) Bindings() *Bindings {
	if o == nil {
		return nil
	}
	return o.bindings
}

// SetStrictCrossInstanceTab toggles strict cross-instance handling. See
// the strictCrossInstanceTab field for semantics.
func (o *Orchestrator) SetStrictCrossInstanceTab(strict bool) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.strictCrossInstanceTab = strict
	o.mu.Unlock()
}

// InstanceManager returns the decomposed instance manager.
func (o *Orchestrator) InstanceManager() *instance.Manager {
	return o.instanceMgr
}

// SetAllocationPolicy changes the allocation policy at runtime.
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

func installStableBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}
