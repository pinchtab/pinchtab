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

	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
	"github.com/pinchtab/pinchtab/internal/strategy"
	"github.com/pinchtab/pinchtab/internal/web"
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
	// Register orchestrator's instance/profile/tab-specific routes.
	s.orch.RegisterHandlers(mux)

	// Shorthand endpoints — all proxy to first running instance.
	shorthandRoutes := []string{
		"GET /snapshot", "GET /screenshot", "GET /text", "GET /pdf", "POST /pdf",
		"POST /navigate", "POST /action", "POST /actions",
		"POST /tab", "POST /tab/lock", "POST /tab/unlock",
		"GET /cookies", "POST /cookies",
		"GET /stealth/status", "POST /fingerprint/rotate",
		"POST /find",
	}
	for _, route := range shorthandRoutes {
		mux.HandleFunc(route, s.proxyToFirst)
	}
	strategy.RegisterCapabilityRoute(mux, "POST /evaluate", s.orch.AllowsEvaluate(), "evaluate", "security.allowEvaluate", "evaluate_disabled", s.proxyToFirst)
	strategy.RegisterCapabilityRoute(mux, "GET /download", s.orch.AllowsDownload(), "download", "security.allowDownload", "download_disabled", s.proxyToFirst)
	strategy.RegisterCapabilityRoute(mux, "POST /upload", s.orch.AllowsUpload(), "upload", "security.allowUpload", "upload_disabled", s.proxyToFirst)
	strategy.RegisterCapabilityRoute(mux, "GET /screencast", s.orch.AllowsScreencast(), "screencast", "security.allowScreencast", "screencast_disabled", s.proxyToFirst)
	strategy.RegisterCapabilityRoute(mux, "GET /screencast/tabs", s.orch.AllowsScreencast(), "screencast", "security.allowScreencast", "screencast_disabled", s.proxyToFirst)
	strategy.RegisterCapabilityRoute(mux, "POST /macro", s.orch.AllowsMacro(), "macro", "security.allowMacro", "macro_disabled", s.proxyToFirst)

	mux.HandleFunc("GET /tabs", s.handleTabs)
}

// proxyToFirst ensures an instance is running, then proxies the request to it.
func (s *Strategy) proxyToFirst(w http.ResponseWriter, r *http.Request) {
	target, err := s.ensureRunning()
	if err != nil {
		web.Error(w, 503, err)
		return
	}
	proxy.HTTP(w, r, target+r.URL.Path)
}

func (s *Strategy) handleTabs(w http.ResponseWriter, r *http.Request) {
	target := s.orch.FirstRunningURL()
	if target == "" {
		web.JSON(w, 200, map[string]any{"tabs": []any{}})
		return
	}
	proxy.HTTP(w, r, target+"/tabs")
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
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		url := fmt.Sprintf("http://localhost:%s", launched.Port)
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
