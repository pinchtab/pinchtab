package orchestrator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

type Orchestrator struct {
	instances      map[string]*InstanceInternal
	baseDir        string
	binary         string
	profiles       *profiles.ProfileManager
	runner         HostRunner
	mu             sync.RWMutex
	client         *http.Client
	childAuthToken string
}

type InstanceInternal struct {
	bridge.Instance
	URL   string
	Error string

	cmd    Cmd
	logBuf *ringBuffer
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
			slog.Info("installed pinchtab binary", "path", stableBin)
		}
	}

	if _, err := os.Stat(binary); err != nil {
		if _, stableErr := os.Stat(stableBin); stableErr == nil {
			binary = stableBin
		}
	}

	return &Orchestrator{
		instances:      make(map[string]*InstanceInternal),
		baseDir:        baseDir,
		binary:         binary,
		runner:         runner,
		client:         &http.Client{Timeout: 3 * time.Second},
		childAuthToken: os.Getenv("BRIDGE_TOKEN"),
	}
}

func (o *Orchestrator) SetProfileManager(pm *profiles.ProfileManager) {
	o.profiles = pm
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

func (o *Orchestrator) Launch(name, port string, headless bool) (*bridge.Instance, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, inst := range o.instances {
		if inst.Port == port && instanceIsActive(inst) {
			return nil, fmt.Errorf("port %s already in use by instance %q", port, inst.Name)
		}
		if inst.Name == name && instanceIsActive(inst) {
			return nil, fmt.Errorf("profile %q already has an active instance (%s)", name, inst.Status)
		}
	}
	if !o.runner.IsPortAvailable(port) {
		return nil, fmt.Errorf("port %s is already in use on this machine", port)
	}

	id := fmt.Sprintf("%s-%s", name, port)
	if inst, ok := o.instances[id]; ok && inst.Status == "running" {
		return nil, fmt.Errorf("instance %q already running", id)
	}

	profilePath := filepath.Join(o.baseDir, name)
	if err := os.MkdirAll(filepath.Join(profilePath, "Default"), 0755); err != nil {
		return nil, fmt.Errorf("create profile dir: %w", err)
	}
	instanceStateDir := filepath.Join(profilePath, ".pinchtab-state")
	if err := os.MkdirAll(instanceStateDir, 0755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	headlessStr := "true"
	if !headless {
		headlessStr = "false"
	}

	env := mergeEnvWithOverrides(os.Environ(), map[string]string{
		"BRIDGE_PORT":       port,
		"BRIDGE_PROFILE":    profilePath,
		"BRIDGE_STATE_DIR":  instanceStateDir,
		"BRIDGE_HEADLESS":   headlessStr,
		"BRIDGE_NO_RESTORE": "true",
	})

	logBuf := newRingBuffer(64 * 1024)
	slog.Info("starting instance process", "id", id, "binary", o.binary, "port", port, "profile", profilePath)

	cmd, err := o.runner.Run(context.Background(), o.binary, env, logBuf, logBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to start: %w", err)
	}

	inst := &InstanceInternal{
		Instance: bridge.Instance{
			ID:        id,
			Name:      name,
			Profile:   profilePath,
			Port:      port,
			Headless:  headless,
			Status:    "starting",
			StartTime: time.Now(),
		},
		URL:    fmt.Sprintf("http://localhost:%s", port),
		cmd:    cmd,
		logBuf: logBuf,
	}

	o.instances[id] = inst

	go o.monitor(inst)

	return &inst.Instance, nil
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
		return nil
	}
	inst.Status = "stopping"
	o.mu.Unlock()

	if inst.cmd == nil {
		o.markStopped(id)
		return nil
	}

	pid := inst.cmd.PID()

	reqCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, inst.URL+"/shutdown", nil)
	resp, err := o.client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
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
		if inst.Name == name && instanceIsActive(inst) {
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
	defer o.mu.Unlock()
	if inst, ok := o.instances[id]; ok {
		inst.Status = "stopped"
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
	return inst.logBuf.String(), nil
}

func (o *Orchestrator) FirstRunningURL() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	for _, inst := range o.instances {
		if inst.Status == "running" && instanceIsActive(inst) {
			return inst.URL
		}
	}
	return ""
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

	var all []bridge.InstanceTab
	for _, inst := range instances {
		tabs, err := o.fetchTabs(inst.URL)
		if err != nil {
			continue
		}
		for _, tab := range tabs {
			all = append(all, bridge.InstanceTab{
				InstanceID: inst.ID,
				TabID:      tab.ID,
				URL:        tab.URL,
			})
		}
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
	return fmt.Sprintf("ws://localhost:%s/screencast?tabId=%s", inst.Port, tabID)
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
