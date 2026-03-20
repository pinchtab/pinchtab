package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type clipboardRequest struct {
	TabID string  `json:"tabId"`
	Text  *string `json:"text"`
}

// clipboardStore is a simple in-memory clipboard shared across all tabs.
// In headless Chrome, navigator.clipboard and execCommand are unreliable,
// so we maintain clipboard state server-side.
var (
	clipboardText string
	clipboardMu   sync.RWMutex
)

func (h *Handlers) clipboardEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowClipboard
}

// resolveClipboardTab resolves the tab context for clipboard operations.
func (h *Handlers) resolveClipboardTab(r *http.Request, bodyTabID string) (string, error) {
	tabID := strings.TrimSpace(r.URL.Query().Get("tabId"))
	if tabID == "" {
		tabID = strings.TrimSpace(bodyTabID)
	}
	_, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		return "", err
	}
	return resolvedID, nil
}

// HandleClipboardRead reads text from the clipboard.
func (h *Handlers) HandleClipboardRead(w http.ResponseWriter, r *http.Request) {
	if !h.clipboardEnabled() {
		httpx.ErrorCode(w, 403, "clipboard_disabled", httpx.DisabledEndpointMessage("clipboard", "security.allowClipboard"), false, map[string]any{
			"setting": "security.allowClipboard",
		})
		return
	}

	resolvedID, err := h.resolveClipboardTab(r, "")
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err)
		return
	}

	clipboardMu.RLock()
	text := clipboardText
	clipboardMu.RUnlock()

	slog.Info("clipboard: read",
		"tabId", resolvedID,
		"textLen", len(text),
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, http.StatusOK, map[string]any{
		"tabId": resolvedID,
		"text":  text,
	})
}

// HandleClipboardWrite writes text to the clipboard.
func (h *Handlers) HandleClipboardWrite(w http.ResponseWriter, r *http.Request) {
	if !h.clipboardEnabled() {
		httpx.ErrorCode(w, 403, "clipboard_disabled", httpx.DisabledEndpointMessage("clipboard", "security.allowClipboard"), false, map[string]any{
			"setting": "security.allowClipboard",
		})
		return
	}

	var req clipboardRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, fmt.Errorf("decode: %w", err))
		return
	}
	if req.Text == nil {
		httpx.Error(w, http.StatusBadRequest, fmt.Errorf("text required"))
		return
	}

	resolvedID, err := h.resolveClipboardTab(r, req.TabID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err)
		return
	}

	clipboardMu.Lock()
	clipboardText = *req.Text
	clipboardMu.Unlock()

	slog.Info("clipboard: write",
		"tabId", resolvedID,
		"textLen", len(*req.Text),
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"tabId":   resolvedID,
	})
}

// HandleClipboardCopy is an alias for HandleClipboardWrite.
func (h *Handlers) HandleClipboardCopy(w http.ResponseWriter, r *http.Request) {
	h.HandleClipboardWrite(w, r)
}

// HandleClipboardPaste reads from clipboard (paste = read clipboard content).
func (h *Handlers) HandleClipboardPaste(w http.ResponseWriter, r *http.Request) {
	if !h.clipboardEnabled() {
		httpx.ErrorCode(w, 403, "clipboard_disabled", httpx.DisabledEndpointMessage("clipboard", "security.allowClipboard"), false, map[string]any{
			"setting": "security.allowClipboard",
		})
		return
	}

	// Allow optional body with tabId
	var req clipboardRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req)

	resolvedID, err := h.resolveClipboardTab(r, req.TabID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err)
		return
	}

	clipboardMu.RLock()
	text := clipboardText
	clipboardMu.RUnlock()

	slog.Info("clipboard: read",
		"tabId", resolvedID,
		"textLen", len(text),
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, http.StatusOK, map[string]any{
		"tabId": resolvedID,
		"text":  text,
	})
}
