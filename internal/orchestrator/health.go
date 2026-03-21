package orchestrator

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	instanceHealthPollInterval       = 500 * time.Millisecond
	instanceStartupTimeout           = 45 * time.Second
	attachedBridgeHealthPollInterval = 60 * time.Second
)

func (o *Orchestrator) monitor(inst *InstanceInternal) {
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

		healthy, resolvedURL, lastProbe = o.probeInstanceHealth(inst)
		if healthy {
			break
		}
	}

	o.mu.Lock()
	var eventType string
	switch inst.Status {
	case "stopping", "stopped":
	default:
		if healthy {
			inst.Status = "running"
			if resolvedURL != "" {
				inst.URL = resolvedURL
				inst.Instance.URL = resolvedURL
			}
			o.syncInstanceToManager(&inst.Instance)
			eventType = "instance.started"
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
			eventType = "instance.error"
			slog.Error("instance exited before ready", "id", inst.ID)
		} else {
			inst.Status = "error"
			inst.Error = fmt.Errorf("health check timeout after %s (%s)", instanceStartupTimeout, lastProbe).Error()
			if tail := tailLogLine(inst.logBuf.String()); tail != "" {
				inst.Error += " | " + tail
			}
			eventType = "instance.error"
			slog.Error("instance failed to start", "id", inst.ID)
		}
	}
	instCopy := inst.Instance
	o.mu.Unlock()
	if eventType != "" {
		o.emitEvent(eventType, &instCopy)
	}

	if !exitedEarly {
		<-waitCh
	}
	o.mu.Lock()
	wasStopped := false
	if inst.Status == "running" || inst.Status == "stopping" {
		inst.Status = "stopped"
		wasStopped = true
	}
	instCopy = inst.Instance
	o.mu.Unlock()
	if wasStopped {
		o.emitEvent("instance.stopped", &instCopy)
	}
	slog.Info("instance exited", "id", inst.ID)
}

func (o *Orchestrator) monitorAttachedBridge(inst *InstanceInternal) {
	ticker := time.NewTicker(attachedBridgeHealthPollInterval)
	defer ticker.Stop()

	for range ticker.C {
		if !o.checkAttachedBridgeHealth(inst) {
			return
		}
	}
}

func (o *Orchestrator) checkAttachedBridgeHealth(inst *InstanceInternal) bool {
	o.mu.RLock()
	current, ok := o.instances[inst.ID]
	shouldStop := !ok || current != inst || inst.Status != "running" || !inst.Attached || inst.AttachType != "bridge"
	o.mu.RUnlock()
	if shouldStop {
		return false
	}

	healthy, resolvedURL, lastProbe := o.probeInstanceHealth(inst)
	if healthy {
		if resolvedURL != "" && resolvedURL != inst.URL {
			o.mu.Lock()
			if current, ok := o.instances[inst.ID]; ok && current == inst {
				inst.URL = resolvedURL
				inst.Instance.URL = resolvedURL
				o.syncInstanceToManager(&inst.Instance)
			}
			o.mu.Unlock()
		}
		return true
	}

	slog.Warn("attached bridge unreachable, removing", "id", inst.ID, "probe", lastProbe)
	o.markStopped(inst.ID)
	return false
}

func (o *Orchestrator) probeInstanceHealth(inst *InstanceInternal) (bool, string, string) {
	lastProbe := "no response"
	for _, baseURL := range instanceBaseURLs(inst.URL, inst.Port) {
		baseParsed, parseErr := url.Parse(baseURL)
		if parseErr != nil {
			lastProbe = fmt.Sprintf("%s -> %s", baseURL, parseErr.Error())
			continue
		}
		target := &url.URL{Scheme: baseParsed.Scheme, Host: baseParsed.Host, Path: "/health"}
		req, reqErr := http.NewRequest(http.MethodGet, target.String(), nil)
		if reqErr != nil {
			lastProbe = fmt.Sprintf("%s -> %s", baseURL, reqErr.Error())
			continue
		}
		o.applyInstanceAuth(req, inst)
		resp, err := o.client.Do(req)
		if err != nil {
			lastProbe = fmt.Sprintf("%s -> %s", baseURL, err.Error())
			continue
		}
		_ = resp.Body.Close()
		lastProbe = fmt.Sprintf("%s -> HTTP %d", baseURL, resp.StatusCode)
		if isInstanceHealthyStatus(resp.StatusCode) {
			return true, baseURL, lastProbe
		}
	}
	return false, "", lastProbe
}

type remoteTab struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type remoteMetrics struct {
	Memory *memoryMetrics `json:"memory,omitempty"`
}

type memoryMetrics struct {
	JSHeapUsedMB  float64 `json:"jsHeapUsedMB"`
	JSHeapTotalMB float64 `json:"jsHeapTotalMB"`
	Documents     int64   `json:"documents"`
	Frames        int64   `json:"frames"`
	Nodes         int64   `json:"nodes"`
	Listeners     int64   `json:"listeners"`
}

func (o *Orchestrator) fetchTabs(inst *InstanceInternal) ([]remoteTab, error) {
	target, err := o.instancePathURL(inst, "/tabs", "")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	o.applyInstanceAuth(req, inst)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch tabs: status %d", resp.StatusCode)
	}

	var result struct {
		Tabs []remoteTab `json:"tabs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Tabs, nil
}

func (o *Orchestrator) fetchMetrics(inst *InstanceInternal) (*memoryMetrics, error) {
	target, err := o.instancePathURL(inst, "/metrics", "")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	o.applyInstanceAuth(req, inst)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, nil
	}

	var result remoteMetrics
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Memory, nil
}

func isInstanceHealthyStatus(code int) bool {
	return code > 0 && code < http.StatusInternalServerError
}

func instanceBaseURLs(rawURL, port string) []string {
	if rawURL != "" {
		return []string{strings.TrimRight(rawURL, "/")}
	}
	return []string{
		fmt.Sprintf("http://127.0.0.1:%s", port),
		fmt.Sprintf("http://[::1]:%s", port),
		fmt.Sprintf("http://localhost:%s", port),
	}
}
