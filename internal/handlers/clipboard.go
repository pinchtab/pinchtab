package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type clipboardRequest struct {
	TabID string  `json:"tabId"`
	Text  *string `json:"text"`
}

const maxClipboardTextBytes = 64 << 10

// clipboardStore is a simple in-memory clipboard shared across all requests.
// In headless Chrome, navigator.clipboard and execCommand are unreliable,
// so we maintain clipboard state server-side.
type clipboardStore struct {
	mu   sync.RWMutex
	text string
}

func (s *clipboardStore) Read() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.text
}

func (s *clipboardStore) Write(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text = text
}

func (h *Handlers) clipboardEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowClipboard
}

// HandleClipboardRead reads text from the clipboard.
func (h *Handlers) HandleClipboardRead(w http.ResponseWriter, r *http.Request) {
	if !h.clipboardEnabled() {
		httpx.ErrorCode(w, 403, "clipboard_disabled", httpx.DisabledEndpointMessage("clipboard", "security.allowClipboard"), false, map[string]any{
			"setting": "security.allowClipboard",
		})
		return
	}

	text := h.clipboard.Read()

	slog.Info("clipboard: read",
		"textLen", len(text),
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, http.StatusOK, map[string]any{
		"text": text,
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
	if len(*req.Text) > maxClipboardTextBytes {
		httpx.Error(w, http.StatusBadRequest, fmt.Errorf("text too large (max %d bytes)", maxClipboardTextBytes))
		return
	}

	h.clipboard.Write(*req.Text)

	slog.Info("clipboard: write",
		"textLen", len(*req.Text),
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, http.StatusOK, map[string]any{
		"success": true,
	})
}

// HandleClipboardCopy is an alias for HandleClipboardWrite.
func (h *Handlers) HandleClipboardCopy(w http.ResponseWriter, r *http.Request) {
	h.HandleClipboardWrite(w, r)
}

// HandleClipboardPaste reads from clipboard (paste = read clipboard content).
func (h *Handlers) HandleClipboardPaste(w http.ResponseWriter, r *http.Request) {
	h.HandleClipboardRead(w, r)
}
