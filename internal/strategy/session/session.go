// Package session implements the "session" allocation strategy.
//
// Agents get a session ID; Pinchtab manages instance and tab lifecycle.
// Sessions provide sticky routing — once a session is bound to an instance,
// all operations go to that instance until the session expires or is deleted.
//
// Key endpoints:
//
//	POST /session          — create a new session (allocates instance + tab)
//	POST /browse           — navigate + snapshot in one call (auto-creates session)
//	GET  /session/{id}     — get session info
//	DELETE /session/{id}   — delete session (closes tab)
//	GET  /sessions         — list all sessions
//
// Tab operations use X-Session-ID header for routing. Falls back to tab ID in path.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/instance"
	"github.com/pinchtab/pinchtab/internal/primitive"
	"github.com/pinchtab/pinchtab/internal/web"
)

// Config holds session strategy configuration.
type Config struct {
	SessionTTL time.Duration
	AutoLaunch bool
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		SessionTTL: 30 * time.Minute,
		AutoLaunch: true,
	}
}

// Session represents an agent's browser session.
type Session struct {
	ID         string    `json:"id"`
	TabID      string    `json:"tabId"`
	InstanceID string    `json:"instanceId"`
	Port       string    `json:"port"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsed   time.Time `json:"lastUsed"`
	mu         sync.Mutex
}

// Strategy manages sessions for agents.
// Each session is sticky to one instance — allocation happens once at session creation.
type Strategy struct {
	mgr    *instance.Manager
	bridge *instance.BridgeClient
	config Config

	sessions map[string]*Session
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// New creates a Session strategy backed by the given InstanceManager.
func New(mgr *instance.Manager) *Strategy {
	return &Strategy{
		mgr:      mgr,
		bridge:   instance.NewBridgeClient(),
		config:   DefaultConfig(),
		sessions: make(map[string]*Session),
	}
}

func (s *Strategy) Name() string { return "session" }

// Init receives primitives (unused — Session strategy uses instance.Manager directly).
func (s *Strategy) Init(_ *primitive.Primitives) error { return nil }

// Start begins the session cleanup goroutine.
func (s *Strategy) Start(ctx context.Context) error {
	s.stopCh = make(chan struct{})
	go s.cleanupLoop(ctx)
	return nil
}

// Stop terminates background tasks.
func (s *Strategy) Stop() error {
	if s.stopCh != nil {
		close(s.stopCh)
	}
	return nil
}

// RegisterRoutes adds session and tab endpoints to the mux.
func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	// Session management.
	mux.HandleFunc("POST /session", s.handleCreateSession)
	mux.HandleFunc("GET /session/{id}", s.handleGetSession)
	mux.HandleFunc("DELETE /session/{id}", s.handleDeleteSession)
	mux.HandleFunc("GET /sessions", s.handleListSessions)

	// Browse: navigate + snapshot in one call (auto-creates session).
	mux.HandleFunc("POST /browse", s.handleBrowse)

	// Tab operations (use X-Session-ID header for routing).
	mux.HandleFunc("POST /navigate", s.handleNavigate)
	mux.HandleFunc("GET /navigate", s.handleNavigate)
	mux.HandleFunc("GET /snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /screenshot", s.handleScreenshot)
	mux.HandleFunc("GET /text", s.handleText)
	mux.HandleFunc("POST /action", s.handleAction)
	mux.HandleFunc("POST /actions", s.handleActions)
	mux.HandleFunc("POST /evaluate", s.handleEvaluate)
	mux.HandleFunc("POST /find", s.handleFind)
	mux.HandleFunc("GET /cookies", s.handleGetCookies)
	mux.HandleFunc("POST /cookies", s.handleSetCookies)

	// Tab-specific endpoints (explicit tab ID in path).
	mux.HandleFunc("POST /tabs/{id}/navigate", s.handleTabProxy("/navigate"))
	mux.HandleFunc("GET /tabs/{id}/snapshot", s.handleTabProxy("/snapshot"))
	mux.HandleFunc("GET /tabs/{id}/screenshot", s.handleTabProxy("/screenshot"))
	mux.HandleFunc("GET /tabs/{id}/text", s.handleTabProxy("/text"))
	mux.HandleFunc("POST /tabs/{id}/action", s.handleTabProxy("/action"))
	mux.HandleFunc("POST /tabs/{id}/actions", s.handleTabProxy("/actions"))
	mux.HandleFunc("POST /tabs/{id}/evaluate", s.handleTabProxy("/evaluate"))
	mux.HandleFunc("GET /tabs/{id}/pdf", s.handleTabProxy("/pdf"))
	mux.HandleFunc("POST /tabs/{id}/pdf", s.handleTabProxy("/pdf"))
	mux.HandleFunc("GET /tabs/{id}/cookies", s.handleTabProxy("/cookies"))
	mux.HandleFunc("POST /tabs/{id}/cookies", s.handleTabProxy("/cookies"))
	mux.HandleFunc("POST /tabs/{id}/find", s.handleTabProxy("/find"))
	mux.HandleFunc("POST /tabs/{id}/close", s.handleTabClose)
	mux.HandleFunc("DELETE /tabs/{id}", s.handleTabClose)

	// Tab + instance listing.
	mux.HandleFunc("POST /tab", s.handleTabManage)
	mux.HandleFunc("GET /tabs", s.handleListTabs)
}

// --- Session management ---

func (s *Strategy) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.createSession(r.Context())
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}

	web.JSON(w, http.StatusCreated, map[string]string{
		"sessionId": sess.ID,
		"tabId":     sess.TabID,
	})
}

func (s *Strategy) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		web.Error(w, http.StatusNotFound, fmt.Errorf("session %q not found", id))
		return
	}

	web.JSON(w, http.StatusOK, sess)
}

func (s *Strategy) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.mu.Lock()
	sess, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
	}
	s.mu.Unlock()

	if !ok {
		web.Error(w, http.StatusNotFound, fmt.Errorf("session %q not found", id))
		return
	}

	// Close the tab.
	if err := s.bridge.CloseTab(r.Context(), sess.Port, sess.TabID); err != nil {
		slog.Warn("failed to close session tab", "session", id, "tab", sess.TabID, "err", err)
	}
	s.mgr.InvalidateTab(sess.TabID)

	slog.Info("session deleted", "session", id)
	web.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Strategy) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.RUnlock()

	web.JSON(w, http.StatusOK, sessions)
}

// --- Browse: navigate + snapshot in one call ---

func (s *Strategy) handleBrowse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL       string `json:"url"`
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
		return
	}

	if req.URL == "" {
		web.Error(w, http.StatusBadRequest, fmt.Errorf("url is required"))
		return
	}

	// Use existing session from body or header.
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = r.Header.Get("X-Session-ID")
	}

	sess, err := s.getOrCreateSession(r.Context(), sessionID)
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}

	sess.mu.Lock()
	sess.LastUsed = time.Now()
	sess.mu.Unlock()

	// Navigate the session's tab.
	s.bridge.ProxyToTab(w, r, sess.Port, sess.TabID, "/navigate")
}

// --- Shorthand handlers (use X-Session-ID header) ---

func (s *Strategy) handleNavigate(w http.ResponseWriter, r *http.Request) {
	var reqURL string
	if r.Method == http.MethodGet {
		reqURL = r.URL.Query().Get("url")
	} else {
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			web.Error(w, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
			return
		}
		reqURL = req.URL
	}

	if reqURL == "" {
		web.Error(w, http.StatusBadRequest, fmt.Errorf("url is required"))
		return
	}

	sess, err := s.getOrCreateSession(r.Context(), r.Header.Get("X-Session-ID"))
	if err != nil {
		web.Error(w, http.StatusServiceUnavailable, err)
		return
	}

	sess.mu.Lock()
	sess.LastUsed = time.Now()
	sess.mu.Unlock()

	// Create a new tab for this navigation.
	tabID, createErr := s.bridge.CreateTab(r.Context(), sess.Port, reqURL)
	if createErr != nil {
		web.Error(w, http.StatusInternalServerError, fmt.Errorf("create tab: %w", createErr))
		return
	}

	// Update session's current tab.
	sess.mu.Lock()
	oldTab := sess.TabID
	sess.TabID = tabID
	sess.mu.Unlock()

	s.mgr.RegisterTab(tabID, sess.InstanceID)

	// Close old tab (best effort).
	if oldTab != "" && oldTab != tabID {
		_ = s.bridge.CloseTab(r.Context(), sess.Port, oldTab)
		s.mgr.InvalidateTab(oldTab)
	}

	web.JSON(w, http.StatusOK, map[string]string{
		"sessionId": sess.ID,
		"tabId":     tabID,
		"url":       reqURL,
	})
}

func (s *Strategy) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}
	s.touchSession(sess)
	s.bridge.ProxyToTab(w, r, sess.Port, sess.TabID, "/snapshot")
}

func (s *Strategy) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}
	s.touchSession(sess)
	s.bridge.ProxyToTab(w, r, sess.Port, sess.TabID, "/screenshot")
}

func (s *Strategy) handleText(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}
	s.touchSession(sess)
	s.bridge.ProxyToTab(w, r, sess.Port, sess.TabID, "/text")
}

func (s *Strategy) handleAction(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}
	s.touchSession(sess)
	s.bridge.ProxyToTab(w, r, sess.Port, sess.TabID, "/action")
}

func (s *Strategy) handleActions(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}
	s.touchSession(sess)
	s.bridge.ProxyToTab(w, r, sess.Port, sess.TabID, "/actions")
}

func (s *Strategy) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}
	s.touchSession(sess)
	s.bridge.ProxyToTab(w, r, sess.Port, sess.TabID, "/evaluate")
}

func (s *Strategy) handleFind(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}
	s.touchSession(sess)
	// Bridge HandleFind auto-snapshots and accepts tabId in body.
	s.bridge.ProxyWithTabID(w, r, sess.Port, sess.TabID, "/find")
}

func (s *Strategy) handleGetCookies(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}
	s.touchSession(sess)
	s.bridge.ProxyToTab(w, r, sess.Port, sess.TabID, "/cookies")
}

func (s *Strategy) handleSetCookies(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}
	s.touchSession(sess)
	s.bridge.ProxyToTab(w, r, sess.Port, sess.TabID, "/cookies")
}

// --- Tab-specific handlers ---

// handleTabProxy returns a handler that proxies to the bridge for the given suffix.
func (s *Strategy) handleTabProxy(suffix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tabID := r.PathValue("id")
		port, err := s.portForTab(tabID)
		if err != nil {
			web.Error(w, http.StatusNotFound, err)
			return
		}
		s.bridge.ProxyToTab(w, r, port, tabID, suffix)
	}
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

	// Remove from any session that owned this tab.
	s.mu.RLock()
	for _, sess := range s.sessions {
		if sess.TabID == tabID {
			sess.mu.Lock()
			sess.TabID = ""
			sess.mu.Unlock()
		}
	}
	s.mu.RUnlock()

	web.JSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

// --- Tab + instance management ---

func (s *Strategy) handleTabManage(w http.ResponseWriter, r *http.Request) {
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
		sess, err := s.getOrCreateSession(r.Context(), r.Header.Get("X-Session-ID"))
		if err != nil {
			web.Error(w, http.StatusServiceUnavailable, err)
			return
		}
		url := req.URL
		if url == "" {
			url = "about:blank"
		}
		tabID, err := s.bridge.CreateTab(r.Context(), sess.Port, url)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err)
			return
		}
		s.mgr.RegisterTab(tabID, sess.InstanceID)
		web.JSON(w, http.StatusOK, map[string]string{"tabId": tabID, "url": url, "sessionId": sess.ID})

	case "close":
		if req.TabID == "" {
			web.Error(w, http.StatusBadRequest, fmt.Errorf("tabId required"))
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

func (s *Strategy) handleListTabs(w http.ResponseWriter, _ *http.Request) {
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

// --- Internal helpers ---

// createSession allocates an instance and creates a tab for a new session.
func (s *Strategy) createSession(ctx context.Context) (*Session, error) {
	inst, err := s.mgr.Allocate()
	if err != nil {
		return nil, fmt.Errorf("allocate instance: %w", err)
	}

	tabID, err := s.bridge.CreateTab(ctx, inst.Port, "about:blank")
	if err != nil {
		return nil, fmt.Errorf("create tab: %w", err)
	}

	sess := &Session{
		ID:         "sess_" + generateID(8),
		TabID:      tabID,
		InstanceID: inst.ID,
		Port:       inst.Port,
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
	}

	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()

	s.mgr.RegisterTab(tabID, inst.ID)

	slog.Info("session created", "session", sess.ID, "tab", tabID, "instance", inst.ID)
	return sess, nil
}

// getOrCreateSession finds an existing session or creates a new one.
func (s *Strategy) getOrCreateSession(ctx context.Context, sessionID string) (*Session, error) {
	if sessionID != "" {
		s.mu.RLock()
		sess, ok := s.sessions[sessionID]
		s.mu.RUnlock()
		if ok {
			return sess, nil
		}
	}
	return s.createSession(ctx)
}

// sessionFromRequest extracts a session from the X-Session-ID header.
func (s *Strategy) sessionFromRequest(r *http.Request) (*Session, error) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		return nil, fmt.Errorf("X-Session-ID header required")
	}

	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	return sess, nil
}

// portForTab finds the port of the instance that owns a tab.
func (s *Strategy) portForTab(tabID string) (string, error) {
	inst, err := s.mgr.FindInstanceByTabID(tabID)
	if err != nil {
		return "", fmt.Errorf("tab %q not found: %w", tabID, err)
	}
	return inst.Port, nil
}

// touchSession updates the session's LastUsed timestamp.
func (s *Strategy) touchSession(sess *Session) {
	sess.mu.Lock()
	sess.LastUsed = time.Now()
	sess.mu.Unlock()
}

// cleanupLoop removes expired sessions periodically.
func (s *Strategy) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanupExpired()
		}
	}
}

func (s *Strategy) cleanupExpired() {
	now := time.Now()
	var expired []string

	s.mu.RLock()
	for id, sess := range s.sessions {
		if now.Sub(sess.LastUsed) > s.config.SessionTTL {
			expired = append(expired, id)
		}
	}
	s.mu.RUnlock()

	for _, id := range expired {
		s.mu.Lock()
		sess, ok := s.sessions[id]
		if ok {
			delete(s.sessions, id)
		}
		s.mu.Unlock()

		if ok && sess.TabID != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.bridge.CloseTab(ctx, sess.Port, sess.TabID)
			cancel()
			s.mgr.InvalidateTab(sess.TabID)
			slog.Info("session expired", "session", id, "tab", sess.TabID)
		}
	}
}

func generateID(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
