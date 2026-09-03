package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/state"
)

func (h *Handlers) stateExportEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowStateExport
}

// HandleStateList lists all saved state files.
func (h *Handlers) HandleStateList(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
		return
	}

	entries, err := state.List(h.Config.StateDir)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("list states: %w", err))
		return
	}
	httpx.JSON(w, 200, map[string]any{
		"states": entries,
		"count":  len(entries),
	})
}

// HandleStateShow returns the full contents of a saved state file.
func (h *Handlers) HandleStateShow(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		httpx.Error(w, 400, fmt.Errorf("name query parameter is required"))
		return
	}

	encryptionKey := h.stateEncryptionKey()
	path := state.ResolvePath(h.Config.StateDir, name)
	sf, err := state.Load(path, encryptionKey)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("load state: %w", err))
		return
	}
	httpx.JSON(w, 200, h.stateFileInScope(sf))
}

// stateEncryptionKey resolves the key the state files are encrypted with. The
// environment variable wins so an operator can override per process, and
// security.stateEncryptionKey answers otherwise — it is the documented knob,
// it is redacted by the dashboard config API, and it was reachable by
// `config set` while nothing read it, so setting it stored a secret that did
// nothing and the request still refused for want of a key.
func (h *Handlers) stateEncryptionKey() string {
	if key := strings.TrimSpace(os.Getenv("PINCHTAB_STATE_KEY")); key != "" {
		return key
	}
	if h == nil || h.Config == nil {
		return ""
	}
	return strings.TrimSpace(h.Config.StateEncryptionKey)
}

type stateSaveRequest struct {
	Name     string                 `json:"name"`
	Encrypt  bool                   `json:"encrypt"`
	TabID    string                 `json:"tabId"`
	Metadata map[string]interface{} `json:"metadata"`
}

// HandleStateSave captures the current browser state and writes it to disk.
func (h *Handlers) HandleStateSave(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
		return
	}

	var req stateSaveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	encryptionKey := ""
	if req.Encrypt {
		encryptionKey = h.stateEncryptionKey()
		if err := state.ValidateEncryptionKey(encryptionKey); err != nil {
			httpx.Error(w, 400, fmt.Errorf("encryption key required: set security.stateEncryptionKey or the PINCHTAB_STATE_KEY environment variable"))
			return
		}
	}

	ctx, resolvedTabID, ok := h.guardedTabContext(w, r, req.TabID, guardDomainPolicy)
	if !ok {
		return
	}

	captured, err := h.captureBrowserState(ctx, resolvedTabID, req.Metadata)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("capture state: %w", err))
		return
	}
	sf := captured.file
	sf.Name = req.Name

	path, err := state.Save(h.Config.StateDir, sf, encryptionKey)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("save state: %w", err))
		return
	}

	slog.Info("state saved",
		"name", sf.Name,
		"path", path,
		"cookies", len(sf.Cookies),
		"origin", firstOrigin(sf.Origins),
		"encrypted", req.Encrypt,
		"tabId", resolvedTabID,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, map[string]any{
		"name":      sf.Name,
		"path":      path,
		"cookies":   len(sf.Cookies),
		"origins":   sf.Origins,
		"encrypted": req.Encrypt,
	})
}

// HandleStateDelete removes a saved state file.
func (h *Handlers) HandleStateDelete(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
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
	slog.Info("state deleted", "name", name, "remoteAddr", r.RemoteAddr)
	httpx.JSON(w, 200, map[string]any{"deleted": name})
}

// HandleStateClean removes state files older than a given duration.
func (h *Handlers) HandleStateClean(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
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
	slog.Info("state clean", "olderThanHours", req.OlderThanHours, "removed", removed, "remoteAddr", r.RemoteAddr)
	httpx.JSON(w, 200, map[string]any{
		"removed":        removed,
		"olderThanHours": req.OlderThanHours,
		"sessionsDir":    filepath.Base(state.SessionsDir(h.Config.StateDir)),
	})
}
