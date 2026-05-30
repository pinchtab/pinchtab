package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type uploadRequest struct {
	Selector string   `json:"selector"`
	Files    []string `json:"files"`
	Paths    []string `json:"paths"`
}

const (
	uploadSandboxDirName  = "uploads"
	uploadStagedDirPrefix = "pinchtab-upload-"

	// CDP keeps only the file path on the input, so decoded files need to remain
	// available after /upload returns. Bound the lifetime so persistent StateDir
	// installs do not retain successful base64 uploads forever.
	uploadStagedRetention = 24 * time.Hour
)

var (
	setUploadFileInputFiles = func(ctx context.Context, selector string, paths []string) error {
		return chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				nodeID, err := resolveSelector(ctx, selector)
				if err != nil {
					return fmt.Errorf("selector %q: %w", selector, err)
				}
				return dom.SetFileInputFiles(paths).WithNodeID(nodeID).Do(ctx)
			}),
		)
	}

	cleanupStagedUploadDirAfter = func(dir string, after time.Duration) {
		if dir == "" {
			return
		}
		if after <= 0 {
			_ = os.RemoveAll(dir)
			return
		}
		time.AfterFunc(after, func() {
			_ = os.RemoveAll(dir)
		})
	}
)

// HandleUpload sets files on an <input type="file"> element via CDP.
//
// POST /upload?tabId=<id>
//
//	{
//	  "selector": "input[type=file]",   // unified selector: CSS, XPath, text, ref, or semantic
//	  "files": ["data:image/png;base64,...", "base64:..."],
//	  "paths": ["uploads/photo.jpg"]
//	}
//
// Either "files" (base64 data) or "paths" (relative sandbox paths) must be
// provided. Both can be combined. Files are written to a temp dir and passed to
// CDP. Path-based uploads are limited to StateDir/uploads/. Successful base64
// staging dirs are retained briefly for lazy browser reads, then cleaned.
func (h *Handlers) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowUpload {
		httpx.ErrorCode(w, 403, "upload_disabled", httpx.DisabledEndpointMessage("upload", "security.allowUpload"), false, map[string]any{
			"setting": "security.allowUpload",
		})
		return
	}
	tabID := r.URL.Query().Get("tabId")
	maxRequestBytes := h.Config.EffectiveUploadMaxRequestBytes()
	maxFiles := h.Config.EffectiveUploadMaxFiles()
	maxFileBytes := h.Config.EffectiveUploadMaxFileBytes()
	maxTotalBytes := h.Config.EffectiveUploadMaxTotalBytes()

	r.Body = http.MaxBytesReader(w, r.Body, int64(maxRequestBytes))

	var req uploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("invalid JSON body: %w", err))
		return
	}

	if req.Selector == "" {
		req.Selector = "input[type=file]"
	}

	if len(req.Files) == 0 && len(req.Paths) == 0 {
		httpx.Error(w, 400, fmt.Errorf("either 'files' (base64) or 'paths' (sandbox paths) required"))
		return
	}
	if len(req.Files)+len(req.Paths) > maxFiles {
		httpx.Error(w, 400, fmt.Errorf("too many files: max %d", maxFiles))
		return
	}

	uploadBase := filepath.Join(h.Config.StateDir, uploadSandboxDirName)
	var totalBytes int64
	for i, p := range req.Paths {
		safe, size, err := validateUploadSandboxPath(uploadBase, p, maxFileBytes)
		if err != nil {
			httpx.Error(w, 400, fmt.Errorf("invalid path: %w", err))
			return
		}
		totalBytes += size
		if totalBytes > int64(maxTotalBytes) {
			httpx.Error(w, 400, fmt.Errorf("upload payload too large: max %d bytes total", maxTotalBytes))
			return
		}
		req.Paths[i] = safe
	}

	// Decode base64 files into a staged dir that OUTLIVES this request. CDP
	// SetFileInputFiles only records the path on the <input>; the browser reads the
	// bytes LAZILY at form-submit time, which is typically a separate, later
	// request. Deleting the decoded file when this handler returns (the previous
	// `defer os.RemoveAll`) left the file gone by submit time, so multipart
	// submissions failed with ERR_FILE_NOT_FOUND ("Your file couldn't be accessed.
	// It may have been moved, edited, or deleted."). Keep the staged files on the
	// success path long enough for lazy reads, and only remove them immediately if
	// the upload fails before attaching to a tab.
	var tempFiles []string
	var stagedDir string
	uploadSucceeded := false
	defer func() {
		if !uploadSucceeded && stagedDir != "" {
			_ = os.RemoveAll(stagedDir)
		}
	}()
	if len(req.Files) > 0 {
		// Stage under StateDir/uploads when configured so paths survive this
		// request, else the OS temp dir; never the process working directory.
		stageBase := os.TempDir()
		if strings.TrimSpace(h.Config.StateDir) != "" {
			if err := os.MkdirAll(uploadBase, 0o755); err != nil {
				httpx.Error(w, 500, fmt.Errorf("create upload dir: %w", err))
				return
			}
			stageBase = uploadBase
		}
		dir, err := os.MkdirTemp(stageBase, uploadStagedDirPrefix+"*")
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("create staged dir: %w", err))
			return
		}
		stagedDir = dir

		for i, f := range req.Files {
			data, ext, err := decodeFileData(f)
			if err != nil {
				httpx.Error(w, 400, fmt.Errorf("file[%d]: %w", i, err))
				return
			}
			if len(data) > maxFileBytes {
				httpx.Error(w, 400, fmt.Errorf("file[%d] exceeds max size %d bytes", i, maxFileBytes))
				return
			}
			totalBytes += int64(len(data))
			if totalBytes > int64(maxTotalBytes) {
				httpx.Error(w, 400, fmt.Errorf("upload payload too large: max %d bytes total", maxTotalBytes))
				return
			}
			path := fmt.Sprintf("%s/upload-%d%s", stagedDir, i, ext)
			if err := os.WriteFile(path, data, 0600); err != nil {
				httpx.Error(w, 500, fmt.Errorf("write staged file: %w", err))
				return
			}
			tempFiles = append(tempFiles, path)
		}
	}

	allPaths := append(tempFiles, req.Paths...)

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	owner := resolveOwner(r, "")
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		httpx.ErrorCode(w, http.StatusLocked, "tab_locked", err.Error(), false, nil)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	if err := setUploadFileInputFiles(tCtx, req.Selector, allPaths); err != nil {
		httpx.Error(w, 500, fmt.Errorf("upload: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "upload", TabID: resolvedTabID})

	// Files attached successfully; keep the staged copies so the browser can read
	// them at form-submit time, but schedule bounded cleanup.
	uploadSucceeded = true
	if stagedDir != "" {
		cleanupStagedUploadDirAfter(stagedDir, uploadStagedRetention)
	}

	httpx.JSON(w, 200, map[string]any{
		"status": "ok",
		"files":  len(allPaths),
	})
}

// HandleTabUpload uploads files for a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/upload
func (h *Handlers) HandleTabUpload(w http.ResponseWriter, r *http.Request) {
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

	h.HandleUpload(w, req)
}

// resolveSelector finds a DOM node by a unified selector string and returns its NodeID.
// Supports CSS (default), XPath (xpath: prefix or // auto-detect), and text (text: prefix).
func resolveSelector(ctx context.Context, sel string) (cdp.NodeID, error) {
	// Determine the JavaScript expression based on selector type.
	var expr string
	switch {
	case strings.HasPrefix(sel, "xpath:"):
		xpath := sel[len("xpath:"):]
		expr = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue})()`, xpath)
	case strings.HasPrefix(sel, "//") || strings.HasPrefix(sel, "(//"):
		expr = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue})()`, sel)
	case strings.HasPrefix(sel, "text:"):
		text := sel[len("text:"):]
		expr = fmt.Sprintf(`(function(){var w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(w.nextNode()){if(w.currentNode.textContent.includes(%q))return w.currentNode.parentElement}return null})()`, text)
	case strings.HasPrefix(sel, "css:"):
		css := sel[len("css:"):]
		expr = fmt.Sprintf(`document.querySelector(%q)`, css)
	default:
		// Bare selector: treat as CSS (backward compatible).
		expr = fmt.Sprintf(`document.querySelector(%q)`, sel)
	}

	val, _, err := runtime.Evaluate(expr).Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("evaluate: %w", err)
	}
	if val.ObjectID == "" {
		return 0, fmt.Errorf("no element matches selector")
	}
	node, err := dom.RequestNode(val.ObjectID).Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("request node: %w", err)
	}
	return node, nil
}

func validateUploadSandboxPath(baseDir, rawPath string, maxFileBytes int) (string, int64, error) {
	normalized := normalizeUploadSandboxPath(rawPath)
	safe, err := httpx.SafeExistingPath(baseDir, normalized)
	if err != nil {
		return "", 0, err
	}
	info, err := os.Lstat(safe)
	if err != nil {
		return "", 0, fmt.Errorf("file not found: %s", safe)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", 0, fmt.Errorf("symlinks are not allowed: %s", rawPath)
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("path must reference a regular file: %s", rawPath)
	}
	if info.Size() > int64(maxFileBytes) {
		return "", 0, fmt.Errorf("file exceeds max size %d bytes: %s", maxFileBytes, rawPath)
	}
	return safe, info.Size(), nil
}

func normalizeUploadSandboxPath(rawPath string) string {
	trimmed := filepath.ToSlash(strings.TrimSpace(rawPath))
	trimmed = strings.TrimPrefix(trimmed, uploadSandboxDirName+"/")
	return filepath.FromSlash(trimmed)
}

// CleanupStaleUploads removes old decoded-upload staging dirs left in StateDir.
// It is intentionally scoped to dirs created by HandleUpload and leaves
// user-managed files in StateDir/uploads untouched.
func CleanupStaleUploads(stateDir string) {
	if strings.TrimSpace(stateDir) == "" {
		return
	}
	uploadBase := filepath.Join(stateDir, uploadSandboxDirName)
	entries, err := os.ReadDir(uploadBase)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-uploadStagedRetention)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), uploadStagedDirPrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(uploadBase, entry.Name()))
	}
}

// decodeFileData handles "data:mime;base64,..." and raw base64 strings.
// Returns decoded bytes and a file extension guess.
func decodeFileData(input string) ([]byte, string, error) {
	ext := ""
	var b64 string

	if strings.HasPrefix(input, "data:") {
		// data:image/png;base64,iVBOR...
		parts := strings.SplitN(input, ",", 2)
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid data URL")
		}
		b64 = parts[1]
		// Extract mime for extension.
		meta := strings.TrimPrefix(parts[0], "data:")
		mime := strings.SplitN(meta, ";", 2)[0]
		ext = mimeToExt(mime)
	} else {
		b64 = input
	}

	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		// Try URL-safe encoding.
		data, err = base64.URLEncoding.DecodeString(b64)
		if err != nil {
			return nil, "", fmt.Errorf("base64 decode: %w", err)
		}
	}

	if ext == "" {
		ext = sniffExt(data)
	}

	return data, ext, nil
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "text/csv":
		return ".csv"
	default:
		return ".bin"
	}
}

func sniffExt(data []byte) string {
	if len(data) < 4 {
		return ".bin"
	}
	switch {
	case data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return ".png"
	case data[0] == 0xFF && data[1] == 0xD8:
		return ".jpg"
	case string(data[:3]) == "GIF":
		return ".gif"
	case string(data[:4]) == "RIFF" && len(data) > 11 && string(data[8:12]) == "WEBP":
		return ".webp"
	case data[0] == '%' && data[1] == 'P' && data[2] == 'D' && data[3] == 'F':
		return ".pdf"
	default:
		return ".bin"
	}
}
