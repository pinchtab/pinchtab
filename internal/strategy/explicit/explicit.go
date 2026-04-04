// Package explicit implements the "explicit" strategy.
// All orchestrator endpoints are exposed directly — agents manage
// instances, profiles, and tabs explicitly. This reproduces the
// default dashboard behavior prior to the strategy framework.
package explicit

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
	strategy.MustRegister("explicit", func() strategy.Strategy {
		return &Strategy{}
	})
}

// Strategy exposes all orchestrator endpoints directly.
type Strategy struct {
	orch *orchestrator.Orchestrator
}

func (s *Strategy) Name() string { return "explicit" }

// SetOrchestrator injects the orchestrator after construction.
func (s *Strategy) SetOrchestrator(o *orchestrator.Orchestrator) {
	s.orch = o
}

func (s *Strategy) Start(_ context.Context) error { return nil }
func (s *Strategy) Stop() error                   { return nil }

// RegisterRoutes adds all orchestrator routes plus shorthand proxy endpoints.
func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	s.orch.RegisterHandlers(mux)
	strategy.RegisterShorthandRoutes(mux, s.orch, s.proxyToFirst)
	mux.HandleFunc("GET /tabs", s.handleTabs)
}

func (s *Strategy) proxyToFirst(w http.ResponseWriter, r *http.Request) {
	target := s.orch.FirstRunningURL()
	if target == "" {
		httpx.Error(w, 503, fmt.Errorf("no running instances — launch one from the Profiles tab"))
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
