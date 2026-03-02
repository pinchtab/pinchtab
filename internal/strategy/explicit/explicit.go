// Package explicit implements the default "power user" strategy.
// All primitive endpoints are exposed directly - agents manage
// instances, profiles, and tabs explicitly.
package explicit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/primitive"
	"github.com/pinchtab/pinchtab/internal/strategy"
	"github.com/pinchtab/pinchtab/internal/web"
)

func init() {
	strategy.Register("explicit", func() strategy.Strategy {
		return &Strategy{}
	})
}

// Strategy exposes all primitives as REST endpoints.
// This is the default behavior - full control for power users.
type Strategy struct {
	p *primitive.Primitives
}

// Name returns the strategy identifier.
func (s *Strategy) Name() string {
	return "explicit"
}

// Init receives primitives.
func (s *Strategy) Init(p *primitive.Primitives) error {
	s.p = p
	return nil
}

// Start is a no-op for explicit strategy.
func (s *Strategy) Start(ctx context.Context) error {
	return nil
}

// Stop is a no-op for explicit strategy.
func (s *Strategy) Stop() error {
	return nil
}

// RegisterRoutes adds all explicit endpoints.
func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	// Instance endpoints
	mux.HandleFunc("GET /instances", s.handleListInstances)
	mux.HandleFunc("POST /instances/launch", s.handleLaunch)
	mux.HandleFunc("GET /instances/{id}", s.handleGetInstance)
	mux.HandleFunc("POST /instances/{id}/stop", s.handleStop)

	// Tab management via instance
	mux.HandleFunc("POST /instances/{id}/tabs/open", s.handleOpenTab)
	mux.HandleFunc("GET /instances/{id}/tabs", s.handleListInstanceTabs)

	// Aggregated tabs
	mux.HandleFunc("GET /tabs", s.handleListAllTabs)
	mux.HandleFunc("GET /instances/tabs", s.handleListAllTabs)

	// Tab operations
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
	mux.HandleFunc("POST /tabs/{id}/lock", s.handleLock)
	mux.HandleFunc("POST /tabs/{id}/unlock", s.handleUnlock)

	// Profile endpoints
	mux.HandleFunc("GET /profiles", s.handleListProfiles)
	mux.HandleFunc("POST /profiles", s.handleCreateProfile)
	mux.HandleFunc("GET /profiles/{id}", s.handleGetProfile)
	mux.HandleFunc("DELETE /profiles/{id}", s.handleDeleteProfile)
	mux.HandleFunc("POST /profiles/{id}/reset", s.handleResetProfile)
}

// Instance handlers

func (s *Strategy) handleListInstances(w http.ResponseWriter, r *http.Request) {
	instances := s.p.Instances.List()
	web.JSON(w, http.StatusOK, instances)
}

func (s *Strategy) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile  string `json:"profile"`
		Port     int    `json:"port"`
		Headless *bool  `json:"headless"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}

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

func (s *Strategy) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := s.p.Instances.Get(id)
	if !ok {
		web.Error(w, http.StatusNotFound, fmt.Errorf("instance not found"))
		return
	}
	web.JSON(w, http.StatusOK, inst)
}

func (s *Strategy) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.p.Instances.Stop(r.Context(), id); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// Tab management handlers

func (s *Strategy) handleOpenTab(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.URL = "about:blank"
	}

	tabID, err := s.p.Tabs.Open(r.Context(), instanceID, req.URL)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}

	web.JSON(w, http.StatusOK, map[string]string{"tabId": tabID})
}

func (s *Strategy) handleListInstanceTabs(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	tabs, err := s.p.Tabs.List(r.Context(), instanceID)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, tabs)
}

func (s *Strategy) handleListAllTabs(w http.ResponseWriter, r *http.Request) {
	tabs, err := s.p.Tabs.ListAll(r.Context())
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, tabs)
}

func (s *Strategy) handleCloseTab(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if err := s.p.Tabs.Close(r.Context(), tabID); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

// Tab operation handlers

func (s *Strategy) handleNavigate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	var req struct {
		URL         string `json:"url"`
		Timeout     int    `json:"timeout"`
		WaitUntil   string `json:"waitUntil"`
		BlockImages bool   `json:"blockImages"`
		BlockMedia  bool   `json:"blockMedia"`
		BlockAds    bool   `json:"blockAds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, http.StatusBadRequest, err)
		return
	}

	opts := primitive.NavigateOpts{
		WaitUntil:   req.WaitUntil,
		BlockImages: req.BlockImages,
		BlockMedia:  req.BlockMedia,
		BlockAds:    req.BlockAds,
	}

	if err := s.p.Tabs.Navigate(r.Context(), tabID, req.URL, opts); err != nil {
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
	q := r.URL.Query()

	opts := primitive.ScreenshotOpts{
		Format:   q.Get("format"),
		FullPage: q.Get("fullPage") == "true",
		Selector: q.Get("selector"),
	}

	data, err := s.p.Tabs.Screenshot(r.Context(), tabID, opts)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}

	contentType := "image/png"
	if opts.Format == "jpeg" {
		contentType = "image/jpeg"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func (s *Strategy) handlePDF(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")

	opts := primitive.PDFOpts{
		Landscape:       r.URL.Query().Get("landscape") == "true",
		PrintBackground: r.URL.Query().Get("printBackground") == "true",
	}

	data, err := s.p.Tabs.PDF(r.Context(), tabID, opts)
	if err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(data)
}

func (s *Strategy) handleText(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	opts := primitive.TextOpts{
		Raw: r.URL.Query().Get("raw") == "true",
	}

	result, err := s.p.Tabs.Text(r.Context(), tabID, opts)
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

func (s *Strategy) handleLock(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	_ = tabID // TODO: implement via primitive

	web.JSON(w, http.StatusOK, map[string]string{"status": "locked"})
}

func (s *Strategy) handleUnlock(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	_ = tabID // TODO: implement via primitive

	web.JSON(w, http.StatusOK, map[string]string{"status": "unlocked"})
}

// Profile handlers

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

func (s *Strategy) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	profile, err := s.p.Profiles.Get(id)
	if err != nil {
		web.Error(w, http.StatusNotFound, err)
		return
	}
	web.JSON(w, http.StatusOK, profile)
}

func (s *Strategy) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.p.Profiles.Delete(id); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Strategy) handleResetProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.p.Profiles.Reset(id); err != nil {
		web.Error(w, http.StatusInternalServerError, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"status": "reset"})
}
