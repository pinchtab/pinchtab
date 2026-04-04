// Package noinstance implements the "no-instance" strategy.
// No local browser instances are launched. The server acts as a hub
// that only accepts remote bridges via /instances/attach-bridge.
package noinstance

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/strategy"
)

func init() {
	strategy.MustRegister("no-instance", func() strategy.Strategy {
		return &Strategy{}
	})
}

// Strategy accepts only remote bridges — no local Chrome processes.
type Strategy struct {
	orch *orchestrator.Orchestrator
}

func (s *Strategy) Name() string { return "no-instance" }

func (s *Strategy) SetOrchestrator(o *orchestrator.Orchestrator) {
	s.orch = o
}

func (s *Strategy) Start(_ context.Context) error { return nil }
func (s *Strategy) Stop() error                   { return nil }

func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	s.orch.RegisterHandlersNoLaunch(mux)
	strategy.RegisterShorthandRoutes(mux, s.orch, s.proxyToFirst)
	mux.HandleFunc("GET /tabs", s.handleTabs)
}

func (s *Strategy) proxyToFirst(w http.ResponseWriter, r *http.Request) {
	target := s.orch.FirstRunningURL()
	if target == "" {
		httpx.Error(w, 503, fmt.Errorf("no remote instances connected — attach a bridge first"))
		return
	}
	activity.EnrichRouteActivity(r)
	strategy.EnrichForTarget(r, s.orch, target)
	s.orch.ProxyToTarget(w, r, target+r.URL.Path)
}

func (s *Strategy) handleTabs(w http.ResponseWriter, r *http.Request) {
	target := s.orch.FirstRunningURL()
	if target == "" {
		httpx.JSON(w, 200, map[string]any{"tabs": []any{}})
		return
	}
	s.orch.ProxyToTarget(w, r, target+"/tabs")
}
