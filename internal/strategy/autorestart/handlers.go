package autorestart

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/strategy"
)

// RegisterRoutes adds shorthand endpoints that proxy to the managed instance.
func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	s.orch.RegisterHandlers(mux)
	strategy.RegisterShorthandRoutes(mux, s.orch, s.proxyToManaged)
	mux.HandleFunc("GET /tabs", s.handleTabs)
	mux.HandleFunc("GET "+s.config.StatusPath, s.handleStatus)
}

// proxyToManaged ensures the managed instance is running, then proxies.
func (s *Strategy) proxyToManaged(w http.ResponseWriter, r *http.Request) {
	target, err := s.ensureRunning()
	if err != nil {
		httpx.Error(w, 503, err)
		return
	}
	activity.EnrichRouteActivity(r)
	strategy.EnrichForTarget(r, s.orch, target)
	s.orch.ProxyToTarget(w, r, target+r.URL.Path)
}

// ensureRunning returns the URL of the managed instance if running.
func (s *Strategy) ensureRunning() (string, error) {
	if s.orch == nil {
		return "", fmt.Errorf("no orchestrator configured")
	}
	if target := s.orch.FirstRunningURL(); target != "" {
		return target, nil
	}
	return "", fmt.Errorf("instance not ready (may be restarting)")
}

func (s *Strategy) handleTabs(w http.ResponseWriter, r *http.Request) {
	target := s.orch.FirstRunningURL()
	if target == "" {
		httpx.JSON(w, 200, map[string]any{"tabs": []any{}})
		return
	}
	s.orch.ProxyToTarget(w, r, target+"/tabs")
}

func (s *Strategy) handleStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, s.State())
}
