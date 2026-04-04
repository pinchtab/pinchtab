package orchestrator

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
)

const (
	instanceHealthPollInterval       = 500 * time.Millisecond
	instanceStartupTimeout           = 45 * time.Second
	attachedBridgeHealthPollInterval = 60 * time.Second
	orchestratorActivitySource       = "orchestrator"
)

type healthProbePolicy int

const (
	healthProbePolicyLoopback healthProbePolicy = iota
	healthProbePolicyAttachAllowlist
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
	probePort, portErr := parsePortNumber(inst.Port)
	if portErr != nil {
		lastProbe = portErr.Error()
	}
	for time.Since(started) < instanceStartupTimeout {
		select {
		case waitErr = <-waitCh:
			exitedEarly = true
		default:
		}
		if exitedEarly {
			break
		}
		if portErr != nil {
			break
		}
		time.Sleep(instanceHealthPollInterval)

		// monitor only probes child bridge processes started by Launch.
		// Attached remote bridges are validated and probed during attach.
		for _, baseURL := range instanceBaseURLs(probePort) {
			targetBaseURL, err := o.validatedHealthProbeBaseURL(baseURL, "", healthProbePolicyLoopback)
			if err != nil {
				lastProbe = fmt.Sprintf("%s -> %s", baseURL, err.Error())
				continue
			}
			req, reqErr := http.NewRequest(http.MethodGet, healthProbeURL(targetBaseURL), nil)
			if reqErr != nil {
				lastProbe = fmt.Sprintf("%s -> %s", baseURL, reqErr.Error())
				continue
			}
			tagOrchestratorMonitoringRequest(req)
			o.applyInstanceAuth(req, inst)
			resp, err := o.client.Do(req)
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
	var baseURLs []string
	if inst.URL != "" {
		baseURLs = []string{strings.TrimRight(inst.URL, "/")}
	} else {
		probePort, err := parsePortNumber(inst.Port)
		if err != nil {
			return false, "", err.Error()
		}
		baseURLs = instanceBaseURLs(probePort)
	}

	policy := healthProbePolicyLoopback
	if inst.Attached && inst.AttachType == "bridge" {
		policy = healthProbePolicyAttachAllowlist
	}

	for _, baseURL := range baseURLs {
		targetBaseURL, err := o.validatedHealthProbeBaseURL(baseURL, "", policy)
		if err != nil {
			lastProbe = fmt.Sprintf("%s -> %s", baseURL, err.Error())
			continue
		}
		req, reqErr := http.NewRequest(http.MethodGet, healthProbeURL(targetBaseURL), nil)
		if reqErr != nil {
			lastProbe = fmt.Sprintf("%s -> %s", baseURL, reqErr.Error())
			continue
		}
		tagOrchestratorMonitoringRequest(req)
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
	tagOrchestratorMonitoringRequest(req)
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
	tagOrchestratorMonitoringRequest(req)
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

func tagOrchestratorMonitoringRequest(req *http.Request) {
	if req == nil {
		return
	}
	req.Header.Set(activity.HeaderPTSource, orchestratorActivitySource)
}

func isInstanceHealthyStatus(code int) bool {
	return code > 0 && code < http.StatusInternalServerError
}

func (o *Orchestrator) validatedHealthProbeBaseURL(rawURL, port string, policy healthProbePolicy) (*url.URL, error) {
	baseURL, err := o.parseHTTPInstanceURL(rawURL, port)
	if err != nil {
		return nil, err
	}

	host := baseURL.Hostname()
	switch policy {
	case healthProbePolicyAttachAllowlist:
		if o.runtimeCfg == nil {
			return nil, fmt.Errorf("blocked: attach not configured")
		}
		if !isAllowedAttachHost(host, o.runtimeCfg.AttachAllowHosts) {
			slog.Warn("health probe blocked: host not allowed", "url", rawURL, "host", host)
			return nil, fmt.Errorf("blocked: host not allowed")
		}
	default:
		if !isAllowedProbeHost(host) {
			slog.Warn("health probe blocked: non-loopback host", "url", rawURL, "host", host)
			return nil, fmt.Errorf("blocked: non-loopback host")
		}
	}

	// Reconstruct from validated components to break the CodeQL taint chain
	// from the user-controlled rawURL to the outgoing HTTP request.
	return sanitizedBaseURL(baseURL.Scheme, baseURL.Host), nil
}

// sanitizedBaseURL builds a fresh url.URL from individually validated scheme
// and host strings. This intentionally severs any data-flow link to the
// original user-supplied URL so static-analysis tools (CodeQL CWE-918) can
// verify the value is server-controlled.
func sanitizedBaseURL(scheme, host string) *url.URL {
	return &url.URL{Scheme: scheme, Host: host}
}

func healthProbeURL(baseURL *url.URL) string {
	return (&url.URL{
		Scheme: baseURL.Scheme,
		Host:   baseURL.Host,
		Path:   "/health",
	}).String()
}

// isAllowedProbeHost restricts health probes to loopback addresses to
// prevent SSRF when inst.URL is attacker-controlled.
func isAllowedProbeHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func instanceBaseURLs(port int) []string {
	return []string{
		fmt.Sprintf("http://127.0.0.1:%d", port),
		fmt.Sprintf("http://[::1]:%d", port),
		fmt.Sprintf("http://localhost:%d", port),
	}
}
