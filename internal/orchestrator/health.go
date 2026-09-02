package orchestrator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	instanceHealthPollInterval = 500 * time.Millisecond
	instanceStartupTimeout     = 45 * time.Second
)

type startupProbe struct {
	healthy     bool
	exitedEarly bool
	waitErr     error
	resolvedURL string
	lastProbe   string
	waitCh      chan error
}

// startMonitor runs monitor for inst and registers it with o.monitors, so
// Shutdown can wait for it. Every monitor must start this way; a bare
// `go o.monitor(inst)` is a goroutine that outlives its orchestrator.
func (o *Orchestrator) startMonitor(inst *InstanceInternal) {
	o.monitors.Add(1)
	go func() {
		defer o.monitors.Done()
		o.monitor(inst)
	}()
}

// shuttingDown reports whether Shutdown has been called. Safe on an
// Orchestrator built without the constructor: a nil channel is never ready.
func (o *Orchestrator) shuttingDown() bool {
	select {
	case <-o.shutdownCh:
		return true
	default:
		return false
	}
}

// sleepOrShutdown waits for d, or returns early once Shutdown is called. It
// replaces a bare time.Sleep in the probe loop so a shutdown does not have to
// wait out a poll interval per instance.
func (o *Orchestrator) sleepOrShutdown(d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-o.shutdownCh:
	}
}

func (o *Orchestrator) monitor(inst *InstanceInternal) {
	p := o.probeStartupHealth(inst)
	o.applyStartupOutcome(inst, p)
	o.finalizeInstanceExit(inst, p)
}

func (o *Orchestrator) probeStartupHealth(inst *InstanceInternal) startupProbe {
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- inst.cmd.Wait()
	}()
	p := startupProbe{lastProbe: "no response", waitCh: waitCh}
	started := time.Now()
	probePort, portErr := parsePortNumber(inst.Port)
	if portErr != nil {
		p.lastProbe = portErr.Error()
	}
	for time.Since(started) < instanceStartupTimeout {
		select {
		case p.waitErr = <-waitCh:
			p.exitedEarly = true
		default:
		}
		if p.exitedEarly {
			break
		}
		if portErr != nil {
			break
		}
		// Stop probing a starting instance once the orchestrator is going down;
		// applyStartupOutcome below leaves a stopping/stopped instance alone, so
		// bailing here loses nothing.
		if o.shuttingDown() {
			p.lastProbe = "orchestrator shutting down"
			break
		}
		o.sleepOrShutdown(instanceHealthPollInterval)
		if o.shuttingDown() {
			p.lastProbe = "orchestrator shutting down"
			break
		}

		for _, baseURL := range instanceBaseURLs(configuredChildBind(o.runtimeCfg), probePort) {
			targetBaseURL, err := o.validatedHealthProbeBaseURL(baseURL, "", healthProbePolicyLoopback)
			if err != nil {
				p.lastProbe = fmt.Sprintf("%s -> %s", baseURL, err.Error())
				continue
			}
			ready, probe := o.probeChildInstanceReady(inst, targetBaseURL)
			p.lastProbe = fmt.Sprintf("%s -> %s", baseURL, probe)
			if ready {
				p.healthy = true
				p.resolvedURL = baseURL
				break
			}
		}
		if p.healthy {
			break
		}
	}
	return p
}

func (o *Orchestrator) applyStartupOutcome(inst *InstanceInternal, p startupProbe) {
	o.mu.Lock()
	var eventType string
	switch inst.Status {
	case "stopping", "stopped":
	default:
		if p.healthy {
			inst.Status = "running"
			if p.resolvedURL != "" {
				inst.URL = p.resolvedURL
				inst.Instance.URL = p.resolvedURL
			}
			eventType = "instance.started"
			slog.Info("instance ready", "id", inst.ID, "port", inst.Port)
		} else if p.exitedEarly {
			inst.Status = "error"
			if p.waitErr != nil {
				inst.Error = "process exited before health check: " + p.waitErr.Error()
			} else {
				inst.Error = "process exited before health check succeeded"
			}
			if tail := tailLogLine(inst.logBuf.String()); tail != "" {
				inst.Error += " | " + tail
			}
			inst.lastFailureReason = ClassifyLaunchFailure(errors.New(inst.Error))
			eventType = "instance.error"
			slog.Error("instance exited before ready", "id", inst.ID, "reason", string(inst.lastFailureReason))
		} else {
			inst.Status = "error"
			inst.Error = fmt.Errorf("health check timeout after %s (%s)", instanceStartupTimeout, p.lastProbe).Error()
			if tail := tailLogLine(inst.logBuf.String()); tail != "" {
				inst.Error += " | " + tail
			}
			inst.lastFailureReason = ClassifyLaunchFailure(errors.New(inst.Error))
			eventType = "instance.error"
			slog.Error("instance failed to start", "id", inst.ID, "reason", string(inst.lastFailureReason))
		}
	}
	// The repository holds a snapshot, so every status transition must re-sync;
	// error and timeout outcomes used to reach it only by sharing this struct.
	o.syncInstanceToManager(&inst.Instance)
	instCopy := inst.Instance
	o.mu.Unlock()
	if eventType != "" {
		o.emitEvent(eventType, &instCopy)
	}
}

func (o *Orchestrator) finalizeInstanceExit(inst *InstanceInternal, p startupProbe) {
	if !p.exitedEarly {
		// Interruptible: this waits for the child process to report exit, which
		// for a process that never exits is forever. Shutdown waits on the
		// monitors, so a bare receive here makes shutdown unbounded. Stop has
		// already signalled the process by the time shutdownCh closes; the state
		// transition below still runs.
		select {
		case <-p.waitCh:
		case <-o.shutdownCh:
		}
	}
	o.mu.Lock()
	wasStopped := false
	if inst.Status == "running" || inst.Status == "stopping" {
		inst.Status = "stopped"
		wasStopped = true
		o.syncInstanceToManager(&inst.Instance)
	}
	instCopy := inst.Instance
	o.mu.Unlock()
	if wasStopped {
		o.emitEvent("instance.stopped", &instCopy)
	}
	slog.Info("instance exited", "id", inst.ID)
}

func (o *Orchestrator) probeChildInstanceReady(inst *InstanceInternal, baseURL *url.URL) (bool, string) {
	req, reqErr := http.NewRequest(http.MethodGet, healthProbeURL(baseURL), nil)
	if reqErr != nil {
		return false, reqErr.Error()
	}
	tagOrchestratorMonitoringRequest(req)
	o.applyInstanceAuth(req, inst)
	resp, err := o.client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	_ = resp.Body.Close()
	if !isInstanceHealthyStatus(resp.StatusCode) {
		return false, fmt.Sprintf("health HTTP %d", resp.StatusCode)
	}

	tabID, err := o.warmInstanceTabLifecycle(inst, baseURL)
	if err != nil {
		if tabID != "" {
			slog.Debug("instance startup warmup left tab open", "id", inst.ID, "tabId", tabID, "err", err)
		}
		return false, fmt.Sprintf("warmup failed: %v", err)
	}
	return true, "ready"
}

func (o *Orchestrator) warmInstanceTabLifecycle(inst *InstanceInternal, baseURL *url.URL) (string, error) {
	payload := []byte(`{"action":"new","url":"about:blank"}`)
	tabURL := *baseURL
	tabURL.Path = "/tab"
	createReq, err := http.NewRequest(http.MethodPost, tabURL.String(), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	createReq.Header.Set("Content-Type", "application/json")
	tagOrchestratorMonitoringRequest(createReq)
	o.applyInstanceAuth(createReq, inst)

	createResp, err := o.client.Do(createReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = createResp.Body.Close() }()

	body, readErr := io.ReadAll(createResp.Body)
	if readErr != nil {
		return "", fmt.Errorf("read create-tab response: %w", readErr)
	}
	if createResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create tab HTTP %d: %s", createResp.StatusCode, compactBody(body))
	}

	var result struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode create-tab response: %w", err)
	}
	if strings.TrimSpace(result.TabID) == "" {
		return "", fmt.Errorf("create-tab response missing tabId")
	}

	closePayload, err := json.Marshal(map[string]string{"tabId": result.TabID})
	if err != nil {
		return result.TabID, err
	}
	closeURL := *baseURL
	closeURL.Path = "/close"
	closeReq, err := http.NewRequest(http.MethodPost, closeURL.String(), bytes.NewReader(closePayload))
	if err != nil {
		return result.TabID, err
	}
	closeReq.Header.Set("Content-Type", "application/json")
	tagOrchestratorMonitoringRequest(closeReq)
	o.applyInstanceAuth(closeReq, inst)

	closeResp, err := o.client.Do(closeReq)
	if err != nil {
		return result.TabID, fmt.Errorf("close warmup tab: %w", err)
	}
	defer func() { _ = closeResp.Body.Close() }()
	closeBody, readErr := io.ReadAll(closeResp.Body)
	if readErr != nil {
		return result.TabID, fmt.Errorf("read close-tab response: %w", readErr)
	}
	if closeResp.StatusCode != http.StatusOK {
		return result.TabID, fmt.Errorf("close warmup tab HTTP %d: %s", closeResp.StatusCode, compactBody(closeBody))
	}
	return result.TabID, nil
}
