// Package explicit implements the "explicit" strategy and the parameterised
// direct-exposure strategy it shares with "no-instance": orchestrator endpoints
// are exposed directly and shorthand routes proxy to the first running instance.
package explicit

import (
	"context"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/strategy"
)

func init() {
	strategy.MustRegister("explicit", func() strategy.Strategy {
		return New()
	})
}

// Strategy exposes orchestrator endpoints directly. Name, the orchestrator
// route set and the empty-state message are what distinguish its variants.
type Strategy struct {
	name              string
	registerHandlers  func(*orchestrator.Orchestrator, *http.ServeMux)
	noInstanceMessage string
	orch              *orchestrator.Orchestrator
}

// New returns the explicit strategy: every orchestrator endpoint, including launch.
func New() *Strategy {
	return NewDirect("explicit", (*orchestrator.Orchestrator).RegisterHandlers,
		"no running instances — launch one from the Profiles tab")
}

// NewDirect returns a direct-exposure strategy registering routes with
// registerHandlers and answering shorthand routes with noInstanceMessage when
// nothing is running.
func NewDirect(name string, registerHandlers func(*orchestrator.Orchestrator, *http.ServeMux), noInstanceMessage string) *Strategy {
	return &Strategy{name: name, registerHandlers: registerHandlers, noInstanceMessage: noInstanceMessage}
}

func (s *Strategy) Name() string { return s.name }

// SetOrchestrator injects the orchestrator after construction.
func (s *Strategy) SetOrchestrator(o *orchestrator.Orchestrator) {
	s.orch = o
}

func (s *Strategy) Start(_ context.Context) error { return nil }
func (s *Strategy) Stop() error                   { return nil }

func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	s.registerHandlers(s.orch, mux)
	strategy.RegisterShorthandRoutes(mux, s.orch, s.proxyToFirst)
	mux.HandleFunc("GET /tabs", s.handleTabs)
}

func (s *Strategy) proxyToFirst(w http.ResponseWriter, r *http.Request) {
	strategy.ProxyToFirstRunning(s.orch, w, r, s.noInstanceMessage)
}

func (s *Strategy) handleTabs(w http.ResponseWriter, r *http.Request) {
	strategy.ProxyTabsToFirst(s.orch, w, r)
}
