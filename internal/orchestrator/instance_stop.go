package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers/providerhooks"
	"github.com/pinchtab/pinchtab/internal/config"
)

var (
	shutdownRequestTimeout      = 4 * time.Second
	registeredBridgeStopTimeout = 5 * time.Second
	gracefulProcessStopTimeout  = 5 * time.Second
	termProcessStopTimeout      = 3 * time.Second
	killProcessStopTimeout      = 2 * time.Second
)

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
		if inst.AttachType == "bridge" || inst.AttachType == "cdp-bridge" {
			if err := o.stopRegisteredBridge(inst); err != nil {
				o.setStopError(id, err.Error())
				return err
			}
		}
		o.markStopped(id)
		return nil
	}

	pid := inst.cmd.PID()

	reqCtx, cancel := context.WithTimeout(context.Background(), shutdownRequestTimeout)
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
		if waitForProcessExit(pid, gracefulProcessStopTimeout) {
			o.markStopped(id)
			return nil
		}

		if err := killProcessGroup(pid, sigTERM); err != nil {
			slog.Warn("failed to send SIGTERM to instance", "id", id, "pid", pid, "err", err)
		}
		if waitForProcessExit(pid, termProcessStopTimeout) {
			o.markStopped(id)
			return nil
		}

		if err := killProcessGroup(pid, sigKILL); err != nil {
			slog.Warn("failed to send SIGKILL to instance", "id", id, "pid", pid, "err", err)
		}
	}

	inst.cmd.Cancel()

	if pid > 0 {
		if waitForProcessExit(pid, killProcessStopTimeout) {
			o.markStopped(id)
			return nil
		}
		o.setStopError(id, fmt.Sprintf("failed to stop process %d; still running", pid))
		return fmt.Errorf("failed to stop instance %q gracefully", id)
	}

	o.markStopped(id)
	return nil
}

func (o *Orchestrator) stopRegisteredBridge(inst *InstanceInternal) error {
	shutdownURL, err := o.instancePathURL(inst, "/shutdown", "")
	if err != nil {
		return fmt.Errorf("cannot stop registered bridge %q: %w", inst.ID, err)
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), shutdownRequestTimeout)
	request, requestErr := http.NewRequestWithContext(requestCtx, http.MethodPost, shutdownURL.String(), nil)
	if requestErr == nil {
		o.applyInstanceAuth(request, inst)
		response, doErr := o.client.Do(request)
		if response != nil {
			_ = response.Body.Close()
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				requestErr = fmt.Errorf("shutdown returned HTTP %d", response.StatusCode)
			}
		}
		if doErr != nil {
			requestErr = doErr
		}
	}
	cancel()

	if o.waitForBridgeEndpointExit(inst, registeredBridgeStopTimeout) {
		return nil
	}
	if requestErr != nil {
		return fmt.Errorf("registered bridge %q did not stop: %w", inst.ID, requestErr)
	}
	return fmt.Errorf("registered bridge %q acknowledged shutdown but its endpoint is still reachable", inst.ID)
}

func (o *Orchestrator) waitForBridgeEndpointExit(inst *InstanceInternal, timeout time.Duration) bool {
	healthURL, err := o.instancePathURL(inst, "/health", "")
	if err != nil {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		probeCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		request, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, healthURL.String(), nil)
		o.applyInstanceAuth(request, inst)
		response, probeErr := o.client.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		cancel()
		if probeErr != nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
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
	browser := inst.browser
	attached := inst.Attached
	delete(o.instances, id)
	o.mu.Unlock()

	o.releaseInstancePorts(id, inst)
	o.removeInstanceFromManager(id)

	slog.Info("instance stopped and removed", "id", id, "profile", profileName)
	o.cleanupStoppedProfile(profileName, browser)
	if attached {
		// Attach children write per-instance state under baseDir/attach/<id>;
		// without this the dirs accumulate across attach/detach cycles.
		stateDir := filepath.Join(o.baseDir, "attach", id)
		if err := os.RemoveAll(stateDir); err != nil {
			slog.Warn("failed to remove attach state dir", "id", id, "dir", stateDir, "err", err)
		}
	}
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
		slog.Debug("released browser debug port", "id", id, "port", inst.cdpPort)
	}
}

func (o *Orchestrator) removeInstanceFromManager(id string) {
	if o != nil && o.instanceMgr != nil {
		o.instanceMgr.Locator.InvalidateInstance(id)
		o.instanceMgr.Repo.Remove(id)
	}
}

func (o *Orchestrator) cleanupStoppedProfile(profileName, browser string) {
	profilePath := filepath.Join(o.baseDir, profileName)
	if browser == "" && o.runtimeCfg != nil {
		browser = config.NormalizeBrowser(o.runtimeCfg.DefaultBrowser)
	}
	if browser == "" {
		// Every provider's cleanup hook is the same chrome-process sweep;
		// skipping orphan cleanup entirely (no hook registered under "") is
		// strictly worse than a chrome-targeted one.
		browser = config.BrowserChrome
	}
	providerhooks.CleanupProfile(browser, profilePath)

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
