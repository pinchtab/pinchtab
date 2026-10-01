package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/sanitize"
)

const orchestratorActivitySource = activity.SourceOrchestrator

type remoteTab struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type remoteMetrics struct {
	Memory *memoryMetrics `json:"memory,omitempty"`
}

type remoteHealth struct {
	Crashes *bridge.CrashSummary `json:"crashes,omitempty"`
}

type memoryMetrics struct {
	MemoryMB          float64             `json:"memoryMB"`
	Renderers         int                 `json:"renderers"`
	Page              *bridge.PageMetrics `json:"page,omitempty"`
	UnreadableTargets int                 `json:"unreadableTargets"`
}

func (o *Orchestrator) instanceGet(ctx context.Context, inst *InstanceInternal, path string) (*http.Response, error) {
	return o.instanceQueryRequest(ctx, http.MethodGet, inst, path, "", nil)
}

func (o *Orchestrator) instanceRequest(ctx context.Context, method string, inst *InstanceInternal, path string, header http.Header) (*http.Response, error) {
	return o.instanceQueryRequest(ctx, method, inst, path, "", header)
}

func (o *Orchestrator) instanceQueryRequest(ctx context.Context, method string, inst *InstanceInternal, path, rawQuery string, header http.Header) (*http.Response, error) {
	target, err := o.instancePathURL(inst, path, rawQuery)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return nil, err
	}
	maps.Copy(req.Header, header)
	tagOrchestratorMonitoringRequest(req)
	o.applyInstanceAuth(req, inst)
	return o.client.Do(req)
}

func (o *Orchestrator) fetchTabs(inst *InstanceInternal) ([]remoteTab, error) {
	return o.fetchTabList(inst, "")
}

func (o *Orchestrator) fetchOwnedTabs(inst *InstanceInternal) ([]remoteTab, error) {
	return o.fetchTabList(inst, handlers.IncludeTransientTabsQuery+"=1")
}

func (o *Orchestrator) fetchTabList(inst *InstanceInternal, rawQuery string) ([]remoteTab, error) {
	resp, err := o.instanceQueryRequest(context.Background(), http.MethodGet, inst, "/tabs", rawQuery, nil)
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
	resp, err := o.instanceGet(context.Background(), inst, "/metrics")
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

func (o *Orchestrator) fetchCrashes(ctx context.Context, inst *InstanceInternal) (*bridge.CrashSummary, error) {
	resp, err := o.instanceGet(ctx, inst, "/health")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var result remoteHealth
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Crashes, nil
}

func (o *Orchestrator) probeTabs(ctx context.Context, inst *InstanceInternal) error {
	resp, err := o.instanceGet(ctx, inst, "/tabs")
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func classifyResponsiveness(healthErr, tabsErr error) string {
	switch {
	case healthErr != nil:
		return bridge.ResponsivenessUnknown
	case tabsErr == nil:
		return bridge.ResponsivenessResponsive
	case errors.Is(tabsErr, context.DeadlineExceeded):
		return bridge.ResponsivenessUnresponsive
	default:
		return bridge.ResponsivenessUnknown
	}
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
