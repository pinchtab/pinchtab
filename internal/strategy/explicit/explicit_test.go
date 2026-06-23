package explicit

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
)

// noopRunner never starts an instance, so the orchestrator reports no running
// instances and the explicit strategy's proxy helpers take their empty-state
// branches.
type noopRunner struct{}

func (noopRunner) Run(context.Context, string, []string, []string, io.Writer, io.Writer) (orchestrator.Cmd, error) {
	return nil, nil
}

func (noopRunner) InspectPort(string) orchestrator.PortInspection {
	return orchestrator.PortInspection{Available: true}
}

func newStrategy(t *testing.T) *Strategy {
	t.Helper()
	orch := orchestrator.NewOrchestratorWithRunner(t.TempDir(), noopRunner{})
	orch.ApplyRuntimeConfig(&config.RuntimeConfig{})
	return &Strategy{orch: orch}
}

func TestExplicitStrategy_Name(t *testing.T) {
	s := &Strategy{}
	if s.Name() != "explicit" {
		t.Fatalf("expected 'explicit', got %q", s.Name())
	}
}

func TestExplicitStrategy_ProxyToFirst_NoInstance_Returns503(t *testing.T) {
	s := newStrategy(t)

	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	rec := httptest.NewRecorder()

	s.proxyToFirst(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no running instances") {
		t.Fatalf("expected empty-state message in body, got %s", rec.Body.String())
	}
}

func TestExplicitStrategy_HandleTabs_NoInstance_ReturnsEmptyTabs(t *testing.T) {
	s := newStrategy(t)

	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	rec := httptest.NewRecorder()

	s.handleTabs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Tabs []any `json:"tabs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v (raw %q)", err, rec.Body.String())
	}
	if resp.Tabs == nil || len(resp.Tabs) != 0 {
		t.Fatalf("expected empty tabs array, got body %s", rec.Body.String())
	}
}
