package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/sanitize"
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
	MemoryMB  float64 `json:"memoryMB"`
	Renderers int     `json:"renderers"`
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

// maxCompactBodyBytes bounds the slice of a remote response body that reaches an
// operator-facing error message.
const maxCompactBodyBytes = 220

// compactBody is the marked form on purpose: every caller embeds the result in an
// fmt.Errorf, and an operator reading one needs to tell a short body from a cut one.
// The body is arbitrary remote bytes, so the cut goes through sanitize rather than a
// byte slice — a raw trimmed[:220] lands mid-rune on any non-ASCII body and puts U+FFFD
// in the message where the server's own words should be.
func compactBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "<empty>"
	}
	return sanitize.TruncateUTF8BytesWithEllipsis(trimmed, maxCompactBodyBytes)
}
