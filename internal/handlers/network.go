package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const (
	defaultWaitRetainedTimeout = 2000 * time.Millisecond
	maxWaitRetainedTimeout     = 30 * time.Second
)

type networkBodyMode string

const (
	networkBodyModeAuto              networkBodyMode = "auto"
	networkBodyModeRetainedPreferred networkBodyMode = "retained-preferred"
	networkBodyModeRetainedOnly      networkBodyMode = "retained-only"
	networkBodyModeLiveOnly          networkBodyMode = "live-only"
)

// parseBufferSize extracts an optional bufferSize query param. Returns 0 if absent.
func parseBufferSize(r *http.Request) int {
	if v := r.URL.Query().Get("bufferSize"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseNetworkBodyMode(r *http.Request) networkBodyMode {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("bodyMode"))) {
	case "", "auto":
		return networkBodyModeAuto
	case "retained-preferred", "retainedpreferred":
		return networkBodyModeRetainedPreferred
	case "retained-only", "retainedonly":
		return networkBodyModeRetainedOnly
	case "live-only", "liveonly":
		return networkBodyModeLiveOnly
	default:
		if parseBoolQuery(r.URL.Query().Get("waitRetained")) {
			return networkBodyModeRetainedPreferred
		}
		return networkBodyModeAuto
	}
}

func parseWaitRetainedTimeout(r *http.Request) time.Duration {
	if v := r.URL.Query().Get("timeoutMs"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			switch {
			case n <= 0:
				return 0
			case n > int(maxWaitRetainedTimeout/time.Millisecond):
				return maxWaitRetainedTimeout
			default:
				return time.Duration(n) * time.Millisecond
			}
		}
	}
	return defaultWaitRetainedTimeout
}

func waitForRetainedBody(buf *bridge.NetworkBuffer, requestID string, timeout time.Duration) (bridge.NetworkEntry, bool) {
	if timeout <= 0 {
		return buf.Get(requestID)
	}
	deadline := time.Now().Add(timeout)
	for {
		entry, ok := buf.Get(requestID)
		if !ok {
			return bridge.NetworkEntry{}, false
		}
		if entry.BodyRetained || !entry.BodyPending || entry.BodyError != "" {
			return entry, true
		}
		if time.Now().After(deadline) {
			return entry, true
		}
		remaining := time.Until(deadline)
		if remaining > 25*time.Millisecond {
			remaining = 25 * time.Millisecond
		}
		time.Sleep(remaining)
	}
}

func populateRetainedBodyResult(result map[string]any, entry bridge.NetworkEntry) {
	if entry.ResponseBody != "" || entry.BodyRetained {
		result["responseBody"] = entry.ResponseBody
	}
	if entry.Base64Encoded {
		result["base64Encoded"] = entry.Base64Encoded
	}
	if entry.BodyRetained {
		result["bodyRetained"] = true
		result["bodySource"] = "retained"
	}
	if entry.BodyPending {
		result["bodyPending"] = true
	}
	if entry.BodySkipped {
		result["bodySkipped"] = true
	}
	if entry.BodySkipReason != "" {
		result["bodySkipReason"] = entry.BodySkipReason
	}
	if entry.BodyTruncated {
		result["bodyTruncated"] = true
	}
	if entry.BodyError != "" {
		result["bodyError"] = entry.BodyError
	}
}

func populateLiveBodyResult(result map[string]any, body string, base64Encoded bool) {
	result["responseBody"] = body
	result["bodySource"] = "live"
	if base64Encoded {
		result["base64Encoded"] = true
	}
}

// HandleNetwork lists recent network entries for a tab.
//
// @Endpoint GET /network
// @Description Returns captured network requests/responses for the active or specified tab
//
// @Param tabId string query Tab ID (optional, uses current tab if empty)
// @Param filter string query URL pattern filter (optional)
// @Param method string query HTTP method filter (optional)
// @Param status string query Status code range filter e.g. "4xx", "5xx", "200" (optional)
// @Param type string query Resource type filter e.g. "xhr", "fetch", "document" (optional)
// @Param limit int query Maximum entries to return (optional)
// @Param bufferSize int query Buffer size for new capture (optional, default from config)
//
// @Response 200 application/json List of network entries
// @Response 404 application/json Tab not found
func (h *Handlers) HandleNetwork(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureBrowser(h.Config); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	tabID := r.URL.Query().Get("tabId")
	tabCtx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, tabCtx, resolvedTabID); !ok {
		return
	}

	nm := h.Bridge.NetworkMonitor()
	if nm == nil {
		httpx.JSON(w, 200, map[string]any{"entries": []any{}, "items": []any{}, "count": 0})
		return
	}

	bufferSize := parseBufferSize(r)

	// Lazily start capture if not already active for this tab
	buf := nm.GetBuffer(resolvedTabID)
	if buf == nil {
		if err := nm.StartCaptureWithSize(tabCtx, resolvedTabID, bufferSize); err != nil {
			httpx.Error(w, 500, fmt.Errorf("start network capture: %w", err))
			return
		}
		buf = nm.GetBuffer(resolvedTabID)
	}

	filter := bridge.NetworkFilter{
		URLPattern:   r.URL.Query().Get("filter"),
		Method:       r.URL.Query().Get("method"),
		StatusRange:  r.URL.Query().Get("status"),
		ResourceType: r.URL.Query().Get("type"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	entries := buf.List(filter)

	if filter.Limit > 0 && len(entries) > filter.Limit {
		entries = entries[len(entries)-filter.Limit:]
	}

	httpx.JSON(w, 200, map[string]any{
		"entries": entries,
		"items":   entries,
		"count":   len(entries),
		"tabId":   resolvedTabID,
	})
}

// HandleNetworkByID returns details for a specific network request.
//
// @Endpoint GET /network/{requestId}
// @Description Returns full details for a specific captured network request
//
// @Param requestId string path Request ID (required)
// @Param tabId string query Tab ID (optional)
// @Param body bool query Include response body (optional, default: false)
//
// @Response 200 application/json Network entry details
// @Response 404 application/json Request not found
func (h *Handlers) HandleNetworkByID(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureBrowser(h.Config); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	requestID := r.PathValue("requestId")
	if requestID == "" {
		httpx.Error(w, 400, fmt.Errorf("requestId required"))
		return
	}

	tabID := r.URL.Query().Get("tabId")
	tabCtx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, tabCtx, resolvedTabID); !ok {
		return
	}

	nm := h.Bridge.NetworkMonitor()
	if nm == nil {
		httpx.Error(w, 404, fmt.Errorf("network monitoring not active"))
		return
	}

	buf := nm.GetBuffer(resolvedTabID)
	if buf == nil {
		httpx.Error(w, 404, fmt.Errorf("no network data for tab %s", resolvedTabID))
		return
	}

	entry, ok := buf.Get(requestID)
	if !ok {
		httpx.Error(w, 404, fmt.Errorf("request %s not found", requestID))
		return
	}

	result := map[string]any{
		"entry": entry,
		"tabId": resolvedTabID,
	}

	// Optionally include response body
	if r.URL.Query().Get("body") == "true" && entry.Finished && !entry.Failed {
		bodyMode := parseNetworkBodyMode(r)
		if bodyMode == networkBodyModeRetainedPreferred && entry.BodyPending {
			entry, ok = waitForRetainedBody(buf, requestID, parseWaitRetainedTimeout(r))
			if !ok {
				httpx.Error(w, 404, fmt.Errorf("request %s not found", requestID))
				return
			}
			result["entry"] = entry
		}
		switch {
		case bodyMode == networkBodyModeLiveOnly:
			body, base64Encoded, err := bridge.GetResponseBodyDirect(tabCtx, requestID)
			if err != nil {
				result["bodyError"] = err.Error()
			} else {
				populateLiveBodyResult(result, body, base64Encoded)
			}
		case entry.BodyRetained:
			populateRetainedBodyResult(result, entry)
		case bodyMode == networkBodyModeRetainedOnly:
			populateRetainedBodyResult(result, entry)
		case entry.BodyPending || entry.BodyError != "":
			populateRetainedBodyResult(result, entry)
		default:
			body, base64Encoded, err := bridge.GetResponseBodyDirect(tabCtx, requestID)
			if err != nil {
				result["bodyError"] = err.Error()
			} else {
				populateLiveBodyResult(result, body, base64Encoded)
			}
		}
		populateRetainedBodyResult(result, entry)
	}

	httpx.JSON(w, 200, result)
}

// HandleNetworkClear clears captured network data.
//
// @Endpoint POST /network/clear
// @Description Clears all captured network data for a tab or all tabs
//
// @Param tabId string query Tab ID (optional, clears all if empty)
//
// @Response 200 application/json Success
func (h *Handlers) HandleNetworkClear(w http.ResponseWriter, r *http.Request) {
	nm := h.Bridge.NetworkMonitor()
	if nm == nil {
		httpx.JSON(w, 200, map[string]any{"cleared": true})
		return
	}

	tabID := r.URL.Query().Get("tabId")
	if tabID != "" {
		_, resolvedTabID, err := h.tabContext(r, tabID)
		if err != nil {
			WriteTabContextError(w, err, 404)
			return
		}
		nm.ClearTab(resolvedTabID)
		httpx.JSON(w, 200, map[string]any{"cleared": true, "tabId": resolvedTabID})
	} else {
		nm.ClearAll()
		httpx.JSON(w, 200, map[string]any{"cleared": true, "all": true})
	}
}

// HandleNetworkStream streams network entries via Server-Sent Events.
//
// @Endpoint GET /network/stream
// @Description Streams network entries in real-time as they are captured
//
// @Param tabId string query Tab ID (optional, uses current tab if empty)
// @Param filter string query URL pattern filter (optional)
// @Param method string query HTTP method filter (optional)
// @Param status string query Status code range filter (optional)
// @Param type string query Resource type filter (optional)
// @Param bufferSize int query Buffer size for new capture (optional)
//
// @Response 200 text/event-stream SSE stream of network entries
func (h *Handlers) HandleNetworkStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Problem(w, http.StatusInternalServerError, "streaming_not_supported", "streaming not supported", false, nil)
		return
	}

	// Clear write deadline for long-lived SSE connections; ignore errors
	// (e.g. httptest.ResponseRecorder doesn't support this).
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

	if err := h.ensureBrowser(h.Config); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	tabID := r.URL.Query().Get("tabId")
	tabCtx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, tabCtx, resolvedTabID); !ok {
		return
	}

	nm := h.Bridge.NetworkMonitor()
	if nm == nil {
		httpx.Error(w, 500, fmt.Errorf("network monitoring not available"))
		return
	}

	bufferSize := parseBufferSize(r)

	// Ensure capture is active
	buf := nm.GetBuffer(resolvedTabID)
	if buf == nil {
		if err := nm.StartCaptureWithSize(tabCtx, resolvedTabID, bufferSize); err != nil {
			httpx.Error(w, 500, fmt.Errorf("start network capture: %w", err))
			return
		}
		buf = nm.GetBuffer(resolvedTabID)
	}

	filter := bridge.NetworkFilter{
		URLPattern:   r.URL.Query().Get("filter"),
		Method:       r.URL.Query().Get("method"),
		StatusRange:  r.URL.Query().Get("status"),
		ResourceType: r.URL.Query().Get("type"),
	}

	subID, ch := buf.Subscribe()
	defer buf.Unsubscribe(subID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case entry, ok := <-ch:
			if !ok {
				return
			}
			if !filter.Match(entry) {
				continue
			}
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: network\ndata: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()

		case <-keepalive.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

// HandleTabNetwork lists network entries for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/network
func (h *Handlers) HandleTabNetwork(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}
	q := r.URL.Query()
	q.Set("tabId", tabID)
	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u
	h.HandleNetwork(w, req)
}

// HandleTabNetworkByID returns details for a specific request in a tab.
//
// @Endpoint GET /tabs/{id}/network/{requestId}
func (h *Handlers) HandleTabNetworkByID(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	requestID := r.PathValue("requestId")
	if tabID == "" || requestID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id and request id required"))
		return
	}
	q := r.URL.Query()
	q.Set("tabId", tabID)
	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u
	// Set the requestId path value by creating a new request with the path
	h.HandleNetworkByID(w, req)
}

// HandleTabNetworkStream streams network entries for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/network/stream
func (h *Handlers) HandleTabNetworkStream(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}
	q := r.URL.Query()
	q.Set("tabId", tabID)
	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u
	h.HandleNetworkStream(w, req)
}
