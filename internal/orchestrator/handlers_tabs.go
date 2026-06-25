package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

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
	resp, body, err := o.doInstanceRequest(r.Context(), proxyReq.Method, proxyReq.Header, payload, targetURL, inst)
	if err != nil {
		httpx.Error(w, 502, err)
		return
	}
	if shouldRetryInstanceTabOpen(resp.StatusCode, body) {
		ensureURL, ensureErr := o.instancePathURL(inst, "/ensure-browser", "")
		if ensureErr == nil {
			if err := o.ensureInstanceChrome(inst, ensureURL); err == nil {
				if retryResp, retryBody, retryErr := o.doInstanceRequest(r.Context(), proxyReq.Method, proxyReq.Header, payload, targetURL, inst); retryErr == nil {
					resp = retryResp
					body = retryBody
				}
			}
		}
	}
	o.handleProxyResponseHeaders(r, resp, inst.ID)
	enrichActivityFromResponse(r, body)
	copyProxyResponse(w, resp, body)
}

func (o *Orchestrator) ensureInstanceChrome(inst *InstanceInternal, targetURL *url.URL) error {
	if inst == nil || targetURL == nil {
		return fmt.Errorf("instance ensure-browser target missing")
	}
	req, err := http.NewRequest(http.MethodPost, targetURL.String(), nil)
	if err != nil {
		return err
	}
	tagOrchestratorMonitoringRequest(req)
	o.applyInstanceAuth(req, inst)
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ensure-browser HTTP %d: %s", resp.StatusCode, compactBody(body))
	}
	return nil
}

func (o *Orchestrator) doInstanceRequest(ctx context.Context, method string, header http.Header, body []byte, targetURL *url.URL, inst *InstanceInternal) (*http.Response, []byte, error) {
	proxyReq, err := http.NewRequestWithContext(ctx, method, targetURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	proxyReq.ContentLength = int64(len(body))
	proxyReq.Header = header.Clone()
	tagOrchestratorMonitoringRequest(proxyReq)
	o.applyInstanceAuth(proxyReq, inst)

	resp, err := o.client.Do(proxyReq)
	if err != nil {
		return nil, nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, nil, readErr
	}
	return resp, body, nil
}

func copyProxyResponse(w http.ResponseWriter, resp *http.Response, body []byte) {
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func shouldRetryInstanceTabOpen(statusCode int, body []byte) bool {
	if statusCode != http.StatusInternalServerError {
		return false
	}
	msg := strings.ToLower(string(body))
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "context cancelled")
}
