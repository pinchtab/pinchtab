package autorestart

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/readiness"
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

func (s *Strategy) proxyToManaged(w http.ResponseWriter, r *http.Request) {
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

// ensureRunning returns the URL of the managed instance, waiting briefly for
// the initial launch / a restart to finish before giving up.
func (s *Strategy) ensureRunning(r *http.Request) (string, int, error) {
	if s.orch == nil {
		return "", 503, fmt.Errorf("no orchestrator configured")
	}
	var lastStatus int
	target, err := readiness.WaitUntil(context.Background(), instanceReadyWait, 200*time.Millisecond,
		func() (string, bool, error) {
			t, status, err := s.orch.FirstRunningURLForRequest(r)
			if err != nil {
				lastStatus = status
				return "", false, err
			}
			return t, t != "", nil
		})
	if err != nil {
		if errors.Is(err, readiness.ErrNotReady) {
			return "", 503, fmt.Errorf("instance not ready after %s (may be restarting)", instanceReadyWait)
		}
		return "", lastStatus, err
	}
	return target, 0, nil
}

func (s *Strategy) handleTabs(w http.ResponseWriter, r *http.Request) {
	strategy.ProxyTabsToFirst(s.orch, w, r)
}

func (s *Strategy) handleStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, s.State())
}
