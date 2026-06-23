package orchestrator

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const attachedBridgeHealthPollInterval = 60 * time.Second

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
		baseURLs = instanceBaseURLs("", probePort)
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
