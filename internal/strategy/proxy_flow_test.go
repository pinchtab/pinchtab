package strategy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/strategy"
)

// proxyFlowRunner is a no-op orchestrator runner: no instance is ever started,
// so the orchestrator reports no running instances and the helpers under test
// take their empty-state branches.
type proxyFlowRunner struct{}

func (proxyFlowRunner) Run(context.Context, string, []string, []string, io.Writer, io.Writer) (orchestrator.Cmd, error) {
	return nil, nil
}

func (proxyFlowRunner) InspectPort(string) orchestrator.PortInspection {
	return orchestrator.PortInspection{Available: true}
}

// newEmptyOrch returns an orchestrator with a legacy (no-targets) runtime
// config and no running instances, so FirstRunningURLForRequest resolves to an
// empty target without error.
func newEmptyOrch(t *testing.T) *orchestrator.Orchestrator {
	t.Helper()
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), proxyFlowRunner{})
	orch.ApplyRuntimeConfig(&config.RuntimeConfig{})
	return orch
}

func TestProxyToFirstRunning_NoInstance_Returns503WithEmptyMessage(t *testing.T) {
	orch := newEmptyOrch(t)
	const emptyMsg = "no running browser instance for proxy flow test"

	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	rec := httptest.NewRecorder()

	strategy.ProxyToFirstRunning(orch, rec, req, emptyMsg)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v (raw %q)", err, rec.Body.String())
	}
	if got, _ := resp["error"].(string); got != emptyMsg {
		t.Fatalf("expected error %q, got %q (body %s)", emptyMsg, got, rec.Body.String())
	}
}

func TestProxyToFirstRunning_UnknownBrowser_ReturnsResolveError(t *testing.T) {
	// A requested-but-invalid browser makes FirstRunningURLForRequest return a
	// non-zero status + error, exercising the err branch (not the empty branch).
	orch := newEmptyOrch(t)

	req := httptest.NewRequest(http.MethodGet, "/snapshot?browser=definitely-not-a-browser", nil)
	rec := httptest.NewRecorder()

	strategy.ProxyToFirstRunning(orch, rec, req, "unused empty message")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "definitely-not-a-browser") {
		t.Fatalf("expected error to mention the unknown browser, got %s", rec.Body.String())
	}
}

func TestProxyTabsToFirst_NoInstance_ReturnsEmptyTabs(t *testing.T) {
	orch := newEmptyOrch(t)

	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	rec := httptest.NewRecorder()

	strategy.ProxyTabsToFirst(orch, rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Tabs []any `json:"tabs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v (raw %q)", err, rec.Body.String())
	}
	if resp.Tabs == nil {
		t.Fatalf("expected non-nil empty tabs array, got body %s", rec.Body.String())
	}
	if len(resp.Tabs) != 0 {
		t.Fatalf("expected empty tabs, got %d: %s", len(resp.Tabs), rec.Body.String())
	}
}

func TestProxyTabsToFirst_UnknownBrowser_ReturnsResolveError(t *testing.T) {
	orch := newEmptyOrch(t)

	req := httptest.NewRequest(http.MethodGet, "/tabs?browser=definitely-not-a-browser", nil)
	rec := httptest.NewRecorder()

	strategy.ProxyTabsToFirst(orch, rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
