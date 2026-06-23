// Package simple implements the "simple" allocation strategy.
//
// Simple makes orchestrator mode feel like bridge mode.
// All shorthand endpoints proxy to the first running instance.
// If no instances are running, one is auto-launched on first request.
//
// Tab lifecycle is handled by the bridge — the strategy is just
// a thin proxy with auto-launch.
package simple

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/strategy"
)

func init() {
	strategy.MustRegister("simple", func() strategy.Strategy {
		return &Strategy{}
	})
}

// Strategy proxies all shorthand endpoints to the first running instance,
// auto-launching one if needed.
type Strategy struct {
	orch *orchestrator.Orchestrator
}

func (s *Strategy) Name() string { return "simple" }

// SetOrchestrator injects the orchestrator after construction.
func (s *Strategy) SetOrchestrator(o *orchestrator.Orchestrator) {
	s.orch = o
}

func (s *Strategy) Start(_ context.Context) error { return nil }
func (s *Strategy) Stop() error                   { return nil }

// RegisterRoutes adds shorthand endpoints that proxy to the first running instance.
func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	s.orch.RegisterHandlers(mux)
	strategy.RegisterShorthandRoutes(mux, s.orch, s.proxyToFirst)
	mux.HandleFunc("GET /tabs", s.handleTabs)
}

func (s *Strategy) proxyToFirst(w http.ResponseWriter, r *http.Request) {
	target, status, err := s.ensureRunning(r)
	if err != nil {
		if status == 0 {
			status = 503
		}
		httpx.Error(w, status, err)
		return
	}
	strategy.EnrichAndProxy(s.orch, w, r, target)
}

func (s *Strategy) handleTabs(w http.ResponseWriter, r *http.Request) {
	strategy.ProxyTabsToFirst(s.orch, w, r)
}

// ensureRunning returns the URL of a running instance, auto-launching one if needed.
func (s *Strategy) ensureRunning(r *http.Request) (string, int, error) {
	if s.orch == nil {
		return "", 503, fmt.Errorf("no running instances")
	}
	return s.orch.RouteForRequest(r)
}
