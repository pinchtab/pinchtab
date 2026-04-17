package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/state"
)

func (h *Handlers) stateExportEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowStateExport
}

// HandleStateList lists all saved state files.
// Gated behind CapStateExport (security.allowStateExport).
//
// @Endpoint GET /state/list
func (h *Handlers) HandleStateList(w http.ResponseWriter, r *http.Request) {
	if !h.stateExportEnabled() {
		httpx.ErrorCode(w, 403, "state_export_disabled", httpx.DisabledEndpointMessage("stateExport", "security.allowStateExport"), false, map[string]any{
			"setting": "security.allowStateExport",
		})
		return
	}

	entries, err := state.List(h.Config.StateDir)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("list states: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "state.list"})

	httpx.JSON(w, 200, map[string]any{
		"states": entries,
		"count":  len(entries),
	})
}

// HandleStateShow returns the full contents of a saved state file.
// Gated behind CapStateExport (security.allowStateExport).
//
// @Endpoint GET /state/show
func (h *Handlers) HandleStateShow(w http.ResponseWriter, r *http.Request) {
	if !h.stateExportEnabled() {
		httpx.ErrorCode(w, 403, "state_export_disabled", httpx.DisabledEndpointMessage("stateExport", "security.allowStateExport"), false, map[string]any{
			"setting": "security.allowStateExport",
		})
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		httpx.Error(w, 400, fmt.Errorf("name query parameter is required"))
		return
	}

	encryptionKey := h.Config.StateEncryptionKey
	path := state.ResolvePath(h.Config.StateDir, name)

	sf, err := state.Load(path, encryptionKey)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("load state: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "state.show"})

	httpx.JSON(w, 200, sf)
}

type stateSaveRequest struct {
	Name     string                 `json:"name"`
	Encrypt  bool                   `json:"encrypt"`
	TabID    string                 `json:"tabId"`
	Metadata map[string]interface{} `json:"metadata"`
}

// HandleStateSave captures the current browser state (cookies, storage, metadata)
// and writes it to disk. Gated behind CapStateExport (security.allowStateExport).
// Storage is captured only for the current origin (active tab).
//
// @Endpoint POST /state/save
func (h *Handlers) HandleStateSave(w http.ResponseWriter, r *http.Request) {
	if !h.stateExportEnabled() {
		httpx.ErrorCode(w, 403, "state_export_disabled", httpx.DisabledEndpointMessage("stateExport", "security.allowStateExport"), false, map[string]any{
			"setting": "security.allowStateExport",
		})
		return
	}

	var req stateSaveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	encryptionKey := ""
	if req.Encrypt {
		encryptionKey = h.Config.StateEncryptionKey
		if err := state.ValidateEncryptionKey(encryptionKey); err != nil {
			httpx.Error(w, 400, fmt.Errorf("encryption key required: set security.stateEncryptionKey in config"))
			return
		}
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 30*time.Second)
	defer tCancel()

	// Step 1: Get all cookies
	var cookies []*network.Cookie
	if err := chromedp.Run(tCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().Do(ctx)
		return err
	})); err != nil {
		httpx.Error(w, 500, fmt.Errorf("get cookies: %w", err))
		return
	}

	// Step 2: Get storage and metadata via JS evaluation
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
					origin: window.location.origin,
					userAgent: navigator.userAgent
				});
			} catch(e) {
				return JSON.stringify({
					error: e.message,
					local: {},
					session: {},
					url: window.location.href,
					origin: window.location.origin,
					userAgent: navigator.userAgent
				});
			}
		})()
	`

	var storageJSON string
	if err := chromedp.Run(tCtx, chromedp.Evaluate(storageScript, &storageJSON)); err != nil {
		httpx.Error(w, 500, fmt.Errorf("evaluate storage: %w", err))
		return
	}

	var storageResult struct {
		Local     map[string]string `json:"local"`
		Session   map[string]string `json:"session"`
		URL       string            `json:"url"`
		Origin    string            `json:"origin"`
		UserAgent string            `json:"userAgent"`
		Error     string            `json:"error"`
	}
	if err := json.Unmarshal([]byte(storageJSON), &storageResult); err != nil {
		httpx.Error(w, 500, fmt.Errorf("parse storage result: %w", err))
		return
	}

	// Build state file
	stateCookies := make([]state.Cookie, len(cookies))
	for i, c := range cookies {
		stateCookies[i] = state.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite.String(),
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
	if storageResult.Origin != "" {
		origins = append(origins, storageResult.Origin)
	}

	metadata := map[string]interface{}{
		"url":       storageResult.URL,
		"origin":    storageResult.Origin,
		"userAgent": storageResult.UserAgent,
	}
	// Namespace user-provided metadata under "custom" to prevent overwriting
	// browser-captured provenance fields (url, origin, userAgent).
	if len(req.Metadata) > 0 {
		metadata["custom"] = req.Metadata
	}

	sf := &state.StateFile{
		Name:    req.Name,
		SavedAt: time.Now(),
		Origins: origins,
		Cookies: stateCookies,
		Storage: map[string]state.OriginStorage{
			storageResult.Origin: {
				Local:   storageResult.Local,
				Session: storageResult.Session,
			},
		},
		Metadata: metadata,
	}

	path, err := state.Save(h.Config.StateDir, sf, encryptionKey)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("save state: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "state.save"})

	slog.Info("state saved",
		"name", sf.Name,
		"path", path,
		"cookies", len(stateCookies),
		"origin", storageResult.Origin,
		"encrypted", req.Encrypt,
		"tabId", resolvedTabID,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, map[string]any{
		"name":      sf.Name,
		"path":      path,
		"cookies":   len(stateCookies),
		"origins":   origins,
		"encrypted": req.Encrypt,
	})
}

// HandleStateLoad reads a state file and restores cookies and storage into the browser.
// Gated behind CapStateExport: restoring cookies/storage is session injection.
//
// If the given name has no exact match, the most recent file whose name starts
// with the given prefix is used (prefix-based loading).
//
// @Endpoint POST /state/load
func (h *Handlers) HandleStateLoad(w http.ResponseWriter, r *http.Request) {
	if !h.stateExportEnabled() {
		httpx.ErrorCode(w, 403, "state_export_disabled", httpx.DisabledEndpointMessage("stateExport", "security.allowStateExport"), false, map[string]any{
			"setting": "security.allowStateExport",
		})
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

	encryptionKey := h.Config.StateEncryptionKey
	path := state.ResolvePath(h.Config.StateDir, req.Name)

	// ResolvePath returns a constructed path even when the file doesn't exist
	// (for the "new save" codepath). We must verify the file actually exists
	// before treating it as an exact match; otherwise prefix resolution
	// never triggers.
	if path != "" {
		if _, statErr := os.Stat(path); statErr != nil {
			path = "" // file doesn't exist — fall through to prefix resolution
		}
	}

	// If no exact file found, try prefix-based resolution (most recent match).
	if path == "" {
		matches, err := state.FindByPrefix(h.Config.StateDir, req.Name)
		if err != nil || len(matches) == 0 {
			httpx.Error(w, 404, fmt.Errorf("no state file found for name or prefix %q", req.Name))
			return
		}
		// Matches are sorted newest first; resolve the actual file path.
		resolvedPath := state.ResolvePath(h.Config.StateDir, matches[0].Name)
		// Verify the resolved path actually exists on disk
		if resolvedPath != "" {
			if _, statErr := os.Stat(resolvedPath); statErr != nil {
				resolvedPath = ""
			}
		}
		if resolvedPath == "" {
			// Last-resort fallback: scan the dir directly
			dir := state.SessionsDir(h.Config.StateDir)
			for _, ext := range []string{".json.enc", ".json"} {
				candidate := filepath.Join(dir, matches[0].Name+ext)
				if _, statErr := os.Stat(candidate); statErr == nil {
					resolvedPath = candidate
					break
				}
			}
		}
		if resolvedPath == "" {
			httpx.Error(w, 404, fmt.Errorf("state file for prefix %q found in index but not on disk", req.Name))
			return
		}
		path = resolvedPath
	}

	sf, err := state.Load(path, encryptionKey)
	if err != nil {
		// Retry without encryption key in case the file is not encrypted
		sf, err = state.Load(path, "")
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("load state: %w", err))
			return
		}
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 30*time.Second)
	defer tCancel()

	// Step 1: Restore cookies
	cookiesRestored := 0
	if len(sf.Cookies) > 0 {
		if err := chromedp.Run(tCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			for _, c := range sf.Cookies {
				params := network.SetCookie(c.Name, c.Value).
					WithDomain(c.Domain).
					WithPath(c.Path).
					WithSecure(c.Secure).
					WithHTTPOnly(c.HTTPOnly)

				if c.SameSite != "" {
					var sameSite network.CookieSameSite
					switch c.SameSite {
					case "Strict":
						sameSite = network.CookieSameSiteStrict
					case "Lax":
						sameSite = network.CookieSameSiteLax
					case "None":
						sameSite = network.CookieSameSiteNone
					}
					if sameSite != "" {
						params = params.WithSameSite(sameSite)
					}
				}

				if err := chromedp.Run(tCtx, params); err == nil {
					cookiesRestored++
				}
			}
			return nil
		})); err != nil {
			httpx.Error(w, 500, fmt.Errorf("restore cookies: %w", err))
			return
		}
	}

	// Step 2: Restore storage for each origin
	storageRestored := 0
	for _, originStorage := range sf.Storage {
		// Build JS to restore localStorage
		for k, v := range originStorage.Local {
			keyJSON, _ := json.Marshal(k)
			valueJSON, _ := json.Marshal(v)
			script := fmt.Sprintf(`localStorage.setItem(%s, %s)`, string(keyJSON), string(valueJSON))
			if err := chromedp.Run(tCtx, chromedp.Evaluate(script, nil)); err == nil {
				storageRestored++
			}
		}

		// Build JS to restore sessionStorage
		for k, v := range originStorage.Session {
			keyJSON, _ := json.Marshal(k)
			valueJSON, _ := json.Marshal(v)
			script := fmt.Sprintf(`sessionStorage.setItem(%s, %s)`, string(keyJSON), string(valueJSON))
			if err := chromedp.Run(tCtx, chromedp.Evaluate(script, nil)); err == nil {
				storageRestored++
			}
		}
	}

	h.recordActivity(r, activity.Update{Action: "state.load"})

	slog.Info("state loaded",
		"name", req.Name,
		"path", path,
		"cookiesRestored", cookiesRestored,
		"storageItemsRestored", storageRestored,
		"tabId", resolvedTabID,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, map[string]any{
		"name":                 sf.Name,
		"cookiesRestored":      cookiesRestored,
		"storageItemsRestored": storageRestored,
		"origins":              sf.Origins,
	})
}

// HandleStateDelete removes a saved state file.
// Gated behind CapStateExport: deletion is a destructive operation.
//
// @Endpoint DELETE /state
func (h *Handlers) HandleStateDelete(w http.ResponseWriter, r *http.Request) {
	if !h.stateExportEnabled() {
		httpx.ErrorCode(w, 403, "state_export_disabled", httpx.DisabledEndpointMessage("stateExport", "security.allowStateExport"), false, map[string]any{
			"setting": "security.allowStateExport",
		})
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		httpx.Error(w, 400, fmt.Errorf("name query parameter is required"))
		return
	}

	if err := state.Delete(h.Config.StateDir, name); err != nil {
		httpx.Error(w, 500, fmt.Errorf("delete state: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "state.delete"})

	slog.Info("state deleted",
		"name", name,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, map[string]any{
		"deleted": name,
	})
}

// HandleStateClean removes state files older than a given duration.
// Gated behind CapStateExport: bulk deletion is a destructive operation.
//
// @Endpoint POST /state/clean
func (h *Handlers) HandleStateClean(w http.ResponseWriter, r *http.Request) {
	if !h.stateExportEnabled() {
		httpx.ErrorCode(w, 403, "state_export_disabled", httpx.DisabledEndpointMessage("stateExport", "security.allowStateExport"), false, map[string]any{
			"setting": "security.allowStateExport",
		})
		return
	}
	var req struct {
		OlderThanHours int `json:"olderThanHours"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if req.OlderThanHours <= 0 {
		req.OlderThanHours = 24
	}

	duration := time.Duration(req.OlderThanHours) * time.Hour
	removed, err := state.Clean(h.Config.StateDir, duration)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("clean states: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "state.clean"})

	slog.Info("state clean",
		"olderThanHours", req.OlderThanHours,
		"removed", removed,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, map[string]any{
		"removed":        removed,
		"olderThanHours": req.OlderThanHours,
		"sessionsDir":    filepath.Base(state.SessionsDir(h.Config.StateDir)),
	})
}
