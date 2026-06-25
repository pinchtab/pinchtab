package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
)

const orchestratorActivitySource = "orchestrator"

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

func compactBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "<empty>"
	}
	const max = 220
	if len(trimmed) > max {
		return trimmed[:max]
	}
	return trimmed
}
