package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/state"
)

type currentBrowserStateResponse struct {
	TabID    string                         `json:"tabId"`
	URL      string                         `json:"url,omitempty"`
	Title    string                         `json:"title,omitempty"`
	Origins  []string                       `json:"origins"`
	Cookies  []state.Cookie                 `json:"cookies"`
	Storage  map[string]state.OriginStorage `json:"storage"`
	Metadata map[string]interface{}         `json:"metadata,omitempty"`
}

type capturedBrowserState struct {
	tabID string
	url   string
	title string
	file  *state.StateFile
}

// HandleStateCurrent returns the current gated browser state for a tab.
func (h *Handlers) HandleStateCurrent(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
		return
	}

	tabID := r.URL.Query().Get("tabId")
	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	captured, err := h.captureBrowserState(ctx, resolvedTabID, nil)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("capture state: %w", err))
		return
	}

	httpx.JSON(w, 200, currentBrowserStateResponse{
		TabID:    captured.tabID,
		URL:      captured.url,
		Title:    captured.title,
		Origins:  captured.file.Origins,
		Cookies:  captured.file.Cookies,
		Storage:  captured.file.Storage,
		Metadata: captured.file.Metadata,
	})
}

// HandleStateLoad reads a state file and restores cookies and storage.
func (h *Handlers) HandleStateLoad(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
		return
	}

	var req struct {
		Name  string `json:"name"`
		TabID string `json:"tabId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.Name == "" {
		httpx.Error(w, 400, fmt.Errorf("name is required"))
		return
	}

	encryptionKey := os.Getenv("PINCHTAB_STATE_KEY")
	path := state.ResolvePath(h.Config.StateDir, req.Name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		matches, matchErr := state.FindByPrefix(h.Config.StateDir, req.Name)
		if matchErr == nil && len(matches) > 0 {
			path = state.ResolvePath(h.Config.StateDir, matches[0].Name)
		}
	}
	sf, err := state.Load(path, encryptionKey)
	if err != nil {
		sf, err = state.Load(path, "")
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("load state: %w", err))
			return
		}
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 30*time.Second)
	defer tCancel()

	cookiesRestored := 0
	for _, c := range sf.Cookies {
		if err := h.Bridge.SetRawCookie(tCtx, bridge.RawSetCookieParams{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite,
		}); err == nil {
			cookiesRestored++
		}
	}

	storageRestored := 0
	for _, originStorage := range sf.Storage {
		if n, err := h.restoreOriginStorage(tCtx, originStorage); err == nil {
			storageRestored += n
		}
	}

	slog.Info("state loaded",
		"name", req.Name,
		"path", path,
		"cookiesRestored", cookiesRestored,
		"storageItemsRestored", storageRestored,
		"tabId", resolvedTabID,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, map[string]any{
		"name":                 req.Name,
		"cookiesRestored":      cookiesRestored,
		"storageItemsRestored": storageRestored,
		"origins":              sf.Origins,
	})
}

// restoreOriginStorage restores one origin's local and session storage in a single
// Evaluate call rather than one CDP round trip per key, returning the number of items
// the in-page script successfully set.
func (h *Handlers) restoreOriginStorage(ctx context.Context, originStorage state.OriginStorage) (int, error) {
	localJSON, _ := json.Marshal(originStorage.Local)
	sessionJSON, _ := json.Marshal(originStorage.Session)
	script := fmt.Sprintf(`(function(){
		var n = 0, local = %s, session = %s;
		for (var k in local) { try { localStorage.setItem(k, local[k]); n++; } catch(e){} }
		for (var k in session) { try { sessionStorage.setItem(k, session[k]); n++; } catch(e){} }
		return n;
	})()`, string(localJSON), string(sessionJSON))

	var n int
	if err := h.Bridge.Evaluate(ctx, script, &n, bridge.EvalOpts{}); err != nil {
		return 0, err
	}
	return n, nil
}

func (h *Handlers) captureBrowserState(ctx context.Context, resolvedTabID string, extraMetadata map[string]interface{}) (*capturedBrowserState, error) {
	tCtx, tCancel := context.WithTimeout(ctx, 30*time.Second)
	defer tCancel()

	cookies, err := h.Bridge.GetRawCookies(tCtx)
	if err != nil {
		return nil, fmt.Errorf("get cookies: %w", err)
	}

	storageScript := `
		(function() {
			try {
				var localEntries = {};
				for (var i = 0; i < localStorage.length; i++) {
					var k = localStorage.key(i);
					localEntries[k] = localStorage.getItem(k);
				}
				var sessionEntries = {};
				for (var i = 0; i < sessionStorage.length; i++) {
					var k = sessionStorage.key(i);
					sessionEntries[k] = sessionStorage.getItem(k);
				}
				return JSON.stringify({
					local: localEntries,
					session: sessionEntries,
					url: window.location.href,
					title: document.title,
					origin: window.location.origin,
					userAgent: navigator.userAgent
				});
			} catch(e) {
				return JSON.stringify({
					error: e.message,
					local: {},
					session: {},
					url: window.location.href,
					title: document.title,
					origin: window.location.origin,
					userAgent: navigator.userAgent
				});
			}
		})()
	`

	var storageJSON string
	if err := h.Bridge.Evaluate(tCtx, storageScript, &storageJSON, bridge.EvalOpts{}); err != nil {
		return nil, fmt.Errorf("evaluate storage: %w", err)
	}

	var storageResult struct {
		Local     map[string]string `json:"local"`
		Session   map[string]string `json:"session"`
		URL       string            `json:"url"`
		Title     string            `json:"title"`
		Origin    string            `json:"origin"`
		UserAgent string            `json:"userAgent"`
		Error     string            `json:"error"`
	}
	if err := json.Unmarshal([]byte(storageJSON), &storageResult); err != nil {
		return nil, fmt.Errorf("parse storage result: %w", err)
	}

	stateCookies := make([]state.Cookie, len(cookies))
	for i, c := range cookies {
		stateCookies[i] = state.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite,
			Expires:  c.Expires,
		}
	}
	if storageResult.Local == nil {
		storageResult.Local = map[string]string{}
	}
	if storageResult.Session == nil {
		storageResult.Session = map[string]string{}
	}

	origins := []string{}
	storageMap := map[string]state.OriginStorage{}
	if storageResult.Origin != "" {
		origins = append(origins, storageResult.Origin)
		storageMap[storageResult.Origin] = state.OriginStorage{
			Local:   storageResult.Local,
			Session: storageResult.Session,
		}
	}

	metadata := map[string]interface{}{
		"url":       storageResult.URL,
		"title":     storageResult.Title,
		"origin":    storageResult.Origin,
		"userAgent": storageResult.UserAgent,
	}
	if storageResult.Error != "" {
		metadata["storageError"] = storageResult.Error
	}
	for k, v := range extraMetadata {
		metadata[k] = v
	}

	return &capturedBrowserState{
		tabID: resolvedTabID,
		url:   storageResult.URL,
		title: storageResult.Title,
		file: &state.StateFile{
			SavedAt:  time.Now(),
			Origins:  origins,
			Cookies:  stateCookies,
			Storage:  storageMap,
			Metadata: metadata,
		},
	}, nil
}

func firstOrigin(origins []string) string {
	if len(origins) == 0 {
		return ""
	}
	return origins[0]
}
