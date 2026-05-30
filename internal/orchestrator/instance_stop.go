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

	"github.com/pinchtab/pinchtab/internal/bridge"
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
