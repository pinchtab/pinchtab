package orchestrator

import (
	"context"
	"crypto/subtle"
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
	portAllocator  *PortAllocator
	idMgr          *ids.Manager
	eventHandlers  []EventHandler
	instanceMgr    *instance.Manager
	runtimeCfg     *config.RuntimeConfig
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
		portAllocator:  NewPortAllocator(9868, 9968),
		idMgr:          ids.NewManager(),
	}

	bridgeClient := instance.NewBridgeClient()
	orch.instanceMgr = instance.NewManager(
		&orchestratorLauncher{orch: orch},
		bridgeClient,
	)

	return orch
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

func (o *Orchestrator) AllowsUpload() bool {
	return o != nil && o.runtimeCfg != nil && o.runtimeCfg.AllowUpload
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
	if !o.runner.IsPortAvailable(port) {
		o.mu.Unlock()
		return nil, fmt.Errorf("port %s is already in use on this machine", port)
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
	if err := os.MkdirAll(instanceStateDir, 0755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	childConfigPath, err := o.writeChildConfig(port, cdpPort, profilePath, instanceStateDir, headless, extensionPaths)
	if err != nil {
		return nil, fmt.Errorf("write child config: %w", err)
	}

	envOverrides := map[string]string{
		"PINCHTAB_PORT":   port,
		"PINCHTAB_CONFIG": childConfigPath,
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
			ID:          instanceID,
			ProfileID:   profileID,
			ProfileName: name,
			Port:        port,
			URL:         fmt.Sprintf("http://localhost:%s", port),
			Headless:    headless,
			Status:      "starting",
			StartTime:   time.Now(),
		},
		URL:     fmt.Sprintf("http://localhost:%s", port),
		cdpPort: cdpPort,
		cmd:     cmd,
		logBuf:  logBuf,
	}

	o.mu.Lock()
	o.instances[instanceID] = inst
	o.mu.Unlock()
	reservedPorts = nil

	go o.monitor(inst)

	return &inst.Instance, nil
}

func (o *Orchestrator) writeChildConfig(port string, cdpPort int, profilePath, instanceStateDir string, headless bool, extensionPaths []string) (string, error) {
	fc := config.FileConfigFromRuntime(o.runtimeCfg)
	fc.Server.Port = port
	fc.Server.StateDir = instanceStateDir
	fc.Browser.ChromeDebugPort = intPtr(cdpPort)
	fc.Profiles.BaseDir = filepath.Dir(profilePath)
	fc.Profiles.DefaultProfile = filepath.Base(profilePath)
	if headless {
		fc.InstanceDefaults.Mode = "headless"
	} else {
		fc.InstanceDefaults.Mode = "headed"
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

	configPath := filepath.Join(instanceStateDir, "config.json")
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "", err
	}
	return configPath, nil
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

// Attach connects to an externally managed Chrome instance via CDP URL.
// Unlike Launch, this does not start a Chrome process - it only registers
// the external instance for tracking and proxying.
func (o *Orchestrator) Attach(name, cdpURL string) (*bridge.Instance, error) {
	inst, _, err := o.attachExternalInstance(name, bridge.Instance{
		Attached:   true,
		AttachType: "cdp",
		CdpURL:     cdpURL,
		URL:        cdpURL,
	}, "")
	if err != nil {
		return nil, err
	}

	slog.Info("attached to external Chrome", "id", inst.ID, "name", name, "cdpUrl", internalurls.RedactForLog(cdpURL))

	// Emit event
	o.emitEvent("instance.attached", inst)
	return inst, nil
}

// AttachBridge registers an already-running bridge server as an attached instance.
// If a bridge with the same name is already attached, it is updated in place (upsert)
// provided the caller presents the current bridge token.
func (o *Orchestrator) AttachBridge(name, baseURL, token string) (*bridge.Instance, bool, error) {
	normalizedBaseURL := strings.TrimRight(baseURL, "/")
	if parsed, err := url.Parse(normalizedBaseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		normalizedBaseURL = parsed.Scheme + "://" + parsed.Host
	}

	inst, created, err := o.attachExternalInstance(name, bridge.Instance{
		Attached:   true,
		AttachType: "bridge",
		URL:        normalizedBaseURL,
	}, token)
	if err != nil {
		return nil, false, err
	}

	slog.Info("attached to external bridge", "id", inst.ID, "name", name, "url", internalurls.RedactForLog(inst.URL))
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

	portStr := inst.Port
	if portInt, err := strconv.Atoi(portStr); err == nil {
		o.portAllocator.ReleasePort(portInt)
		slog.Debug("released port", "id", id, "port", portStr)
	}
	if inst.cdpPort > 0 {
		o.portAllocator.ReleasePort(inst.cdpPort)
		slog.Debug("released chrome debug port", "id", id, "port", inst.cdpPort)
	}

	profileName := inst.ProfileName
	delete(o.instances, id)
	o.mu.Unlock()

	if o.instanceMgr != nil {
		o.instanceMgr.Locator.InvalidateInstance(id)
		o.instanceMgr.Repo.Remove(id)
	}

	slog.Info("instance stopped and removed", "id", id, "profile", profileName)

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

func (o *Orchestrator) AllTabs() []bridge.InstanceTab {
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
		tabs, err := o.fetchTabs(inst)
		if err != nil {
			continue
		}
		for _, tab := range tabs {
			all = append(all, bridge.InstanceTab{
				ID:         tab.ID,
				InstanceID: inst.ID,
				URL:        tab.URL,
				Title:      tab.Title,
			})
		}
	}
	return all
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
