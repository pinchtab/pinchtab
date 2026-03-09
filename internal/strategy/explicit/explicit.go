// Package explicit implements the "explicit" strategy.
// All orchestrator endpoints are exposed directly — agents manage
// instances, profiles, and tabs explicitly. This reproduces the
// default dashboard behavior prior to the strategy framework.
package explicit

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
	"github.com/pinchtab/pinchtab/internal/strategy"
	"github.com/pinchtab/pinchtab/internal/web"
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

	// Shorthand endpoints proxy to first running instance.
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

	// /tabs returns empty list when no instances running.
	mux.HandleFunc("GET /tabs", s.handleTabs)
}

func (s *Strategy) proxyToFirst(w http.ResponseWriter, r *http.Request) {
	target := s.orch.FirstRunningURL()
	if target == "" {
		web.Error(w, 503, fmt.Errorf("no running instances — launch one from the Profiles tab"))
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
