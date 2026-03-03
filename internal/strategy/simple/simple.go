// Package simple implements the Simple allocation strategy.
//
// Simple strategy makes Orchestrator mode feel like Bridge mode.
// Agents get shorthand endpoints (/navigate, /snapshot, /action, etc.)
// without needing to know about instances or tab IDs.
//
// The strategy:
//  1. Allocates an instance using the configured AllocationPolicy
//  2. Creates tabs and navigates automatically
//  3. Remembers the "current tab" for subsequent operations
//  4. Proxies operations to the owning bridge
package simple

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/pinchtab/pinchtab/internal/instance"
	"github.com/pinchtab/pinchtab/internal/primitive"
	"github.com/pinchtab/pinchtab/internal/web"
)

// Strategy exposes shorthand endpoints that abstract away instance management.
// Agents interact as if talking to a single browser.
type Strategy struct {
	mgr    *instance.Manager
	bridge *instance.BridgeClient

	mu          sync.RWMutex
	currentTab  string // current tab ID
	currentInst string // instance ID owning the current tab
	currentPort string // port of the current instance
}

// New creates a Simple strategy backed by the given InstanceManager.
func New(mgr *instance.Manager) *Strategy {
	return &Strategy{
		mgr:    mgr,
		bridge: instance.NewBridgeClient(),
	}
}

func (s *Strategy) Name() string { return "simple" }

// Init receives primitives (unused — Simple strategy uses instance.Manager directly).
func (s *Strategy) Init(_ *primitive.Primitives) error { return nil }

func (s *Strategy) Start(_ context.Context) error { return nil }
func (s *Strategy) Stop() error                   { return nil }

// RegisterRoutes adds shorthand endpoints to the mux.
func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	// Shorthand endpoints (no tab ID required).
	mux.HandleFunc("POST /navigate", s.handleNavigate)
	mux.HandleFunc("GET /navigate", s.handleNavigate)
	mux.HandleFunc("GET /snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /screenshot", s.handleScreenshot)
	mux.HandleFunc("GET /text", s.handleText)
	mux.HandleFunc("POST /action", s.handleAction)
	mux.HandleFunc("POST /actions", s.handleActions)
	mux.HandleFunc("POST /evaluate", s.handleEvaluate)
	mux.HandleFunc("GET /cookies", s.handleGetCookies)
	mux.HandleFunc("POST /cookies", s.handleSetCookies)

	// Tab-specific endpoints (for agents that track tab IDs).
	mux.HandleFunc("POST /tabs/{id}/navigate", s.handleTabNavigate)
	mux.HandleFunc("GET /tabs/{id}/snapshot", s.handleTabSnapshot)
	mux.HandleFunc("GET /tabs/{id}/screenshot", s.handleTabScreenshot)
	mux.HandleFunc("GET /tabs/{id}/text", s.handleTabText)
	mux.HandleFunc("POST /tabs/{id}/action", s.handleTabAction)
	mux.HandleFunc("POST /tabs/{id}/actions", s.handleTabActions)
	mux.HandleFunc("POST /tabs/{id}/evaluate", s.handleTabEvaluate)
	mux.HandleFunc("GET /tabs/{id}/pdf", s.handleTabPDF)
	mux.HandleFunc("POST /tabs/{id}/pdf", s.handleTabPDF)
	mux.HandleFunc("GET /tabs/{id}/cookies", s.handleTabGetCookies)
	mux.HandleFunc("POST /tabs/{id}/cookies", s.handleTabSetCookies)
	mux.HandleFunc("POST /tabs/{id}/close", s.handleTabClose)

	// Tab management.
	mux.HandleFunc("POST /tab", s.handleTab)
	mux.HandleFunc("GET /tabs", s.handleListTabs)

	// Instance info (read-only, delegates to manager).
	mux.HandleFunc("GET /instances", s.handleListInstances)
}

// --- Shorthand handlers (use current tab) ---

func (s *Strategy) handleNavigate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}

	if r.Method == http.MethodGet {
		req.URL = r.URL.Query().Get("url")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			web.Error(w, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
			return
		}
	}

	if req.URL == "" {
		web.Error(w, http.StatusBadRequest, fmt.Errorf("url is required"))
		return
	}

	// Ensure we have an instance allocated.
	port, err := s.ensureInstance()
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, fmt.Errorf("no instance available: %w", err))
		return
	}

	// Create a new tab and navigate.
	tabID, err := s.bridge.CreateTab(r.Context(), port, req.URL)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, fmt.Errorf("create tab: %w", err))
		return
	}

	s.setCurrentTab(tabID)

	web.JSON(w, http.StatusOK, map[string]string{
		"tabId": tabID,
		"url":   req.URL,
	})
}

func (s *Strategy) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	tabID, port, err := s.currentOrFirst(r.Context())
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/snapshot")
}

func (s *Strategy) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	tabID, port, err := s.currentOrFirst(r.Context())
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/screenshot")
}

func (s *Strategy) handleText(w http.ResponseWriter, r *http.Request) {
	tabID, port, err := s.currentOrFirst(r.Context())
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/text")
}

func (s *Strategy) handleAction(w http.ResponseWriter, r *http.Request) {
	tabID, port, err := s.currentOrFirst(r.Context())
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/action")
}

func (s *Strategy) handleActions(w http.ResponseWriter, r *http.Request) {
	tabID, port, err := s.currentOrFirst(r.Context())
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/actions")
}

func (s *Strategy) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	tabID, port, err := s.currentOrFirst(r.Context())
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/evaluate")
}

func (s *Strategy) handleGetCookies(w http.ResponseWriter, r *http.Request) {
	tabID, port, err := s.currentOrFirst(r.Context())
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/cookies")
}

func (s *Strategy) handleSetCookies(w http.ResponseWriter, r *http.Request) {
	tabID, port, err := s.currentOrFirst(r.Context())
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/cookies")
}

// --- Tab-specific handlers (explicit tab ID in path) ---

func (s *Strategy) handleTabNavigate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/navigate")
}

func (s *Strategy) handleTabSnapshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/snapshot")
}

func (s *Strategy) handleTabScreenshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/screenshot")
}

func (s *Strategy) handleTabText(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/text")
}

func (s *Strategy) handleTabAction(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/action")
}

func (s *Strategy) handleTabActions(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/actions")
}

func (s *Strategy) handleTabEvaluate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/evaluate")
}

func (s *Strategy) handleTabPDF(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/pdf")
}

func (s *Strategy) handleTabGetCookies(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/cookies")
}

func (s *Strategy) handleTabSetCookies(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	s.bridge.ProxyToTab(w, r, port, tabID, "/cookies")
}

func (s *Strategy) handleTabClose(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	port, err := s.portForTab(tabID)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}

	if err := s.bridge.CloseTab(r.Context(), port, tabID); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}

	s.mgr.InvalidateTab(tabID)

	// If this was the current tab, clear it.
	s.mu.Lock()
	if s.currentTab == tabID {
		s.currentTab = ""
		s.currentInst = ""
		s.currentPort = ""
	}
	s.mu.Unlock()

	web.JSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

// --- Tab management ---

func (s *Strategy) handleTab(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		TabID  string `json:"tabId"`
		URL    string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
		return
	}

	switch req.Action {
	case "new":
		port, err := s.ensureInstance()
		if err != nil {
			web.Error(w, http.StatusServiceUnavailable, err)
			return
		}
		url := req.URL
		if url == "" {
			url = "about:blank"
		}
		tabID, err := s.bridge.CreateTab(r.Context(), port, url)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err)
			return
		}
		s.setCurrentTab(tabID)
		web.JSON(w, http.StatusOK, map[string]string{"tabId": tabID, "url": url})

	case "close":
		if req.TabID == "" {
			web.Error(w, http.StatusBadRequest, fmt.Errorf("tabId required for close"))
			return
		}
		port, err := s.portForTab(req.TabID)
		if err != nil {
			web.Error(w, http.StatusNotFound, err)
			return
		}
		if err := s.bridge.CloseTab(r.Context(), port, req.TabID); err != nil {
			web.Error(w, http.StatusInternalServerError, err)
			return
		}
		s.mgr.InvalidateTab(req.TabID)
		web.JSON(w, http.StatusOK, map[string]string{"status": "closed"})

	default:
		web.Error(w, http.StatusBadRequest, fmt.Errorf("unknown action: %s", req.Action))
	}
}

func (s *Strategy) handleListTabs(w http.ResponseWriter, r *http.Request) {
	running := s.mgr.Running()
	var allTabs []map[string]string
	for _, inst := range running {
		tabs, err := s.bridge.FetchTabs("http://localhost:" + inst.Port)
		if err != nil {
			continue
		}
		for _, tab := range tabs {
			allTabs = append(allTabs, map[string]string{
				"id":         tab.ID,
				"instanceId": inst.ID,
				"url":        tab.URL,
				"title":      tab.Title,
			})
		}
	}
	if allTabs == nil {
		allTabs = []map[string]string{}
	}
	web.JSON(w, http.StatusOK, allTabs)
}

func (s *Strategy) handleListInstances(w http.ResponseWriter, _ *http.Request) {
	web.JSON(w, http.StatusOK, s.mgr.List())
}

// --- Internal helpers ---

// ensureInstance returns the port of an allocated instance.
// Uses the current instance if available, otherwise allocates via policy.
func (s *Strategy) ensureInstance() (string, error) {
	s.mu.RLock()
	port := s.currentPort
	instID := s.currentInst
	s.mu.RUnlock()

	if port != "" {
		// Verify instance is still running.
		if inst, ok := s.mgr.Get(instID); ok && inst.Status == "running" {
			return port, nil
		}
	}

	// Allocate a new instance.
	inst, err := s.mgr.Allocate()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.currentInst = inst.ID
	s.currentPort = inst.Port
	s.mu.Unlock()

	return inst.Port, nil
}

// currentOrFirst returns the current tab and its port.
// If no current tab, discovers the first tab from running instances.
func (s *Strategy) currentOrFirst(ctx context.Context) (tabID, port string, err error) {
	s.mu.RLock()
	tabID = s.currentTab
	port = s.currentPort
	s.mu.RUnlock()

	if tabID != "" && port != "" {
		return tabID, port, nil
	}

	// No current tab — try to find the first available tab.
	running := s.mgr.Running()
	for _, inst := range running {
		tabs, fetchErr := s.bridge.FetchTabs("http://localhost:" + inst.Port)
		if fetchErr != nil {
			continue
		}
		if len(tabs) > 0 {
			s.mu.Lock()
			s.currentTab = tabs[0].ID
			s.currentInst = inst.ID
			s.currentPort = inst.Port
			s.mu.Unlock()
			return tabs[0].ID, inst.Port, nil
		}
	}

	return "", "", fmt.Errorf("no tabs available; use POST /navigate to create one")
}

// portForTab finds the port of the instance that owns a tab.
func (s *Strategy) portForTab(tabID string) (string, error) {
	inst, err := s.mgr.FindInstanceByTabID(tabID)
	if err != nil {
		return "", fmt.Errorf("tab %q not found: %w", tabID, err)
	}
	return inst.Port, nil
}

// setCurrentTab updates the current tab and registers it in the locator cache.
func (s *Strategy) setCurrentTab(tabID string) {
	s.mu.Lock()
	s.currentTab = tabID
	s.mu.Unlock()

	// Register in locator cache for fast future lookups.
	s.mu.RLock()
	instID := s.currentInst
	s.mu.RUnlock()
	if instID != "" {
		s.mgr.RegisterTab(tabID, instID)
	}
}
