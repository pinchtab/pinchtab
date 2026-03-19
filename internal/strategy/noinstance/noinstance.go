// Package noinstance implements the "no-instance" strategy.
// No local browser instances are launched. The server acts as a hub
// that only accepts remote bridges via /instances/attach-bridge.
package noinstance

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
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
	// Register orchestrator handlers without local launch endpoints
	s.orch.RegisterHandlersNoLaunch(mux)

	// Shorthand endpoints proxy to first running (remote) instance
	shorthandRoutes := []string{
		"GET /snapshot", "GET /screenshot", "GET /text", "GET /pdf", "POST /pdf",
		"POST /navigate", "POST /back", "POST /forward", "POST /reload",
		"POST /action", "POST /actions",
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

func (s *Strategy) proxyToFirst(w http.ResponseWriter, r *http.Request) {
	target := s.orch.FirstRunningURL()
	if target == "" {
		httpx.Error(w, 503, fmt.Errorf("no remote instances connected — attach a bridge first"))
		return
	}
	strategy.EnrichForTarget(r, s.orch, target)
	proxy.HTTP(w, r, target+r.URL.Path)
}

func (s *Strategy) handleTabs(w http.ResponseWriter, r *http.Request) {
	target := s.orch.FirstRunningURL()
	if target == "" {
		httpx.JSON(w, 200, map[string]any{"tabs": []any{}})
		return
	}
	proxy.HTTP(w, r, target+"/tabs")
}
