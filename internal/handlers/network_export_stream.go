package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/pinchtab/pinchtab/internal/bridge"
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
	if !h.ensureBrowserReady(w) {
		return
	}

	userPath := r.URL.Query().Get("path")
	if userPath == "" {
		httpx.Error(w, 400, fmt.Errorf("path required for streaming export"))
		return
	}

	ec, ok := h.resolveExportContext(w, r, "network-export-stream")
	if !ok {
		return
	}

	nm := h.Bridge.NetworkMonitor()
	if nm == nil {
		httpx.Error(w, 500, fmt.Errorf("network monitor not available"))
		return
	}

	buf, ok := h.ensureCaptureBuffer(w, r, nm, ec.tabCtx, ec.resolvedTabID)
	if !ok {
		return
	}

	session, ok := h.startExportStream(w, r, ec, nm, buf, userPath)
	if !ok {
		return
	}
	session.run(r)
}

// exportStreamSession holds the per-connection state for a streaming export: the
// SSE writer, the open temp file + encoder, the buffer subscriptions, and the
// running entry count. Its methods split the controller into setup (startExportStream),
// per-entry encode/emit (exportEntry), and finalization (finalize/sendDone).
type exportStreamSession struct {
	w             http.ResponseWriter
	flusher       http.Flusher
	enc           observe.ExportEncoder
	f             *os.File
	nm            *bridge.NetworkMonitor
	tabCtx        context.Context
	buf           *bridge.NetworkBuffer
	filter        bridge.NetworkFilter
	includeBody   bool
	redactHeaders bool
	absPath       string
	tmpPath       string
	bodySem       chan struct{}

	subID           int
	entryCh         <-chan bridge.NetworkEntry
	completionSubID int
	completions     <-chan string

	count        int
	finalizeOnce sync.Once
}

// pendingExport stashes an entry that arrived before its response finished, so the
// loop can drain it on completion or at its deadline instead of head-of-line blocking.
type pendingExport struct {
	entry    bridge.NetworkEntry
	deadline time.Time
}

// startExportStream opens the export target file + encoder, subscribes to the
// buffer, and writes the SSE headers — returning a session ready to run. On any
// setup failure it writes the response (cleaning up the temp file) and returns
// ok=false. The browser-ready, path-required, prelude, and capture-bootstrap checks
// run in the caller.
func (h *Handlers) startExportStream(w http.ResponseWriter, r *http.Request, ec exportContext, nm *bridge.NetworkMonitor, buf *bridge.NetworkBuffer, userPath string) (*exportStreamSession, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, 500, fmt.Errorf("streaming not supported"))
		return nil, false
	}

	absPath, tmpPath, f, status, err := h.resolveExportFile(userPath)
	if err != nil {
		httpx.Error(w, status, err)
		return nil, false
	}

	enc := ec.factory("PinchTab", h.version())
	if err := enc.Start(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		httpx.Error(w, 500, fmt.Errorf("start encoder: %w", err))
		return nil, false
	}

	subID, ch := buf.Subscribe()
	completionSubID, completions := buf.SubscribeCompletions()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Clear the per-write deadline for long-lived SSE; the stream timer in run()
	// caps total duration instead.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

	return &exportStreamSession{
		w:               w,
		flusher:         flusher,
		enc:             enc,
		f:               f,
		nm:              nm,
		tabCtx:          ec.tabCtx,
		buf:             buf,
		filter:          parseNetworkFilter(r),
		includeBody:     r.URL.Query().Get("body") == "true",
		redactHeaders:   r.URL.Query().Get("redact") != "false",
		absPath:         absPath,
		tmpPath:         tmpPath,
		bodySem:         make(chan struct{}, bodyFetchConcurrency),
		subID:           subID,
		entryCh:         ch,
		completionSubID: completionSubID,
		completions:     completions,
	}, true
}

// run drives the SSE event loop until the client disconnects, the stream channel
// closes, or the max-duration deadline fires, finalizing the file on exit.
func (s *exportStreamSession) run(r *http.Request) {
	defer s.buf.Unsubscribe(s.subID)
	defer s.buf.UnsubscribeCompletions(s.completionSubID)

	// Hard deadline so forgotten streams don't leak resources.
	streamDeadline := time.NewTimer(maxExportStreamDuration)
	defer streamDeadline.Stop()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	// Entries arrive on requestWillBeSent before status/headers/timing are
	// populated, so incomplete ones are stashed and drained as they finish instead
	// of blocking the loop on a single slow request (head-of-line blocking).
	pending := map[string]pendingExport{}
	pendingTicker := time.NewTicker(200 * time.Millisecond)
	defer pendingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			s.finalize()
			// Don't write to the ResponseWriter after client disconnect.
			return

		case <-streamDeadline.C:
			if s.drainPending(pending) {
				return
			}
			s.finalize()
			data, _ := json.Marshal(map[string]any{"entries": s.count, "reason": "max_duration_reached"})
			_, _ = fmt.Fprintf(s.w, "event: timeout\ndata: %s\n\n", data)
			s.sendDone()
			return

		case entry, ok := <-s.entryCh:
			if !ok {
				if !s.drainPending(pending) {
					s.finalize()
					s.sendDone()
				}
				return
			}

			// The subscriber fires on requestWillBeSent (entry created) before
			// responseReceived/loadingFinished populate status, headers, timing.
			// Completed entries export immediately; incomplete ones are stashed and
			// exported when their completion notification arrives (or at the deadline).
			if entry.Finished || entry.Failed {
				if s.exportEntry(entry) {
					return
				}
			} else {
				pending[entry.RequestID] = pendingExport{entry: entry, deadline: time.Now().Add(entryFinishWait)}
			}

		case reqID := <-s.completions:
			// A request finished/failed: export the stashed entry now (with its
			// latest buffered state) instead of waiting for a poll tick.
			if p, ok := pending[reqID]; ok {
				delete(pending, reqID)
				entry := p.entry
				if updated, found := s.buf.Get(reqID); found {
					entry = updated
				}
				if s.exportEntry(entry) {
					return
				}
			}

		case <-pendingTicker.C:
			// Deadline-only safety sweep: completions are delivered via the channel
			// above, so this just flushes pending entries that never sent a
			// completion (or whose notification was dropped under burst).
			now := time.Now()
			for reqID, p := range pending {
				if !now.After(p.deadline) {
					continue
				}
				entry := p.entry
				if updated, found := s.buf.Get(reqID); found {
					entry = updated
				}
				delete(pending, reqID)
				if s.exportEntry(entry) {
					return
				}
			}

		case <-keepalive.C:
			_, _ = fmt.Fprintf(s.w, ": keepalive\n\n")
			s.flusher.Flush()
		}
	}
}

// exportEntry encodes one finished/failed entry and emits its SSE frame. It returns
// true when encoding fails and the run loop should stop (the file is finalized).
func (s *exportStreamSession) exportEntry(entry bridge.NetworkEntry) (stop bool) {
	if !s.filter.Match(entry) {
		return false
	}
	var body string
	var b64 bool
	if s.includeBody && entry.Finished && !entry.Failed {
		// Throttle body fetches to avoid saturating the CDP connection.
		s.bodySem <- struct{}{}
		body, b64 = resolveExportBody(s.tabCtx, s.nm, entry)
		<-s.bodySem
	}
	export := toExportEntry(entry, body, b64, s.redactHeaders)
	if err := s.enc.Encode(export); err != nil {
		s.finalize()
		return true
	}
	s.count++
	data, _ := json.Marshal(map[string]any{"entries": s.count, "url": safetruncateURL(entry.URL)})
	_, _ = fmt.Fprintf(s.w, "event: export\ndata: %s\n\n", data)
	s.flusher.Flush()
	return false
}

// drainPending flushes any still-pending entries best-effort on graceful shutdown,
// preferring the latest buffered copy. It returns true if encoding stopped the loop.
func (s *exportStreamSession) drainPending(pending map[string]pendingExport) (stop bool) {
	for reqID, p := range pending {
		entry := p.entry
		if updated, found := s.buf.Get(reqID); found {
			entry = updated
		}
		delete(pending, reqID)
		if s.exportEntry(entry) {
			return true
		}
	}
	return false
}

// finalize finishes the encoder and closes the file exactly once, renaming the temp
// file into place when at least one entry was written and removing it otherwise.
func (s *exportStreamSession) finalize() {
	s.finalizeOnce.Do(func() {
		_ = s.enc.Finish()
		if err := s.f.Close(); err == nil {
			if s.count > 0 {
				_ = os.Rename(s.tmpPath, s.absPath)
			} else {
				_ = os.Remove(s.tmpPath)
			}
		} else {
			_ = os.Remove(s.tmpPath)
		}
	})
}

// sendDone emits the final SSE event with the resulting path (empty when no file
// was written).
func (s *exportStreamSession) sendDone() {
	result := map[string]any{"entries": s.count}
	if s.count > 0 {
		result["path"] = s.absPath
	}
	data, _ := json.Marshal(result)
	_, _ = fmt.Fprintf(s.w, "event: done\ndata: %s\n\n", data)
	s.flusher.Flush()
}

// HandleTabNetworkExportStream handles GET /tabs/{id}/network/export/stream.
func (h *Handlers) HandleTabNetworkExportStream(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleNetworkExportStream)
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
