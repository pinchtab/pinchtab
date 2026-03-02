// Package session implements the "casual" strategy.
// Agents get a session ID; Pinchtab manages instance and tab lifecycle.
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

	"github.com/pinchtab/pinchtab/internal/primitive"
	"github.com/pinchtab/pinchtab/internal/strategy"
	"github.com/pinchtab/pinchtab/internal/web"
)

func init() {
	strategy.Register("session", func() strategy.Strategy {
		return &Strategy{
			sessions: make(map[string]*Session),
		}
	})
}

// Config holds session strategy configuration.
type Config struct {
	DefaultProfile string        `json:"defaultProfile" yaml:"default_profile"`
	SessionTTL     time.Duration `json:"sessionTTL" yaml:"session_ttl"`
	AutoLaunch     bool          `json:"autoLaunch" yaml:"auto_launch"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		DefaultProfile: "default",
		SessionTTL:     30 * time.Minute,
		AutoLaunch:     true,
	}
}

// Session represents an agent's browser session.
type Session struct {
	ID         string    `json:"id"`
	TabID      string    `json:"tabId"`
	InstanceID string    `json:"instanceId"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsed   time.Time `json:"lastUsed"`
	mu         sync.Mutex
}

// Strategy manages sessions for casual agents.
type Strategy struct {
	p        *primitive.Primitives
	config   Config
	sessions map[string]*Session
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// Name returns the strategy identifier.
func (s *Strategy) Name() string {
	return "session"
}

// Init receives primitives.
func (s *Strategy) Init(p *primitive.Primitives) error {
	s.p = p
	s.config = DefaultConfig()
	return nil
}

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

// RegisterRoutes adds session endpoints plus primitive endpoints.
func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	// Session-specific endpoints
	mux.HandleFunc("POST /session", s.handleCreateSession)
	mux.HandleFunc("GET /session/{id}", s.handleGetSession)
	mux.HandleFunc("DELETE /session/{id}", s.handleDeleteSession)
	mux.HandleFunc("GET /sessions", s.handleListSessions)

	// Simplified browsing endpoint
	mux.HandleFunc("POST /browse", s.handleBrowse)

	// Also expose primitive endpoints for power users
	// Instances
	mux.HandleFunc("GET /instances", s.handleListInstances)
	mux.HandleFunc("POST /instances/launch", s.handleLaunch)
	mux.HandleFunc("POST /instances/{id}/stop", s.handleStop)

	// Tabs (passthrough to primitives)
	mux.HandleFunc("GET /tabs", s.handleListTabs)
	mux.HandleFunc("POST /tabs/{id}/navigate", s.handleNavigate)
	mux.HandleFunc("GET /tabs/{id}/snapshot", s.handleSnapshot)
	mux.HandleFunc("POST /tabs/{id}/action", s.handleAction)
	mux.HandleFunc("POST /tabs/{id}/actions", s.handleActions)
	mux.HandleFunc("GET /tabs/{id}/screenshot", s.handleScreenshot)
	mux.HandleFunc("GET /tabs/{id}/pdf", s.handlePDF)
	mux.HandleFunc("GET /tabs/{id}/text", s.handleText)
	mux.HandleFunc("POST /tabs/{id}/evaluate", s.handleEvaluate)
	mux.HandleFunc("GET /tabs/{id}/cookies", s.handleGetCookies)
	mux.HandleFunc("POST /tabs/{id}/cookies", s.handleSetCookies)
	mux.HandleFunc("POST /tabs/{id}/close", s.handleCloseTab)

	// Profiles
	mux.HandleFunc("GET /profiles", s.handleListProfiles)
	mux.HandleFunc("POST /profiles", s.handleCreateProfile)
}

// Session handlers

func (s *Strategy) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Ensure instance running
	inst := s.p.Instances.FirstRunning()
	if inst == nil {
		if !s.config.AutoLaunch {
			web.Error(w, http.StatusServiceUnavailable, fmt.Errorf("no running instance and auto-launch disabled"))
			return
		}

		var err error
		inst, err = s.p.Instances.Launch(ctx, s.config.DefaultProfile, 0, true)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err)
			return
		}

		// Wait for instance to be ready
		if err := s.p.Instances.WaitReady(ctx, inst.ID); err != nil {
			web.Error(w, http.StatusInternalServerError, err)
			return
		}
	}

	// Open tab
	tabID, err := s.p.Tabs.Open(ctx, inst.ID, "about:blank")
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}

	// Create session
	sess := &Session{
		ID:         "sess_" + generateID(8),
		TabID:      tabID,
		InstanceID: inst.ID,
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
	}

	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()

	slog.Info("session created", "sessionId", sess.ID, "tabId", tabID)

	web.JSON(w, http.StatusCreated, map[string]string{
		"session_id": sess.ID,
		"tab_id":     sess.TabID,
	})
}

func (s *Strategy) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		web.Error(w, http.StatusNotFound, fmt.Errorf("session not found"))
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
		web.Error(w, http.StatusNotFound, fmt.Errorf("session not found"))
		return
	}

	// Close the tab
	if err := s.p.Tabs.Close(r.Context(), sess.TabID); err != nil {
		slog.Warn("failed to close session tab", "sessionId", id, "tabId", sess.TabID, "err", err)
	}

	slog.Info("session deleted", "sessionId", id)
	web.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Strategy) handleListSessions(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.RUnlock()

	web.JSON(w, http.StatusOK, sessions)
}

// handleBrowse is the simplified browsing endpoint.
// Auto-allocates a session if needed, navigates, returns snapshot.
func (s *Strategy) handleBrowse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.Header.Get("X-Session-ID")

	var sess *Session

	// Find existing session
	if sessionID != "" {
		s.mu.RLock()
		sess = s.sessions[sessionID]
		s.mu.RUnlock()
	}

	// Create new session if needed
	if sess == nil {
		inst := s.p.Instances.FirstRunning()
		if inst == nil {
			if !s.config.AutoLaunch {
				web.Error(w, http.StatusServiceUnavailable, fmt.Errorf("no running instance"))
				return
			}
			var err error
			inst, err = s.p.Instances.Launch(ctx, s.config.DefaultProfile, 0, true)
			if err != nil {
				web.Error(w, http.StatusInternalServerError, err)
				return
			}
			if err := s.p.Instances.WaitReady(ctx, inst.ID); err != nil {
				web.Error(w, http.StatusInternalServerError, err)
				return
			}
		}

		tabID, err := s.p.Tabs.Open(ctx, inst.ID, "about:blank")
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err)
			return
		}

		sess = &Session{
			ID:         "sess_" + generateID(8),
			TabID:      tabID,
			InstanceID: inst.ID,
			CreatedAt:  time.Now(),
			LastUsed:   time.Now(),
		}

		s.mu.Lock()
		s.sessions[sess.ID] = sess
		s.mu.Unlock()
	}

	// Update last used
	sess.mu.Lock()
	sess.LastUsed = time.Now()
	sess.mu.Unlock()

	// Parse request
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}

	// Navigate
	if err := s.p.Tabs.Navigate(ctx, sess.TabID, req.URL, primitive.NavigateOpts{}); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}

	// Get snapshot
	snap, err := s.p.Tabs.Snapshot(ctx, sess.TabID, primitive.SnapshotOpts{
		Interactive: true,
		Compact:     true,
	})
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}

	web.JSON(w, http.StatusOK, map[string]any{
		"session_id": sess.ID,
		"tab_id":     sess.TabID,
		"snapshot":   snap,
	})
}

// Primitive passthrough handlers

func (s *Strategy) handleListInstances(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, http.StatusOK, s.p.Instances.List())
}

func (s *Strategy) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile  string `json:"profile"`
		Port     int    `json:"port"`
		Headless *bool  `json:"headless"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // Allow empty body

	headless := true
	if req.Headless != nil {
		headless = *req.Headless
	}

	inst, err := s.p.Instances.Launch(r.Context(), req.Profile, req.Port, headless)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusCreated, inst)
}

func (s *Strategy) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.p.Instances.Stop(r.Context(), id); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Strategy) handleListTabs(w http.ResponseWriter, r *http.Request) {
	tabs, err := s.p.Tabs.ListAll(r.Context())
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, tabs)
}

func (s *Strategy) handleNavigate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}

	if err := s.p.Tabs.Navigate(r.Context(), tabID, req.URL, primitive.NavigateOpts{}); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Strategy) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	q := r.URL.Query()

	opts := primitive.SnapshotOpts{
		Interactive: q.Get("interactive") == "true",
		Compact:     q.Get("compact") == "true",
		Format:      q.Get("format"),
		Diff:        q.Get("diff") == "true",
	}

	snap, err := s.p.Tabs.Snapshot(r.Context(), tabID, opts)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, snap)
}

func (s *Strategy) handleAction(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	var action primitive.Action
	if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}

	result, err := s.p.Tabs.Action(r.Context(), tabID, action)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, result)
}

func (s *Strategy) handleActions(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	var actions []primitive.Action
	if err := json.NewDecoder(r.Body).Decode(&actions); err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}

	results, err := s.p.Tabs.Actions(r.Context(), tabID, actions)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, results)
}

func (s *Strategy) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	data, err := s.p.Tabs.Screenshot(r.Context(), tabID, primitive.ScreenshotOpts{})
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}

func (s *Strategy) handlePDF(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	data, err := s.p.Tabs.PDF(r.Context(), tabID, primitive.PDFOpts{})
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(data)
}

func (s *Strategy) handleText(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	result, err := s.p.Tabs.Text(r.Context(), tabID, primitive.TextOpts{})
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, result)
}

func (s *Strategy) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	var req struct {
		Expression string `json:"expression"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}

	result, err := s.p.Tabs.Evaluate(r.Context(), tabID, req.Expression)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Strategy) handleGetCookies(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	cookies, err := s.p.Tabs.Cookies(r.Context(), tabID)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, cookies)
}

func (s *Strategy) handleSetCookies(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	var req struct {
		URL     string              `json:"url"`
		Cookies []*primitive.Cookie `json:"cookies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}

	if err := s.p.Tabs.SetCookies(r.Context(), tabID, req.URL, req.Cookies); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Strategy) handleCloseTab(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if err := s.p.Tabs.Close(r.Context(), tabID); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

func (s *Strategy) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.p.Profiles.List()
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, profiles)
}

func (s *Strategy) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}

	if err := s.p.Profiles.Create(req.Name); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]string{"status": "created", "name": req.Name})
}

// cleanupLoop removes expired sessions.
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

		if ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.p.Tabs.Close(ctx, sess.TabID)
			cancel()
			slog.Info("session expired", "sessionId", id, "tabId", sess.TabID)
		}
	}
}

func generateID(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
