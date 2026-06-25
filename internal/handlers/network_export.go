package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// maxExportBodyBytes caps the size of a single response body included in an export.
const maxExportBodyBytes = 10 << 20 // 10 MB

// maxExportStreamDuration caps how long a streaming export can run before auto-closing.
const maxExportStreamDuration = 30 * time.Minute

// bodyFetchConcurrency limits parallel CDP GetResponseBody calls to avoid tying up the tab.
const bodyFetchConcurrency = 4

// entryFinishWait caps how long a live-stream entry may stay pending while waiting for
// responseReceived/loadingFinished before it is exported best-effort.
const entryFinishWait = 10 * time.Second

// staleTmpAge is the minimum age of a .tmp file before it's considered orphaned.
const staleTmpAge = 5 * time.Minute

// CleanupStaleTmpExports removes orphaned .tmp files from the exports directory.
// Called at startup to handle files left behind by a crash or hard kill.
func CleanupStaleTmpExports(stateDir string) {
	exportDir := filepath.Join(stateDir, "exports")
	entries, err := os.ReadDir(exportDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleTmpAge)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue // skip files that might still be in-flight
		}
		_ = os.Remove(filepath.Join(exportDir, entry.Name()))
	}
}

// HandleNetworkExport exports captured network data in a registered format (HAR, NDJSON, etc.).
//
// @Endpoint GET /network/export
// @Description Export captured network entries in HAR 1.2, NDJSON, or other registered formats
//
// @Param tabId   string query Tab ID (optional, uses current tab)
// @Param format  string query Export format: "har" (default), "ndjson", or any registered format
// @Param output  string query "file" to save to disk (optional)
// @Param path    string query Filename when output=file (optional, auto-generated if omitted)
// @Param body    string query "true" to include response bodies (can be slow)
// @Param redact  string query "false" to include sensitive headers like cookies (default: redacted)
// @Param filter  string query URL pattern filter
// @Param method  string query HTTP method filter
// @Param status  string query Status code range filter (e.g. "4xx")
// @Param type    string query Resource type filter
// @Param limit   string query Maximum entries to export
//
// @Response 200 application/har+json|application/x-ndjson  Exported data (encoded incrementally, in capture order)
// @Response 200 application/json                           File save result when output=file
// @Response 400 application/json                           Invalid format or parameters
// @Response 423 application/json                           Tab is locked
// @Response 500 application/json                           Export error
func (h *Handlers) HandleNetworkExport(w http.ResponseWriter, r *http.Request) {
	if !h.ensureBrowserReady(w) {
		return
	}

	ec, ok := h.resolveExportContext(w, r, "network-export")
	if !ok {
		return
	}

	nm := h.Bridge.NetworkMonitor()
	if nm == nil {
		// No monitor: emit an empty document so callers still get a valid export.
		enc := ec.factory("PinchTab", h.version())
		w.Header().Set("Content-Type", enc.ContentType())
		if err := enc.Start(w); err != nil {
			return
		}
		_ = enc.Finish()
		return
	}

	buf, ok := h.ensureCaptureBuffer(w, r, nm, ec.tabCtx, ec.resolvedTabID)
	if !ok {
		return
	}

	filter := parseNetworkFilter(r)
	entries := buf.List(filter)
	if filter.Limit > 0 && len(entries) > filter.Limit {
		entries = entries[len(entries)-filter.Limit:]
	}

	includeBody := r.URL.Query().Get("body") == "true"
	redactHeaders := r.URL.Query().Get("redact") != "false"
	output := r.URL.Query().Get("output")

	fetchCtx, fetchCancel := context.WithTimeout(ec.tabCtx, h.Config.ActionTimeout)
	defer fetchCancel()
	go httpx.CancelOnClientDone(r.Context(), fetchCancel)

	enc := ec.factory("PinchTab", h.version())

	// Entries are encoded incrementally in capture order as their bodies resolve;
	// streamExportEntries caps in-memory entries at bodyFetchConcurrency rather than
	// buffering the whole []observe.ExportEntry before encoding.
	if output == "file" {
		if err := h.writeExportFile(w, r, enc, ec.formatName, func(emit func(observe.ExportEntry) error) error {
			return h.streamExportEntries(fetchCtx, nm, entries, includeBody, redactHeaders, emit)
		}); err != nil {
			httpx.Error(w, 500, fmt.Errorf("write file: %w", err))
		}
		return
	}

	w.Header().Set("Content-Type", enc.ContentType())
	if err := enc.Start(w); err != nil {
		return
	}
	if err := h.streamExportEntries(fetchCtx, nm, entries, includeBody, redactHeaders, enc.Encode); err != nil {
		return
	}
	_ = enc.Finish()
}

// exportContext is the resolved prelude shared by the two export endpoints.
type exportContext struct {
	tabCtx        context.Context
	resolvedTabID string
	factory       observe.ExportEncoderFactory
	formatName    string
}

// resolveExportContext runs the post-browser export prelude shared by
// /network/export and /network/export/stream: tab resolution + domain policy,
// tab-lease enforcement, read-request bookkeeping, and export-format resolution.
// The caller runs the browser-ready guard (and, for the streaming variant, the
// path-required check) first so each endpoint keeps its own ordering. On any
// failure it writes the response and returns ok=false.
func (h *Handlers) resolveExportContext(w http.ResponseWriter, r *http.Request, label string) (exportContext, bool) {
	tabCtx, resolvedTabID, ok := h.resolveNetworkTab(w, r)
	if !ok {
		return exportContext{}, false
	}

	owner := resolveOwner(r, "")
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		httpx.ErrorCode(w, http.StatusLocked, "tab_locked", err.Error(), false, nil)
		return exportContext{}, false
	}

	h.recordReadRequest(r, label, resolvedTabID)

	formatName := r.URL.Query().Get("format")
	if formatName == "" {
		formatName = "har"
	}
	factory := observe.GetFormat(formatName)
	if factory == nil {
		httpx.JSON(w, 400, map[string]any{
			"code":      "unknown_format",
			"error":     fmt.Sprintf("unknown export format %q", formatName),
			"available": observe.ListFormats(),
		})
		return exportContext{}, false
	}

	return exportContext{tabCtx: tabCtx, resolvedTabID: resolvedTabID, factory: factory, formatName: formatName}, true
}

// resolveExportBody returns the response body and base64 flag for an export entry,
// preferring the body already retained in the capture buffer (no CDP round-trip,
// and it survives the resource being evicted from Chrome) and only falling back to
// a live GetResponseBody fetch when nothing is retained.
func resolveExportBody(ctx context.Context, nm *bridge.NetworkMonitor, entry bridge.NetworkEntry) (string, bool) {
	if entry.BodyRetained {
		return clampExportBody(entry.ResponseBody, entry.Base64Encoded)
	}
	body, b64, _ := nm.GetResponseBody(ctx, entry.RequestID)
	return clampExportBody(body, b64)
}

// streamExportEntries converts entries to ExportEntry values and invokes emit for
// each, in the original capture order, fetching response bodies through a bounded
// concurrent pipeline. A producer goroutine cannot run more than bodyFetchConcurrency
// entries ahead of the consumer, so at most that many entries are materialized in
// memory at once — a large capture is never buffered whole. It returns the first
// emit error after draining outstanding fetches so no goroutine leaks.
func (h *Handlers) streamExportEntries(
	ctx context.Context,
	nm *bridge.NetworkMonitor,
	entries []bridge.NetworkEntry,
	includeBody, redactHeaders bool,
	emit func(observe.ExportEntry) error,
) error {
	results := make([]chan observe.ExportEntry, len(entries))
	for i := range results {
		results[i] = make(chan observe.ExportEntry, 1)
	}

	// A slot is acquired before a body fetch starts and released by the consumer
	// once it has encoded that entry, capping in-flight (fetched-but-unencoded)
	// entries — and therefore memory — at bodyFetchConcurrency.
	sem := make(chan struct{}, bodyFetchConcurrency)
	go func() {
		var wg sync.WaitGroup
		for i, entry := range entries {
			sem <- struct{}{}
			wg.Add(1)
			go func(idx int, ent bridge.NetworkEntry) {
				defer wg.Done()
				var body string
				var b64 bool
				if includeBody && ent.Finished && !ent.Failed {
					body, b64 = resolveExportBody(ctx, nm, ent)
				}
				results[idx] <- toExportEntry(ent, body, b64, redactHeaders)
			}(i, entry)
		}
		wg.Wait()
	}()

	var emitErr error
	for i := range results {
		entry := <-results[i]
		if emitErr == nil {
			if err := emit(entry); err != nil {
				emitErr = err
			}
		}
		// Always release the slot, even after an emit error, so producers blocked
		// on sem can finish (drain to avoid leaking goroutines).
		<-sem
	}
	return emitErr
}

// resolveExportFile sanitizes userPath into the server-controlled exports dir
// (filepath.Base + SafeCreatePath + abs-prefix containment), creates the dir,
// and opens the .tmp file 0600. status is the HTTP code for err (400 for
// path/sanitization failures, 500 for filesystem failures). This is the ONLY
// path both export writers may use — do not re-implement it.
func (h *Handlers) resolveExportFile(userPath string) (absPath, tmpPath string, f *os.File, status int, err error) {
	exportDir := filepath.Join(h.Config.StateDir, "exports")
	if err = os.MkdirAll(exportDir, 0750); err != nil {
		return "", "", nil, 500, fmt.Errorf("create dir: %w", err)
	}
	safeName := filepath.Base(userPath)
	safePath, err := httpx.SafeCreatePath(exportDir, safeName)
	if err != nil {
		return "", "", nil, 400, fmt.Errorf("invalid path: %w", err)
	}
	absBase, _ := filepath.Abs(exportDir)
	absPath, err = filepath.Abs(safePath)
	if err != nil || !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
		return "", "", nil, 400, fmt.Errorf("path escapes export directory")
	}
	tmpPath = absPath + ".tmp"
	f, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", "", nil, 500, fmt.Errorf("create file: %w", err)
	}
	return absPath, tmpPath, f, 200, nil
}

// clampExportBody drops a response body that exceeds the export size cap.
func clampExportBody(body string, b64 bool) (string, bool) {
	if len(body) > maxExportBodyBytes {
		return "", false
	}
	return body, b64
}

// toExportEntry converts a captured entry to an export entry, redacting
// sensitive request/response headers when redactHeaders is set.
func toExportEntry(entry bridge.NetworkEntry, body string, b64, redactHeaders bool) observe.ExportEntry {
	e := observe.NetworkEntryToExport(entry, body, b64)
	if redactHeaders {
		e.Request.Headers = observe.RedactSensitiveHeaders(e.Request.Headers)
		e.Response.Headers = observe.RedactSensitiveHeaders(e.Response.Headers)
	}
	return e
}

// writeExportFile encodes an export to a temp file via encodeAll (which pushes
// each ExportEntry through emit), then atomically renames it into place. encodeAll
// drives the incremental producer so entries are never buffered whole in memory.
func (h *Handlers) writeExportFile(
	w http.ResponseWriter,
	r *http.Request,
	enc observe.ExportEncoder,
	formatName string,
	encodeAll func(emit func(observe.ExportEntry) error) error,
) error {
	userPath := r.URL.Query().Get("path")
	if userPath == "" {
		ts := time.Now().Format("20060102-150405")
		userPath = fmt.Sprintf("network-%s%s", ts, enc.FileExtension())
	}

	absPath, tmpPath, f, _, err := h.resolveExportFile(userPath)
	if err != nil {
		return err
	}

	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmpPath)
	}

	if err := enc.Start(f); err != nil {
		cleanup()
		return err
	}
	count := 0
	if err := encodeAll(func(entry observe.ExportEntry) error {
		if err := enc.Encode(entry); err != nil {
			return err
		}
		count++
		return nil
	}); err != nil {
		cleanup()
		return err
	}
	if err := enc.Finish(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, absPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	httpx.JSON(w, 200, map[string]any{
		"path":    absPath,
		"entries": count,
		"format":  formatName,
	})
	return nil
}

// HandleTabNetworkExport handles GET /tabs/{id}/network/export.
func (h *Handlers) HandleTabNetworkExport(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleNetworkExport)
}

func parseNetworkFilter(r *http.Request) bridge.NetworkFilter {
	f := bridge.NetworkFilter{
		URLPattern:   r.URL.Query().Get("filter"),
		Method:       r.URL.Query().Get("method"),
		StatusRange:  r.URL.Query().Get("status"),
		ResourceType: r.URL.Query().Get("type"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	return f
}

func (h *Handlers) version() string {
	if h.Version != "" {
		return h.Version
	}
	return "dev"
}
