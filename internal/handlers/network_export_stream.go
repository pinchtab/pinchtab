package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// HandleNetworkExportStream streams network entries to a file as they arrive.
//
// @Endpoint GET /network/export/stream
// @Description Live capture: write entries to file as they are captured
//
// @Param tabId   string query Tab ID
// @Param format  string query Export format (default: har)
// @Param path    string query Output filename (required)
// @Param body    string query "true" to include response bodies
// @Param redact  string query "false" to include sensitive headers (default: redacted)
// @Param filter... (same as HandleNetworkExport)
//
// @Response 200 text/event-stream  SSE progress events
// @Response 423 application/json   Tab is locked
func (h *Handlers) HandleNetworkExportStream(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	userPath := r.URL.Query().Get("path")
	if userPath == "" {
		httpx.Error(w, 400, fmt.Errorf("path required for streaming export"))
		return
	}

	tabID := r.URL.Query().Get("tabId")
	tabCtx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, tabCtx, resolvedTabID); !ok {
		return
	}

	owner := resolveOwner(r, "")
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		httpx.ErrorCode(w, http.StatusLocked, "tab_locked", err.Error(), false, nil)
		return
	}

	h.recordReadRequest(r, "network-export-stream", resolvedTabID)

	formatName := r.URL.Query().Get("format")
	if formatName == "" {
		formatName = "har"
	}
	factory := observe.GetFormat(formatName)
	if factory == nil {
		httpx.JSON(w, 400, map[string]any{
			"code":      "unknown_format",
			"error":     fmt.Sprintf("unknown format %q", formatName),
			"available": observe.ListFormats(),
		})
		return
	}

	nm := h.Bridge.NetworkMonitor()
	if nm == nil {
		httpx.Error(w, 500, fmt.Errorf("network monitor not available"))
		return
	}

	bufferSize := parseBufferSize(r)
	buf := nm.GetBuffer(resolvedTabID)
	if buf == nil {
		if err := nm.StartCaptureWithSize(tabCtx, resolvedTabID, bufferSize); err != nil {
			httpx.Error(w, 500, fmt.Errorf("start capture: %w", err))
			return
		}
		buf = nm.GetBuffer(resolvedTabID)
	}

	includeBody := r.URL.Query().Get("body") == "true"
	redactHeaders := r.URL.Query().Get("redact") != "false"
	filter := parseNetworkFilter(r)

	// Path safety (#1)
	exportDir := filepath.Join(h.Config.StateDir, "exports")
	if err := os.MkdirAll(exportDir, 0750); err != nil {
		httpx.Error(w, 500, fmt.Errorf("create dir: %w", err))
		return
	}

	safeName := filepath.Base(userPath)
	safePath, err := httpx.SafeCreatePath(exportDir, safeName)
	if err != nil {
		httpx.Error(w, 400, fmt.Errorf("invalid path: %w", err))
		return
	}
	absBase, _ := filepath.Abs(exportDir)
	absPath, err := filepath.Abs(safePath)
	if err != nil || !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
		httpx.Error(w, 400, fmt.Errorf("path escapes export directory"))
		return
	}

	// Write to temp file, rename on finish (#8)
	tmpPath := absPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("create file: %w", err))
		return
	}

	enc := factory("PinchTab", h.version())
	if err := enc.Start(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		httpx.Error(w, 500, fmt.Errorf("start encoder: %w", err))
		return
	}

	// Semaphore to throttle concurrent body fetches in streaming mode.
	streamBodySem := make(chan struct{}, bodyFetchConcurrency)

	subID, ch := buf.Subscribe()
	defer buf.Unsubscribe(subID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = enc.Finish()
		_ = f.Close()
		_ = os.Remove(tmpPath)
		httpx.Error(w, 500, fmt.Errorf("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Set a hard deadline so forgotten streams don't leak resources.
	streamDeadline := time.NewTimer(maxExportStreamDuration)
	defer streamDeadline.Stop()

	// Clear per-write deadline for long-lived SSE (#7), the stream timer above caps total duration.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

	count := 0
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	var finalizeOnce sync.Once
	finalize := func() {
		finalizeOnce.Do(func() {
			_ = enc.Finish()
			if err := f.Close(); err == nil {
				// Atomic rename on success (#8)
				if count > 0 {
					_ = os.Rename(tmpPath, absPath)
				} else {
					_ = os.Remove(tmpPath)
				}
			} else {
				_ = os.Remove(tmpPath)
			}
		})
	}

	// sendDone emits the final SSE event with the correct path (empty when no file was written).
	sendDone := func() {
		result := map[string]any{"entries": count}
		if count > 0 {
			result["path"] = absPath
		}
		data, _ := json.Marshal(result)
		_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
		flusher.Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			finalize()
			// Don't write to ResponseWriter after client disconnect (#6)
			return

		case <-streamDeadline.C:
			finalize()
			data, _ := json.Marshal(map[string]any{"entries": count, "reason": "max_duration_reached"})
			_, _ = fmt.Fprintf(w, "event: timeout\ndata: %s\n\n", data)
			sendDone()
			return

		case entry, ok := <-ch:
			if !ok {
				finalize()
				sendDone()
				return
			}

			// The subscriber fires on requestWillBeSent (entry created) before
			// responseReceived/loadingFinished populate status, headers, timing.
			// Wait for the entry to be Finished before exporting it.
			if !entry.Finished && !entry.Failed {
				reqID := entry.RequestID
				waitDeadline := time.After(10 * time.Second)
				poll := time.NewTicker(200 * time.Millisecond)
			waitDone:
				for {
					select {
					case <-r.Context().Done():
						poll.Stop()
						finalize()
						return
					case <-waitDeadline:
						poll.Stop()
						if updated, found := buf.Get(reqID); found {
							entry = updated
						}
						break waitDone
					case <-poll.C:
						if updated, found := buf.Get(reqID); found && (updated.Finished || updated.Failed) {
							entry = updated
							poll.Stop()
							break waitDone
						}
					}
				}
			}

			if !filter.Match(entry) {
				continue
			}
			var body string
			var b64 bool
			if includeBody && entry.Finished && !entry.Failed {
				// Throttle body fetches to avoid saturating the CDP connection.
				streamBodySem <- struct{}{}
				body, b64, _ = nm.GetResponseBody(tabCtx, entry.RequestID)
				<-streamBodySem
				if len(body) > maxExportBodyBytes {
					body = ""
					b64 = false
				}
			}
			export := observe.NetworkEntryToExport(entry, body, b64)
			if redactHeaders {
				export.Request.Headers = observe.RedactSensitiveHeaders(export.Request.Headers)
				export.Response.Headers = observe.RedactSensitiveHeaders(export.Response.Headers)
			}
			if err := enc.Encode(export); err != nil {
				finalize()
				return
			}
			count++
			data, _ := json.Marshal(map[string]any{"entries": count, "url": safetruncateURL(entry.URL)})
			_, _ = fmt.Fprintf(w, "event: export\ndata: %s\n\n", data)
			flusher.Flush()

		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// HandleTabNetworkExportStream handles GET /tabs/{id}/network/export/stream.
func (h *Handlers) HandleTabNetworkExportStream(w http.ResponseWriter, r *http.Request) {
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
	h.HandleNetworkExportStream(w, req)
}

// safetruncateURL truncates at a valid UTF-8 boundary (#21).
func safetruncateURL(u string) string {
	const maxLen = 120
	if len(u) <= maxLen {
		return u
	}
	for i := maxLen; i > 0; i-- {
		if utf8.RuneStart(u[i]) {
			return u[:i]
		}
	}
	return u[:maxLen]
}
