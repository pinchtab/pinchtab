package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

// handleInstanceTabOpen opens a new tab in a specific instance.
// This has custom logic so it's not genericized.
func (o *Orchestrator) handleInstanceTabOpen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()
	if !ok {
		httpx.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}
	if inst.Status != "running" {
		httpx.Error(w, 503, fmt.Errorf("instance %q is not running (status: %s)", id, inst.Status))
		return
	}

	var req struct {
		URL string `json:"url,omitempty"`
	}
	if r.ContentLength > 0 {
		if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
			httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("invalid JSON"))
			return
		}
	}

	payload, err := json.Marshal(map[string]any{
		"action": "new",
		"url":    req.URL,
	})
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("failed to build tab open request: %w", err))
		return
	}

	proxyReq := r.Clone(r.Context())
	proxyReq.Body = io.NopCloser(bytes.NewReader(payload))
	proxyReq.ContentLength = int64(len(payload))
	proxyReq.Header = r.Header.Clone()
	proxyReq.Header.Set("Content-Type", "application/json")

	targetURL, err := o.instancePathURL(inst, "/tab", r.URL.RawQuery)
	if err != nil {
		httpx.Error(w, 502, err)
		return
	}
	o.proxyToURL(w, proxyReq, targetURL)
}
