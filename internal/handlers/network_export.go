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
// @Response 200 application/har+json|application/x-ndjson  Exported data (streamed)
// @Response 200 application/json                           File save result when output=file
// @Response 400 application/json                           Invalid format or parameters
// @Response 423 application/json                           Tab is locked
// @Response 500 application/json                           Export error
func (h *Handlers) HandleNetworkExport(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
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

	h.recordReadRequest(r, "network-export", resolvedTabID)

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
		return
	}

	nm := h.Bridge.NetworkMonitor()
	if nm == nil {
		enc := factory("PinchTab", h.version())
		w.Header().Set("Content-Type", enc.ContentType())
		if err := enc.Start(w); err != nil {
			return
		}
		_ = enc.Finish()
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

	filter := parseNetworkFilter(r)
	entries := buf.List(filter)
	if filter.Limit > 0 && len(entries) > filter.Limit {
		entries = entries[len(entries)-filter.Limit:]
	}

	includeBody := r.URL.Query().Get("body") == "true"
	redactHeaders := r.URL.Query().Get("redact") != "false"
	output := r.URL.Query().Get("output")

	// Timeout + client disconnect for body fetches (#4, #5)
	fetchCtx, fetchCancel := context.WithTimeout(tabCtx, h.Config.ActionTimeout)
	defer fetchCancel()
	go httpx.CancelOnClientDone(r.Context(), fetchCancel)

	// Convert entries with throttled body fetches to avoid tying up the tab context.
	exportEntries := make([]observe.ExportEntry, len(entries))
	bodySem := make(chan struct{}, bodyFetchConcurrency)
	var wg sync.WaitGroup

	for i, entry := range entries {
		needBody := includeBody && entry.Finished && !entry.Failed
		if needBody {
			wg.Add(1)
			bodySem <- struct{}{}
			go func(idx int, ent bridge.NetworkEntry) {
				defer wg.Done()
				defer func() { <-bodySem }()
				body, b64, _ := nm.GetResponseBody(fetchCtx, ent.RequestID)
				if len(body) > maxExportBodyBytes {
					body = ""
					b64 = false
				}
				e := observe.NetworkEntryToExport(ent, body, b64)
				if redactHeaders {
					e.Request.Headers = observe.RedactSensitiveHeaders(e.Request.Headers)
					e.Response.Headers = observe.RedactSensitiveHeaders(e.Response.Headers)
				}
				exportEntries[idx] = e
			}(i, entry)
		} else {
			e := observe.NetworkEntryToExport(entry, "", false)
			if redactHeaders {
				e.Request.Headers = observe.RedactSensitiveHeaders(e.Request.Headers)
				e.Response.Headers = observe.RedactSensitiveHeaders(e.Response.Headers)
			}
			exportEntries[i] = e
		}
	}
	wg.Wait()

	enc := factory("PinchTab", h.version())

	if output == "file" {
		if err := h.writeExportFile(w, r, enc, exportEntries, formatName); err != nil {
			httpx.Error(w, 500, fmt.Errorf("write file: %w", err))
		}
		return
	}

	// Stream to response
	w.Header().Set("Content-Type", enc.ContentType())
	if err := enc.Start(w); err != nil {
		return
	}
	for _, entry := range exportEntries {
		if err := enc.Encode(entry); err != nil {
			return
		}
	}
	_ = enc.Finish()
}

func (h *Handlers) writeExportFile(
	w http.ResponseWriter,
	r *http.Request,
	enc observe.ExportEncoder,
	entries []observe.ExportEntry,
	formatName string,
) error {
	userPath := r.URL.Query().Get("path")
	if userPath == "" {
		ts := time.Now().Format("20060102-150405")
		userPath = fmt.Sprintf("network-%s%s", ts, enc.FileExtension())
	}

	// Path safety: use SafeCreatePath + containment check (#1)
	exportDir := filepath.Join(h.Config.StateDir, "exports")
	if err := os.MkdirAll(exportDir, 0750); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	safeName := filepath.Base(userPath)
	finalPath, err := httpx.SafeCreatePath(exportDir, safeName)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	absBase, _ := filepath.Abs(exportDir)
	absPath, err := filepath.Abs(finalPath)
	if err != nil || !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
		return fmt.Errorf("path escapes export directory")
	}

	tmpPath := absPath + ".tmp"

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if err := enc.Start(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
	}
	if err := enc.Finish(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
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
		"entries": len(entries),
		"format":  formatName,
	})
	return nil
}

// HandleTabNetworkExport handles GET /tabs/{id}/network/export.
func (h *Handlers) HandleTabNetworkExport(w http.ResponseWriter, r *http.Request) {
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
	h.HandleNetworkExport(w, req)
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
