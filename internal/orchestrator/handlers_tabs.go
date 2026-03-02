package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/pinchtab/pinchtab/internal/web"
)

// handleTabClose closes a tab by finding its instance and sending a close request.
// This has custom logic (constructs a different request body) so it's not genericized.
func (o *Orchestrator) handleTabClose(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	inst, err := o.findRunningInstanceByTabID(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	// Construct request body for the bridge's /tab endpoint
	reqBody, _ := json.Marshal(map[string]string{
		"action": "close",
		"tabId":  tabID,
	})

	targetURL := fmt.Sprintf("http://localhost:%s/tab", inst.Port)
	proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		web.Error(w, 500, err)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		web.Error(w, 502, fmt.Errorf("instance unreachable: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// handleInstanceTabOpen opens a new tab in a specific instance.
// This has custom logic so it's not genericized.
func (o *Orchestrator) handleInstanceTabOpen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()
	if !ok {
		web.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}
	if inst.Status != "running" {
		web.Error(w, 503, fmt.Errorf("instance %q is not running (status: %s)", id, inst.Status))
		return
	}

	var req struct {
		URL string `json:"url,omitempty"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			web.Error(w, 400, fmt.Errorf("invalid JSON"))
			return
		}
	}

	payload, err := json.Marshal(map[string]any{
		"action": "new",
		"url":    req.URL,
	})
	if err != nil {
		web.Error(w, 500, fmt.Errorf("failed to build tab open request: %w", err))
		return
	}

	proxyReq := r.Clone(r.Context())
	proxyReq.Body = io.NopCloser(bytes.NewReader(payload))
	proxyReq.ContentLength = int64(len(payload))
	proxyReq.Header = r.Header.Clone()
	proxyReq.Header.Set("Content-Type", "application/json")

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     "/tab",
		RawQuery: r.URL.RawQuery,
	}
	o.proxyToURL(w, proxyReq, targetURL)
}
