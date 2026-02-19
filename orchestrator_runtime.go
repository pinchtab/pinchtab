package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	instanceHealthPollInterval = 500 * time.Millisecond
	instanceStartupTimeout     = 45 * time.Second
)

func (o *Orchestrator) Launch(name, port string, headless bool) (*Instance, error) {
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
	if !isPortAvailable(port) {
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

	ctx, cancel := context.WithCancel(context.Background())

	headlessStr := "true"
	if !headless {
		headlessStr = "false"
	}

	cmd := exec.CommandContext(ctx, o.binary)
	cmd.Env = mergeEnvWithOverrides(os.Environ(), map[string]string{
		"BRIDGE_PORT":         port,
		"BRIDGE_PROFILE":      profilePath,
		"BRIDGE_STATE_DIR":    instanceStateDir,
		"BRIDGE_HEADLESS":     headlessStr,
		"BRIDGE_NO_RESTORE":   "true",
		"BRIDGE_NO_DASHBOARD": "true",
	})
	slog.Info("starting instance process", "id", id, "binary", o.binary, "port", port, "profile", profilePath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	logBuf := newRingBuffer(64 * 1024)
	cmd.Stdout = logBuf
	cmd.Stderr = logBuf

	inst := &Instance{
		ID:        id,
		Name:      name,
		Profile:   profilePath,
		Port:      port,
		Headless:  headless,
		Status:    "starting",
		StartedAt: time.Now(),
		URL:       fmt.Sprintf("http://localhost:%s", port),
		cmd:       cmd,
		cancel:    cancel,
		logBuf:    logBuf,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start: %w", err)
	}

	inst.PID = cmd.Process.Pid
	o.instances[id] = inst

	go o.monitor(inst)

	return inst, nil
}

func (o *Orchestrator) monitor(inst *Instance) {
	healthy := false
	exitedEarly := false
	lastProbe := "no response"
	resolvedURL := ""
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- inst.cmd.Wait()
	}()
	var waitErr error
	started := time.Now()
	for time.Since(started) < instanceStartupTimeout {
		select {
		case waitErr = <-waitCh:
			exitedEarly = true
		default:
		}
		if exitedEarly {
			break
		}
		time.Sleep(instanceHealthPollInterval)

		for _, baseURL := range instanceBaseURLs(inst.Port) {
			resp, err := o.client.Get(baseURL + "/health")
			if err == nil {
				_ = resp.Body.Close()
				lastProbe = fmt.Sprintf("%s -> HTTP %d", baseURL, resp.StatusCode)
				if isInstanceHealthyStatus(resp.StatusCode) {
					healthy = true
					resolvedURL = baseURL
					break
				}
			} else {
				lastProbe = fmt.Sprintf("%s -> %s", baseURL, err.Error())
			}
		}
		if healthy {
			break
		}
	}

	o.mu.Lock()
	switch inst.Status {
	case "stopping", "stopped":
	default:
		if healthy {
			inst.Status = "running"
			if resolvedURL != "" {
				inst.URL = resolvedURL
			}
			slog.Info("instance ready", "id", inst.ID, "port", inst.Port)
		} else if exitedEarly {
			inst.Status = "error"
			if waitErr != nil {
				inst.Error = "process exited before health check: " + waitErr.Error()
			} else {
				inst.Error = "process exited before health check succeeded"
			}
			if tail := tailLogLine(inst.logBuf.String()); tail != "" {
				inst.Error += " | " + tail
			}
			slog.Error("instance exited before ready", "id", inst.ID)
		} else {
			inst.Status = "error"
			inst.Error = fmt.Sprintf("health check timeout after %s (%s)", instanceStartupTimeout, lastProbe)
			if tail := tailLogLine(inst.logBuf.String()); tail != "" {
				inst.Error += " | " + tail
			}
			slog.Error("instance failed to start", "id", inst.ID)
		}
	}
	o.mu.Unlock()

	if !exitedEarly {
		waitErr = <-waitCh
	}
	o.mu.Lock()
	if inst.Status == "running" || inst.Status == "stopping" {
		inst.Status = "stopped"
		if waitErr != nil {
			inst.Error = waitErr.Error()
		}
	}
	o.mu.Unlock()
	slog.Info("instance exited", "id", inst.ID)
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

	if inst.cmd == nil || inst.cmd.Process == nil {
		o.markStopped(id)
		return nil
	}
	if inst.PID <= 0 {
		inst.cancel()
		o.markStopped(id)
		return nil
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, inst.URL+"/shutdown", nil)
	resp, err := o.client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}

	if waitForProcessExit(inst.PID, 5*time.Second) {
		o.markStopped(id)
		return nil
	}

	if err := syscall.Kill(-inst.PID, syscall.SIGTERM); err != nil {
		slog.Warn("failed to send SIGTERM to instance", "id", id, "pid", inst.PID, "err", err)
	}
	if waitForProcessExit(inst.PID, 3*time.Second) {
		o.markStopped(id)
		return nil
	}

	if err := syscall.Kill(-inst.PID, syscall.SIGKILL); err != nil {
		slog.Warn("failed to send SIGKILL to instance", "id", id, "pid", inst.PID, "err", err)
	}
	inst.cancel()
	if waitForProcessExit(inst.PID, 2*time.Second) {
		o.markStopped(id)
		return nil
	}

	o.setStopError(id, fmt.Sprintf("failed to stop process %d; still running", inst.PID))
	return fmt.Errorf("failed to stop instance %q gracefully", id)
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

func instanceIsActive(inst *Instance) bool {
	if inst == nil {
		return false
	}
	if inst.PID > 0 {
		return isProcessAlive(inst.PID)
	}
	return inst.Status == "starting" || inst.Status == "running" || inst.Status == "stopping"
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return !isProcessAlive(pid)
}

func isProcessAlive(pid int) bool {
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}

func (o *Orchestrator) List() []Instance {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]Instance, 0, len(o.instances))
	for _, inst := range o.instances {
		copyInst := *inst
		copyInst.cmd = nil
		copyInst.cancel = nil
		copyInst.logBuf = nil
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

func (o *Orchestrator) AllTabs() []instanceTab {
	o.mu.RLock()
	instances := make([]*Instance, 0)
	for _, inst := range o.instances {
		if inst.Status == "running" && instanceIsActive(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	var all []instanceTab
	for _, inst := range instances {
		tabs, err := o.fetchTabs(inst.URL)
		if err != nil {
			continue
		}
		for _, tab := range tabs {
			all = append(all, instanceTab{
				InstanceID:   inst.ID,
				InstanceName: inst.Name,
				InstancePort: inst.Port,
				TabID:        tab.ID,
				URL:          tab.URL,
			})
		}
	}
	return all
}

type instanceTab struct {
	InstanceID   string `json:"instanceId"`
	InstanceName string `json:"instanceName"`
	InstancePort string `json:"instancePort"`
	TabID        string `json:"tabId"`
	URL          string `json:"url"`
}

type remoteTab struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func (o *Orchestrator) fetchTabs(baseURL string) ([]remoteTab, error) {
	resp, err := o.client.Get(baseURL + "/screencast/tabs")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var tabs []remoteTab
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return nil, err
	}
	return tabs, nil
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

func isPortAvailable(port string) bool {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func (o *Orchestrator) ForceShutdown() {
	o.mu.RLock()
	instances := make([]*Instance, 0, len(o.instances))
	for _, inst := range o.instances {
		if instanceIsActive(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	for _, inst := range instances {
		if inst.PID > 0 {
			_ = syscall.Kill(-inst.PID, syscall.SIGKILL)
		}
		if inst.cancel != nil {
			inst.cancel()
		}
		o.markStopped(inst.ID)
	}
}

func isInstanceHealthyStatus(code int) bool {
	return code > 0 && code < http.StatusInternalServerError
}

func instanceBaseURLs(port string) []string {
	return []string{
		fmt.Sprintf("http://127.0.0.1:%s", port),
		fmt.Sprintf("http://[::1]:%s", port),
		fmt.Sprintf("http://localhost:%s", port),
	}
}

func tailLogLine(logs string) string {
	if logs == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		const max = 220
		if len(line) > max {
			return line[len(line)-max:]
		}
		return line
	}
	return ""
}

func mergeEnvWithOverrides(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, exists := overrides[key]; exists {
			continue
		}
		out = append(out, kv)
	}

	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+overrides[k])
	}
	return out
}
