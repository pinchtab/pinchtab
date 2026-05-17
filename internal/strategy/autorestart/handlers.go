package autorestart

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/strategy"
)

// instanceReadyWait is the longest a proxied request will block waiting for
// the managed instance to come up. Covers the first request after server
// start (when launchInitial is still spinning up Chrome) without hanging
// indefinitely on a genuinely broken instance.
const instanceReadyWait = 10 * time.Second

// RegisterRoutes adds shorthand endpoints that proxy to the managed instance.
func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	s.orch.RegisterHandlers(mux)
	strategy.RegisterShorthandRoutes(mux, s.orch, s.proxyToManaged)
	mux.HandleFunc("GET /tabs", s.handleTabs)
	mux.HandleFunc("GET "+s.config.StatusPath, s.handleStatus)
}

// proxyToManaged ensures the managed instance is running, then proxies.
func (s *Strategy) proxyToManaged(w http.ResponseWriter, r *http.Request) {
	target, status, err := s.ensureRunning(r)
	if err != nil {
		if status == 0 {
			status = 503
		}
		httpx.Error(w, status, err)
		return
	}
	activity.EnrichRouteActivity(r)
	strategy.EnrichForTarget(r, s.orch, target)
	s.orch.ProxyToTarget(w, r, target+r.URL.Path)
}

// ensureRunning returns the URL of the managed instance, waiting briefly for
// the initial launch / a restart to finish before giving up.
func (s *Strategy) ensureRunning(r *http.Request) (string, int, error) {
	if s.orch == nil {
		return "", 503, fmt.Errorf("no orchestrator configured")
	}
	if target, status, err := s.orch.FirstRunningURLForRequest(r); err != nil {
		return "", status, err
	} else if target != "" {
		return target, 0, nil
	}
	deadline := time.Now().Add(instanceReadyWait)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if target, status, err := s.orch.FirstRunningURLForRequest(r); err != nil {
			return "", status, err
		} else if target != "" {
			return target, 0, nil
		}
	}
	return "", 503, fmt.Errorf("instance not ready after %s (may be restarting)", instanceReadyWait)
}

func (s *Strategy) handleTabs(w http.ResponseWriter, r *http.Request) {
	target, status, err := s.orch.FirstRunningURLForRequest(r)
	if err != nil {
		httpx.Error(w, status, err)
		return
	}
	if target == "" {
		httpx.JSON(w, 200, map[string]any{"tabs": []any{}})
		return
	}
	s.orch.ProxyToTarget(w, r, target+"/tabs")
}

func (s *Strategy) handleStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, s.State())
}
