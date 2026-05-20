package orchestrator

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/ids"
	"github.com/pinchtab/pinchtab/internal/instance"
	"github.com/pinchtab/pinchtab/internal/profiles"
	internalurls "github.com/pinchtab/pinchtab/internal/urls"
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

	requestedBrowserTarget string
	resolvedBrowserTarget  string
	browserProvider        string
	effectiveBinary        string

	lastFailureReason LaunchFailureReason
}

type LaunchOptions struct {
	ExtensionPaths []string
	SecurityPolicy *bridge.SecurityPolicy

	RequestedBrowserTarget string
	BrowserTarget          string
	BrowserProvider        string
}

type AttachOptions struct {
	BrowserTarget   string
	BrowserProvider string
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

func (o *Orchestrator) Launch(name, port string, headless bool, extensionPaths []string) (*bridge.Instance, error) {
	opts := LaunchOptions{
		ExtensionPaths: extensionPaths,
	}
	return o.LaunchWithTargetSelection(name, port, headless, "", nil, opts)
}

func (o *Orchestrator) LaunchWithOptions(name, port string, headless bool, opts LaunchOptions) (*bridge.Instance, error) {
	// Validate profile name to prevent path traversal attacks
	if err := profiles.ValidateProfileName(name); err != nil {
		return nil, err
	}
	reservedPorts := make([]int, 0, 2)
	defer func() {
		for _, reserved := range reservedPorts {
			o.portAllocator.ReleasePort(reserved)
		}
	}()

	o.mu.Lock()

	if port == "" || port == "0" {
		o.mu.Unlock()
		allocatedPort, err := o.portAllocator.AllocatePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate port: %w", err)
		}
		port = fmt.Sprintf("%d", allocatedPort)
		reservedPorts = append(reservedPorts, allocatedPort)
		o.mu.Lock()
	} else {
		o.mu.Unlock()
		portInt, err := parsePortNumber(port)
		if err != nil {
			return nil, err
		}
		port = strconv.Itoa(portInt)
		if err := o.portAllocator.ReservePort(portInt); err != nil {
			return nil, fmt.Errorf("failed to reserve port %s: %w", port, err)
		}
		if portInt >= o.portAllocator.start && portInt <= o.portAllocator.end {
			reservedPorts = append(reservedPorts, portInt)
		}
		o.mu.Lock()
	}

	for _, inst := range o.instances {
		if inst.Port == port && instanceIsActive(inst) {
			o.mu.Unlock()
			return nil, fmt.Errorf("port %s already in use by instance %q", port, inst.ProfileName)
		}
		if inst.ProfileName == name && instanceIsActive(inst) {
			o.mu.Unlock()
			return nil, fmt.Errorf("profile %q already has an active instance (%s)", name, inst.Status)
		}
	}
	portInspection := o.runner.InspectPort(port)
	if !portInspection.Available {
		o.mu.Unlock()
		err := portConflictError(port, portInspection)
		slog.Error("instance launch blocked by port conflict", "profile", name, "port", port, "pid", portInspection.PID, "command", portInspection.Command, "error", err.Error())
		return nil, err
	}

	profileID := o.idMgr.ProfileID(name)
	instanceID := o.idMgr.InstanceID(profileID, name)

	if inst, ok := o.instances[instanceID]; ok && inst.Status == "running" {
		o.mu.Unlock()
		return nil, fmt.Errorf("instance already running for profile %q", name)
	}

	o.mu.Unlock()

	cdpPort, err := o.portAllocator.AllocatePort()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate chrome debug port: %w", err)
	}
	reservedPorts = append(reservedPorts, cdpPort)

	profilePath := filepath.Join(o.baseDir, name)
	if o.profiles != nil {
		if resolvedPath, err := o.profiles.ProfilePath(name); err == nil {
			profilePath = resolvedPath
		}
	}
	if err := os.MkdirAll(filepath.Join(profilePath, "Default"), 0755); err != nil {
		return nil, fmt.Errorf("create profile dir: %w", err)
	}
	instanceStateDir := filepath.Join(profilePath, ".pinchtab-state")
	if err := os.MkdirAll(instanceStateDir, 0700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	if err := os.Chmod(instanceStateDir, 0700); err != nil {
		return nil, fmt.Errorf("set state dir permissions: %w", err)
	}

	requestedPolicy := cloneSecurityPolicy(opts.SecurityPolicy)
	effectivePolicy := effectiveSecurityPolicy(o.runtimeCfg, requestedPolicy)

	// Target-aware launch only applies in orchestrator mode; direct-bridge path reads the global RuntimeConfig.
	effectiveCfg := o.runtimeCfg
	if strings.TrimSpace(opts.BrowserTarget) != "" {
		resolved, err := config.ResolveExplicitBrowserTarget(o.runtimeCfg, opts.BrowserTarget)
		if err != nil {
			return nil, err
		}
		effectiveCfg = resolved.Config
	}

	childConfigPath, err := o.writeChildConfig(effectiveCfg, port, cdpPort, profilePath, instanceStateDir, headless, opts.ExtensionPaths, effectivePolicy)
	if err != nil {
		return nil, fmt.Errorf("write child config: %w", err)
	}

	envOverrides := map[string]string{
		"PINCHTAB_PORT":   port,
		"PINCHTAB_CONFIG": childConfigPath,
	}
	if o.internalToken != "" {
		envOverrides["PINCHTAB_INTERNAL_TOKEN"] = o.internalToken
	}
	env := mergeEnvWithOverrides(filterEnvWithPrefixes(os.Environ(), "PINCHTAB_"), envOverrides)

	logBuf := newRingBuffer(256 * 1024)
	slog.Info("starting instance process", "id", instanceID, "profile", name, "port", port)

	cmd, err := o.runner.Run(context.Background(), o.binary, []string{"bridge"}, env, logBuf, logBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to start: %w", err)
	}

	inst := &InstanceInternal{
		Instance: bridge.Instance{
			ID:              instanceID,
			ProfileID:       profileID,
			ProfileName:     name,
			Port:            port,
			URL:             o.childInstanceBaseURL(port),
			Mode:            bridge.ModeFromHeadless(headless),
			Headless:        headless,
			Status:          "starting",
			StartTime:       time.Now(),
			SecurityPolicy:  effectivePolicy,
			BrowserTarget:   opts.BrowserTarget,
			BrowserProvider: opts.BrowserProvider,
		},
		URL:     o.childInstanceBaseURL(port),
		cdpPort: cdpPort,
		cmd:     cmd,
		logBuf:  logBuf,

		requestedSecurityPolicy: requestedPolicy,
		requestedBrowserTarget:  opts.RequestedBrowserTarget,
		resolvedBrowserTarget:   opts.BrowserTarget,
		browserProvider:         opts.BrowserProvider,
		effectiveBinary:         effectiveBinaryFromCfg(effectiveCfg),
	}

	o.mu.Lock()
	o.instances[instanceID] = inst
	o.mu.Unlock()
	reservedPorts = nil

	go o.monitor(inst)

	return &inst.Instance, nil
}

func (o *Orchestrator) childInstanceBaseURL(port string) string {
	host := configuredChildInstanceHost("")
	if o != nil && o.runtimeCfg != nil {
		host = configuredChildInstanceHost(o.runtimeCfg.Bind)
	}
	return httpBaseURL(host, port)
}

func portConflictError(port string, inspection PortInspection) error {
	if inspection.PID > 0 {
		process := fmt.Sprintf("pid %d", inspection.PID)
		if command := strings.TrimSpace(inspection.Command); command != "" {
			process = fmt.Sprintf("%s (%s)", process, command)
		}
		if strings.Contains(strings.ToLower(inspection.Command), "pinchtab") {
			return fmt.Errorf("instance port %s is already in use by %s; stop the stale process and restart PinchTab, for example: kill %d", port, process, inspection.PID)
		}
		return fmt.Errorf("instance port %s is already in use by %s; stop the process and restart PinchTab, for example: kill %d", port, process, inspection.PID)
	}
	return fmt.Errorf("instance port %s is already in use on this machine", port)
}

// buildChildFileConfig builds the per-child FileConfig; nil effectiveCfg falls back to o.runtimeCfg.
func (o *Orchestrator) buildChildFileConfig(effectiveCfg *config.RuntimeConfig, port string, cdpPort int, profilePath, instanceStateDir string, headless bool, extensionPaths []string, securityPolicy *bridge.SecurityPolicy) config.FileConfig {
	if effectiveCfg == nil {
		effectiveCfg = o.runtimeCfg
	}
	fc := config.FileConfigFromRuntime(effectiveCfg)
	fc.Server.Port = port
	fc.Server.StateDir = instanceStateDir
	activityEnabled := false
	fc.Observability.Activity.Enabled = &activityEnabled
	fc.Browser.ChromeDebugPort = intPtr(cdpPort)
	fc.Profiles.BaseDir = filepath.Dir(profilePath)
	fc.Profiles.DefaultProfile = filepath.Base(profilePath)
	if headless {
		fc.InstanceDefaults.Mode = "headless"
	} else {
		fc.InstanceDefaults.Mode = "headed"
	}
	if securityPolicy != nil {
		fc.Security.AllowedDomains = append([]string(nil), securityPolicy.AllowedDomains...)
	}

	if len(extensionPaths) > 0 {
		seen := make(map[string]bool)
		unique := make([]string, 0, len(fc.Browser.ExtensionPaths)+len(extensionPaths))
		for _, p := range fc.Browser.ExtensionPaths {
			if !seen[p] {
				seen[p] = true
				unique = append(unique, p)
			}
		}
		for _, p := range extensionPaths {
			if !seen[p] {
				seen[p] = true
				unique = append(unique, p)
			}
		}
		fc.Browser.ExtensionPaths = unique
	}
	return fc
}

func (o *Orchestrator) writeChildConfig(effectiveCfg *config.RuntimeConfig, port string, cdpPort int, profilePath, instanceStateDir string, headless bool, extensionPaths []string, securityPolicy *bridge.SecurityPolicy) (string, error) {
	fc := o.buildChildFileConfig(effectiveCfg, port, cdpPort, profilePath, instanceStateDir, headless, extensionPaths, securityPolicy)

	configPath := filepath.Join(instanceStateDir, "config.json")
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return "", err
	}
	if err := os.Chmod(configPath, 0600); err != nil {
		return "", err
	}
	return configPath, nil
}

// writeAttachChildConfig writes a minimal child config for a CDP-attach bridge; RemoteCDPURL is passed via CLI flags.
func (o *Orchestrator) writeAttachChildConfig(port, provider, stateDir string) (string, error) {
	fc := config.FileConfigFromRuntime(o.runtimeCfg)
	fc.Server.Port = port
	fc.Server.StateDir = stateDir
	activityEnabled := false
	fc.Observability.Activity.Enabled = &activityEnabled
	fc.Browser.Provider = provider
	attachDisabled := false
	allowHosts := append([]string(nil), fc.Security.Attach.AllowHosts...)
	allowSchemes := append([]string(nil), fc.Security.Attach.AllowSchemes...)
	fc.Security.Attach = config.AttachConfig{
		Enabled:          &attachDisabled,
		AllowHosts:       allowHosts,
		AllowSchemes:     allowSchemes,
		ForwardProxyAuth: &attachDisabled,
	}

	configPath := filepath.Join(stateDir, "config.json")
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return "", err
	}
	if err := os.Chmod(configPath, 0600); err != nil {
		return "", err
	}
	return configPath, nil
}

func effectiveSecurityPolicy(cfg *config.RuntimeConfig, requested *bridge.SecurityPolicy) *bridge.SecurityPolicy {
	var merged []string
	if cfg != nil {
		merged = mergeAllowedDomains(merged, cfg.AllowedDomains)
	}
	if requested != nil {
		merged = mergeAllowedDomains(merged, requested.AllowedDomains)
	}
	if len(merged) == 0 {
		return nil
	}
	return &bridge.SecurityPolicy{AllowedDomains: merged}
}

func cloneSecurityPolicy(policy *bridge.SecurityPolicy) *bridge.SecurityPolicy {
	if policy == nil {
		return nil
	}
	return &bridge.SecurityPolicy{
		AllowedDomains: append([]string(nil), policy.AllowedDomains...),
	}
}

func mergeAllowedDomains(base []string, extras []string) []string {
	seen := make(map[string]bool, len(base)+len(extras))
	out := make([]string, 0, len(base)+len(extras))
	for _, domain := range base {
		trimmed := strings.TrimSpace(domain)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	for _, domain := range extras {
		trimmed := strings.TrimSpace(domain)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func effectiveBinaryFromCfg(cfg *config.RuntimeConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.ChromeBinary
}

func intPtr(v int) *int {
	if v <= 0 {
		return nil
	}
	n := v
	return &n
}

// attachExternalInstance registers an external instance or updates an existing
// bridge in place (upsert). Non-bridge duplicates still return an error.
func (o *Orchestrator) attachExternalInstance(name string, inst bridge.Instance, authToken string) (*bridge.Instance, bool, error) {
	o.mu.Lock()
	for _, existing := range o.instances {
		if existing.ProfileName == name && instanceIsActive(existing) {
			if existing.Attached && inst.AttachType == "bridge" && existing.AttachType == "bridge" {
				if existing.authToken != "" && subtle.ConstantTimeCompare([]byte(existing.authToken), []byte(authToken)) != 1 {
					o.mu.Unlock()
					return nil, false, fmt.Errorf("bridge %q already attached: token mismatch", name)
				}
				existing.URL = inst.URL
				existing.Instance.URL = inst.URL
				existing.authToken = authToken
				existing.Status = "running"
				existing.Error = ""
				existing.StartTime = time.Now()
				if inst.BrowserTarget != "" {
					existing.BrowserTarget = inst.BrowserTarget
				}
				if inst.BrowserProvider != "" {
					existing.BrowserProvider = inst.BrowserProvider
				}
				result := existing.Instance
				o.mu.Unlock()

				o.syncInstanceToManager(&result)
				return &result, false, nil
			}
			o.mu.Unlock()
			return nil, false, fmt.Errorf("instance with name %q already exists", name)
		}
	}
	o.mu.Unlock()

	profileID := o.idMgr.ProfileID(name)
	instanceID := o.idMgr.InstanceID(profileID, name)
	inst.ID = instanceID
	inst.ProfileID = profileID
	inst.ProfileName = name
	inst.Status = "running"
	inst.StartTime = time.Now()
	internal := &InstanceInternal{
		Instance:  inst,
		URL:       inst.URL,
		authToken: authToken,
	}

	o.mu.Lock()
	o.instances[instanceID] = internal
	o.mu.Unlock()

	o.syncInstanceToManager(&internal.Instance)
	return &internal.Instance, true, nil
}

// Attach wraps an externally-managed browser via a child bridge; the external process is never killed on Stop.
func (o *Orchestrator) Attach(name, cdpURL string) (*bridge.Instance, error) {
	return o.AttachWithProvider(name, cdpURL, "")
}

// AttachWithProvider is Attach with an explicit provider ("" defaults to chrome).
func (o *Orchestrator) AttachWithProvider(name, cdpURL, provider string) (*bridge.Instance, error) {
	return o.AttachWithOptions(name, cdpURL, AttachOptions{BrowserProvider: provider})
}

// AttachWithOptions wraps an externally-managed browser and records its resolved browser target.
func (o *Orchestrator) AttachWithOptions(name, cdpURL string, opts AttachOptions) (*bridge.Instance, error) {
	if err := profiles.ValidateProfileName(name); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cdpURL) == "" {
		return nil, fmt.Errorf("cdpUrl is required")
	}
	if err := o.validateAttachURL(cdpURL); err != nil {
		return nil, err
	}
	resolved, err := o.resolveAttachOptions(opts, config.BrowserProviderChrome)
	if err != nil {
		return nil, err
	}

	bridgePort, err := o.portAllocator.AllocatePort()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate bridge port: %w", err)
	}
	released := false
	defer func() {
		if !released {
			o.portAllocator.ReleasePort(bridgePort)
		}
	}()
	portStr := strconv.Itoa(bridgePort)

	profileID := o.idMgr.ProfileID(name)
	instanceID := o.idMgr.InstanceID(profileID, name)

	o.mu.Lock()
	for _, existing := range o.instances {
		if existing.ProfileName == name && instanceIsActive(existing) {
			o.mu.Unlock()
			return nil, fmt.Errorf("instance with name %q already exists", name)
		}
	}
	o.mu.Unlock()

	// RemoteCDPURL/RemoteBrowserName are passed via CLI flags rather than persisted.
	stateDir := filepath.Join(o.baseDir, "attach", instanceID)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("create attach state dir: %w", err)
	}
	if err := os.Chmod(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("set attach state dir permissions: %w", err)
	}
	childConfigPath, err := o.writeAttachChildConfig(portStr, resolved.BrowserProvider, stateDir)
	if err != nil {
		return nil, fmt.Errorf("write attach child config: %w", err)
	}

	envOverrides := map[string]string{
		"PINCHTAB_PORT":   portStr,
		"PINCHTAB_CONFIG": childConfigPath,
	}
	if o.internalToken != "" {
		envOverrides["PINCHTAB_INTERNAL_TOKEN"] = o.internalToken
	}
	env := mergeEnvWithOverrides(filterEnvWithPrefixes(os.Environ(), "PINCHTAB_"), envOverrides)

	logBuf := newRingBuffer(256 * 1024)
	args := []string{
		"bridge",
		"--cdp-attach", cdpURL,
		"--browser-provider", resolved.BrowserProvider,
		"--remote-browser-name", name,
	}
	slog.Info("starting CDP attach bridge child",
		"id", instanceID, "name", name, "port", portStr, "provider", resolved.BrowserProvider, "browserTarget", resolved.BrowserTarget,
		"cdpUrl", internalurls.RedactForLog(cdpURL),
	)
	cmd, err := o.runner.Run(context.Background(), o.binary, args, env, logBuf, logBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to start CDP attach bridge: %w", err)
	}

	baseURL := o.childInstanceBaseURL(portStr)
	internal := &InstanceInternal{
		Instance: bridge.Instance{
			ID:              instanceID,
			ProfileID:       profileID,
			ProfileName:     name,
			Port:            portStr,
			URL:             baseURL,
			Mode:            bridge.ModeFromHeadless(false),
			Headless:        false,
			Status:          "starting",
			StartTime:       time.Now(),
			Attached:        true,
			AttachType:      "cdp-bridge",
			CdpURL:          cdpURL,
			BrowserTarget:   resolved.BrowserTarget,
			BrowserProvider: resolved.BrowserProvider,
		},
		URL:    baseURL,
		cmd:    cmd,
		logBuf: logBuf,

		requestedBrowserTarget: strings.TrimSpace(opts.BrowserTarget),
		resolvedBrowserTarget:  resolved.BrowserTarget,
		browserProvider:        resolved.BrowserProvider,
	}

	o.mu.Lock()
	o.instances[instanceID] = internal
	o.mu.Unlock()
	released = true

	go o.monitor(internal)

	healthTimeout := o.attachHealthCheckTimeout
	if healthTimeout <= 0 {
		healthTimeout = 15 * time.Second
	}
	if err := waitForChildBridgeHealthyFunc(o, internal, healthTimeout); err != nil {
		tail := tailChildLog(logBuf, 20)
		slog.Warn("CDP attach bridge did not become healthy in time; tearing down child bridge", "id", instanceID, "err", err, "childLogTail", tail)
		if stopErr := o.Stop(instanceID); stopErr != nil {
			slog.Warn("failed to tear down unhealthy CDP attach bridge", "id", instanceID, "err", stopErr)
		}
		if tail != "" {
			return nil, fmt.Errorf("CDP attach bridge did not become healthy: %w; child log tail:\n%s", err, tail)
		}
		return nil, fmt.Errorf("CDP attach bridge did not become healthy: %w", err)
	}

	result := internal.Instance
	slog.Info("attached external browser via CDP bridge",
		"id", result.ID, "name", name, "provider", resolved.BrowserProvider, "browserTarget", resolved.BrowserTarget,
		"url", internalurls.RedactForLog(result.URL),
		"cdpUrl", internalurls.RedactForLog(cdpURL),
	)
	o.emitEvent("instance.attached", &result)
	return &result, nil
}

// waitForChildBridgeHealthy polls the child /health until OK or the deadline elapses.
func (o *Orchestrator) waitForChildBridgeHealthy(inst *InstanceInternal, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := strings.TrimRight(inst.URL, "/") + "/health"
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, healthURL, nil)
		o.applyInstanceAuth(req, inst)
		resp, err := o.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				o.mu.Lock()
				inst.Status = "running"
				o.mu.Unlock()
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("child bridge health check timed out after %v", timeout)
}

// AttachBridge registers an already-running bridge server as an attached instance.
// If a bridge with the same name is already attached, it is updated in place (upsert)
// provided the caller presents the current bridge token.
func (o *Orchestrator) AttachBridge(name, baseURL, token string) (*bridge.Instance, bool, error) {
	return o.AttachBridgeWithOptions(name, baseURL, token, AttachOptions{})
}

// AttachBridgeWithOptions registers an already-running bridge server with target metadata.
func (o *Orchestrator) AttachBridgeWithOptions(name, baseURL, token string, opts AttachOptions) (*bridge.Instance, bool, error) {
	if err := o.validateAttachURL(baseURL); err != nil {
		return nil, false, err
	}
	preserveExistingTarget := strings.TrimSpace(opts.BrowserTarget) == "" &&
		strings.TrimSpace(opts.BrowserProvider) == "" &&
		o.hasActiveAttachedBridge(name)
	resolved, err := o.resolveAttachOptions(opts, "")
	if err != nil {
		return nil, false, err
	}
	if preserveExistingTarget {
		resolved.BrowserTarget = ""
		resolved.BrowserProvider = ""
	}

	normalizedBaseURL := strings.TrimRight(baseURL, "/")
	if parsed, err := url.Parse(normalizedBaseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		normalizedBaseURL = parsed.Scheme + "://" + parsed.Host
	}

	inst, created, err := o.attachExternalInstance(name, bridge.Instance{
		Attached:        true,
		AttachType:      "bridge",
		URL:             normalizedBaseURL,
		Mode:            bridge.ModeFromHeadless(false),
		BrowserTarget:   resolved.BrowserTarget,
		BrowserProvider: resolved.BrowserProvider,
	}, token)
	if err != nil {
		return nil, false, err
	}

	slog.Info("attached to external bridge",
		"id", inst.ID, "name", name, "browserTarget", inst.BrowserTarget, "provider", inst.BrowserProvider,
		"url", internalurls.RedactForLog(inst.URL),
	)
	o.emitEvent("instance.attached", inst)
	if created {
		o.mu.RLock()
		internal := o.instances[inst.ID]
		o.mu.RUnlock()
		if internal != nil {
			go o.monitorAttachedBridge(internal)
		}
	}
	return inst, created, nil
}

func (o *Orchestrator) hasActiveAttachedBridge(name string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	for _, existing := range o.instances {
		if existing.ProfileName == name && instanceIsActive(existing) && existing.Attached && existing.AttachType == "bridge" {
			return true
		}
	}
	return false
}

func (o *Orchestrator) resolveAttachOptions(opts AttachOptions, defaultProvider string) (AttachOptions, error) {
	requestedTarget := strings.TrimSpace(opts.BrowserTarget)
	requestedProvider := strings.TrimSpace(opts.BrowserProvider)

	explicitProvider := requestedProvider != ""
	parsedProvider := ""
	if explicitProvider {
		provider, err := config.ParseBrowserProvider(requestedProvider)
		if err != nil {
			return AttachOptions{}, err
		}
		parsedProvider = provider
	}

	if o.runtimeCfg == nil || len(o.runtimeCfg.Targets) == 0 {
		if parsedProvider == "" && defaultProvider != "" {
			provider, err := config.ParseBrowserProvider(defaultProvider)
			if err != nil {
				return AttachOptions{}, err
			}
			parsedProvider = provider
		}
		if requestedTarget != "" {
			return AttachOptions{}, fmt.Errorf("no browser targets configured; cannot resolve %q", requestedTarget)
		}
		return AttachOptions{BrowserProvider: parsedProvider}, nil
	}

	var resolved *config.ResolvedBrowserTarget
	var err error
	if strings.TrimSpace(requestedTarget) == "" {
		resolved, err = config.ResolveDefaultBrowserTarget(o.runtimeCfg)
	} else {
		resolved, err = config.ResolveExplicitBrowserTarget(o.runtimeCfg, requestedTarget)
	}
	if err != nil {
		return AttachOptions{}, err
	}
	target, targetProvider := "", ""
	if resolved != nil && !resolved.Legacy {
		target = resolved.Name
		targetProvider = resolved.Provider
	}
	if explicitProvider && targetProvider != "" && parsedProvider != targetProvider {
		return AttachOptions{}, fmt.Errorf("browser provider %q conflicts with browserTarget %q provider %q", parsedProvider, target, targetProvider)
	}
	if targetProvider == "" {
		targetProvider = parsedProvider
	}
	if targetProvider == "" {
		targetProvider = config.BrowserProviderChrome
	}
	return AttachOptions{BrowserTarget: target, BrowserProvider: targetProvider}, nil
}

func (o *Orchestrator) Stop(id string) error {
	o.mu.Lock()
	inst, ok := o.instances[id]
	if !ok {
		o.mu.Unlock()
		return fmt.Errorf("instance %q not found", id)
	}
	if inst.Status == "stopped" && !instanceIsActive(inst) {
		o.mu.Unlock()
		o.markStopped(id)
		return nil
	}
	inst.Status = "stopping"
	o.mu.Unlock()

	if inst.cmd == nil {
		if inst.AttachType == "bridge" {
			reqCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			targetURL, targetErr := o.instancePathURL(inst, "/shutdown", "")
			if targetErr == nil {
				req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, targetURL.String(), nil)
				o.applyInstanceAuth(req, inst)
				if resp, err := o.client.Do(req); err == nil {
					_ = resp.Body.Close()
				}
			}
		}
		o.markStopped(id)
		return nil
	}

	pid := inst.cmd.PID()

	reqCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if targetURL, targetErr := o.instancePathURL(inst, "/shutdown", ""); targetErr == nil {
		req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, targetURL.String(), nil)
		o.applyInstanceAuth(req, inst)
		resp, err := o.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}

	if pid > 0 {
		if waitForProcessExit(pid, 5*time.Second) {
			o.markStopped(id)
			return nil
		}

		if err := killProcessGroup(pid, sigTERM); err != nil {
			slog.Warn("failed to send SIGTERM to instance", "id", id, "pid", pid, "err", err)
		}
		if waitForProcessExit(pid, 3*time.Second) {
			o.markStopped(id)
			return nil
		}

		if err := killProcessGroup(pid, sigKILL); err != nil {
			slog.Warn("failed to send SIGKILL to instance", "id", id, "pid", pid, "err", err)
		}
	}

	inst.cmd.Cancel()

	if pid > 0 {
		if waitForProcessExit(pid, 2*time.Second) {
			o.markStopped(id)
			return nil
		}
		o.setStopError(id, fmt.Sprintf("failed to stop process %d; still running", pid))
		return fmt.Errorf("failed to stop instance %q gracefully", id)
	}

	o.markStopped(id)
	return nil
}

func (o *Orchestrator) StopProfile(name string) error {
	o.mu.RLock()
	ids := make([]string, 0, 1)
	for id, inst := range o.instances {
		if inst.ProfileName == name && instanceIsActive(inst) {
			ids = append(ids, id)
		}
	}
	o.mu.RUnlock()

	if len(ids) == 0 {
		return fmt.Errorf("no active instance for profile %q", name)
	}

	var errs []string
	for _, id := range ids {
		if err := o.Stop(id); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to stop profile %q: %s", name, strings.Join(errs, "; "))
	}
	return nil
}

func (o *Orchestrator) markStopped(id string) {
	o.mu.Lock()
	inst, ok := o.instances[id]
	if !ok {
		o.mu.Unlock()
		return
	}

	profileName := inst.ProfileName
	delete(o.instances, id)
	o.mu.Unlock()

	o.releaseInstancePorts(id, inst)
	o.removeInstanceFromManager(id)

	slog.Info("instance stopped and removed", "id", id, "profile", profileName)
	o.cleanupStoppedProfile(profileName)
}

func (o *Orchestrator) releaseInstancePorts(id string, inst *InstanceInternal) {
	if o == nil || o.portAllocator == nil || inst == nil {
		return
	}
	portStr := inst.Port
	if portInt, err := strconv.Atoi(portStr); err == nil {
		o.portAllocator.ReleasePort(portInt)
		slog.Debug("released port", "id", id, "port", portStr)
	}
	if inst.cdpPort > 0 {
		o.portAllocator.ReleasePort(inst.cdpPort)
		slog.Debug("released chrome debug port", "id", id, "port", inst.cdpPort)
	}
}

func (o *Orchestrator) removeInstanceFromManager(id string) {
	if o != nil && o.instanceMgr != nil {
		o.instanceMgr.Locator.InvalidateInstance(id)
		o.instanceMgr.Repo.Remove(id)
	}
}

func (o *Orchestrator) cleanupStoppedProfile(profileName string) {
	// Kill any orphaned Chrome processes using this profile's directory.
	// Chrome spawns helpers (GPU, renderer) in their own process groups,
	// so killing the bridge process group doesn't reach them.
	profilePath := filepath.Join(o.baseDir, profileName)
	bridge.CleanupOrphanedChromeProcesses(profilePath)

	if strings.HasPrefix(profileName, "instance-") {
		profilePath := filepath.Join(o.baseDir, profileName)
		if err := os.RemoveAll(profilePath); err != nil {
			slog.Warn("failed to delete temporary profile directory", "name", profileName, "err", err)
		} else {
			slog.Info("deleted temporary profile", "name", profileName)
		}

		if o.profiles != nil {
			if err := o.profiles.Delete(profileName); err != nil {
				slog.Warn("failed to delete profile metadata", "name", profileName, "err", err)
			}
		}
	}
}

func (o *Orchestrator) setStopError(id, msg string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if inst, ok := o.instances[id]; ok {
		inst.Status = "error"
		inst.Error = msg
	}
}

func (o *Orchestrator) List() []bridge.Instance {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]bridge.Instance, 0, len(o.instances))
	for _, inst := range o.instances {
		copyInst := inst.Instance
		if instanceIsActive(inst) && copyInst.Status == "stopped" {
			copyInst.Status = "running"
		}
		if !instanceIsActive(inst) &&
			(copyInst.Status == "starting" || copyInst.Status == "running" || copyInst.Status == "stopping") {
			copyInst.Status = "stopped"
		}

		result = append(result, copyInst)
	}
	return result
}

func (o *Orchestrator) Logs(id string) (string, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	inst, ok := o.instances[id]
	if !ok {
		return "", fmt.Errorf("instance %q not found", id)
	}
	if inst.logBuf == nil {
		return "", nil
	}
	return inst.logBuf.String(), nil
}

func (o *Orchestrator) FirstRunningURL() string {
	return o.firstRunningURL(nil)
}

func (o *Orchestrator) FirstRunningURLForBrowserTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return o.FirstRunningURL()
	}
	return o.firstRunningURL(func(inst *InstanceInternal) bool {
		return inst != nil && inst.BrowserTarget == target
	})
}

func (o *Orchestrator) firstRunningURL(match func(*InstanceInternal) bool) string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	// Collect running instances and sort by start time for determinism.
	// This works for both local launched instances and attached remote bridges.
	type candidate struct {
		start time.Time
		url   string
	}
	var candidates []candidate
	for _, inst := range o.instances {
		if inst.Status == "running" && instanceIsActive(inst) {
			if inst.URL == "" {
				continue
			}
			if match != nil && !match(inst) {
				continue
			}
			candidates = append(candidates, candidate{start: inst.StartTime, url: inst.URL})
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].start.Equal(candidates[j].start) {
			return candidates[i].url < candidates[j].url
		}
		return candidates[i].start.Before(candidates[j].start)
	})
	return candidates[0].url
}

func (o *Orchestrator) FirstRunningURLForRequest(r *http.Request) (string, int, error) {
	requested := ExtractRequestedBrowserTarget(r)
	if requested == "" {
		resolved, err := config.ResolveDefaultBrowserTarget(o.runtimeCfg)
		if err != nil {
			return "", http.StatusBadRequest, err
		}
		if resolved != nil && !resolved.Legacy {
			if resolved.Name == "" {
				return "", http.StatusBadRequest, fmt.Errorf("no default browser target configured and none requested")
			}
			return o.FirstRunningURLForBrowserTarget(resolved.Name), 0, nil
		}
		return o.FirstRunningURL(), 0, nil
	}
	resolved, err := config.ResolveExplicitBrowserTarget(o.runtimeCfg, requested)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	target := resolved.Name
	if url := o.FirstRunningURLForBrowserTarget(target); url != "" {
		return url, 0, nil
	}
	return "", http.StatusConflict, fmt.Errorf("no running instance for browserTarget %q", target)
}

// instanceTabsCached returns the tab list for inst using the per-instance
// cache. fresh=true forces a bypass and refresh. Returned tabs are
// InstanceTab-shaped (with InstanceID set) so callers can hand them
// straight to JSON encoders.
func (o *Orchestrator) instanceTabsCached(inst *InstanceInternal, fresh bool) ([]bridge.InstanceTab, error) {
	if inst == nil {
		return nil, fmt.Errorf("nil instance")
	}
	if !fresh {
		if cached, ok := o.tabsCache.Get(inst.ID); ok {
			return cached, nil
		}
	}
	tabs, err := o.fetchTabs(inst)
	if err != nil {
		return nil, err
	}
	out := make([]bridge.InstanceTab, 0, len(tabs))
	for _, tab := range tabs {
		out = append(out, bridge.InstanceTab{
			ID:         tab.ID,
			InstanceID: inst.ID,
			URL:        tab.URL,
			Title:      tab.Title,
		})
	}
	o.tabsCache.Set(inst.ID, out)
	return out, nil
}

func (o *Orchestrator) AllTabs() []bridge.InstanceTab {
	return o.allTabs(false)
}

func (o *Orchestrator) allTabs(fresh bool) []bridge.InstanceTab {
	o.mu.RLock()
	instances := make([]*InstanceInternal, 0)
	for _, inst := range o.instances {
		if inst.Status == "running" && instanceIsActive(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	all := make([]bridge.InstanceTab, 0)
	for _, inst := range instances {
		tabs, err := o.instanceTabsCached(inst, fresh)
		if err != nil {
			continue
		}
		all = append(all, tabs...)
	}
	return all
}

// FindInstanceByTab returns the running instance owning tabID, or (nil,false).
func (o *Orchestrator) FindInstanceByTab(tabID string) (*bridge.Instance, bool) {
	if tabID == "" {
		return nil, false
	}

	o.mu.RLock()
	instances := make([]*InstanceInternal, 0, len(o.instances))
	for _, inst := range o.instances {
		if inst.Status == "running" && instanceIsActive(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	for _, inst := range instances {
		tabs, err := o.instanceTabsCached(inst, false)
		if err != nil {
			continue
		}
		for _, tab := range tabs {
			if tab.ID == tabID {
				copyInst := inst.Instance
				return &copyInst, true
			}
		}
	}
	return nil, false
}

func (o *Orchestrator) AllMetrics() []types.InstanceMetrics {
	o.mu.RLock()
	instances := make([]*InstanceInternal, 0)
	for _, inst := range o.instances {
		if inst.Status == "running" && instanceIsActive(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	all := make([]types.InstanceMetrics, 0)
	for _, inst := range instances {
		mem, err := o.fetchMetrics(inst)
		if err != nil || mem == nil {
			continue
		}
		all = append(all, types.InstanceMetrics{
			InstanceID:    inst.ID,
			ProfileName:   inst.ProfileName,
			JSHeapUsedMB:  mem.JSHeapUsedMB,
			JSHeapTotalMB: mem.JSHeapTotalMB,
			Documents:     mem.Documents,
			Frames:        mem.Frames,
			Nodes:         mem.Nodes,
			Listeners:     mem.Listeners,
		})
	}
	return all
}

func (o *Orchestrator) ScreencastURL(instanceID, tabID string) string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	inst, ok := o.instances[instanceID]
	if !ok {
		return ""
	}
	target, err := o.instancePathURL(inst, "/screencast", "tabId="+url.QueryEscape(tabID))
	if err != nil {
		return ""
	}
	switch target.Scheme {
	case "https":
		target.Scheme = "wss"
	default:
		target.Scheme = "ws"
	}
	return target.String()
}

func (o *Orchestrator) Shutdown() {
	o.mu.RLock()
	ids := make([]string, 0, len(o.instances))
	for id, inst := range o.instances {
		if instanceIsActive(inst) {
			ids = append(ids, id)
		}
	}
	o.mu.RUnlock()

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(instanceID string) {
			defer wg.Done()
			slog.Info("stopping instance", "id", instanceID)
			if err := o.Stop(instanceID); err != nil {
				slog.Warn("stop instance failed", "id", instanceID, "err", err)
			}
		}(id)
	}
	wg.Wait()
}

func (o *Orchestrator) ForceShutdown() {
	o.mu.RLock()
	instances := make([]*InstanceInternal, 0, len(o.instances))
	for _, inst := range o.instances {
		if instanceIsActive(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	for _, inst := range instances {
		pid := 0
		if inst.cmd != nil {
			pid = inst.cmd.PID()
			inst.cmd.Cancel()
		}
		if pid > 0 {
			_ = killProcessGroup(pid, sigKILL)
		}
		o.markStopped(inst.ID)
	}
}
