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
	"log/slog"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
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

// proxyToFirst ensures an instance is running, then proxies the request to it.
func (s *Strategy) proxyToFirst(w http.ResponseWriter, r *http.Request) {
	target, err := s.ensureRunning()
	if err != nil {
		httpx.Error(w, 503, err)
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

// ensureRunning returns the URL of a running instance, auto-launching one if needed.
func (s *Strategy) ensureRunning() (string, error) {
	if s.orch == nil {
		return "", fmt.Errorf("no running instances")
	}
	if target := s.orch.FirstRunningURL(); target != "" {
		return target, nil
	}

	slog.Info("simple strategy: no running instances, auto-launching")
	mgr := s.orch.InstanceManager()
	if mgr == nil {
		return "", fmt.Errorf("no running instances")
	}

	launched, err := mgr.Launch("default", "", true)
	if err != nil {
		return "", fmt.Errorf("auto-launch failed: %w", err)
	}

	// Wait for instance to become ready.
	deadline := time.Now().Add(30 * time.Second)
	if launched.URL == "" {
		return "", fmt.Errorf("auto-launched instance %q has no URL", launched.ID)
	}
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		url := launched.URL
		resp, healthErr := http.Get(url + "/health")
		if healthErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return url, nil
			}
		}
	}

	return "", fmt.Errorf("instance launched but did not become ready in time")
}
