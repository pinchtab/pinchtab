package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

type uploadRequest struct {
	Selector  string   `json:"selector"`
	Files     []string `json:"files"`
	FileNames []string `json:"fileNames"`
	Paths     []string `json:"paths"`
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
//	  "fileNames": ["chart.png", "data.csv"],
//	  "paths": ["uploads/photo.jpg"]
//	}
//
// Either "files" (base64 data) or "paths" (relative sandbox paths) must be
// provided. Both can be combined. Files are written to a temp dir and passed to
// CDP. Path-based uploads are limited to StateDir/uploads/.
//
// "fileNames" is index-aligned with "files" and decides the name the page sees.
// A supplied name wins over the content sniff even when the two disagree,
// because that is what a browser does: it sends the user's filename and leaves
// the receiving page to decide whether to trust it. Without a name the extension
// is sniffed from content, which is all a bare blob can support — and sniffing is
// blind to exactly the text formats forms validate by extension.
func (h *Handlers) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowUpload {
		h.writeCapabilityDisabled(w, routes.CapUpload)
		return
	}
	tabID := r.URL.Query().Get("tabId")
	maxRequestBytes := h.Config.EffectiveUploadMaxRequestBytes()

	var req uploadRequest
	if err := httpx.DecodeJSON(w, r, &req, httpx.JSONBodyOptions{MaxBytes: int64(maxRequestBytes), Lenient: true}); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("invalid JSON body: %w", err))
		return
	}

	staged, stageErr := h.stageUploadFiles(req)
	if stageErr != nil {
		httpx.Error(w, stageErr.status, stageErr.err)
		return
	}
	attached := false
	defer func() {
		if !attached {
			staged.discard()
		}
	}()

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.applyTabGuards(w, r, ctx, resolvedTabID, guardTabLease|guardDialogBlocked|guardDomainPolicy|guardHandoffPause); !ok {
		return
	}

	tCtx, tCancel := httpx.WithClientTimeout(r.Context(), ctx, h.Config.ActionTimeout)
	defer tCancel()

	if req.Selector == "" {
		candidates, err := h.fileInputCandidates(tCtx, resolvedTabID)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("upload: list file inputs: %w", err))
			return
		}
		if len(candidates) > 1 {
			httpx.ErrorCode(w, http.StatusBadRequest, "upload_ambiguous", ambiguousUploadMessage(candidates), false, map[string]any{"candidates": candidates})
			return
		}
		req.Selector = defaultUploadSelector
	}

	nodeID, err := h.Bridge.ResolveSelectorToNodeID(tCtx, req.Selector, h.Bridge.GetRefCache(resolvedTabID), h.selectorFrameID(resolvedTabID))
	if err != nil {
		h.respondSelectorFailure(w, resolvedTabID, fmt.Errorf("%w: upload selector %q: %w", ErrElementNotFound, req.Selector, err))
		return
	}

	if err := h.Bridge.SetFileInputFiles(tCtx, nodeID, staged.paths); err != nil {
		if errors.Is(err, bridge.ErrInvalidActionRequest) {
			httpx.ErrorCode(w, http.StatusBadRequest, "invalid_action_request", fmt.Sprintf("upload: %v", err), false, nil)
			return
		}
		httpx.Error(w, 500, fmt.Errorf("upload: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "upload", TabID: resolvedTabID})

	attached = true
	cleanupStagedUploadDirAfter(staged.dir, uploadStagedRetention)

	httpx.JSON(w, 200, map[string]any{
		"status": "ok",
		"files":  len(staged.paths),
	})
}

type stagedUpload struct {
	paths []string
	dir   string
}

func (s stagedUpload) discard() {
	if s.dir != "" {
		_ = os.RemoveAll(s.dir)
	}
}

type uploadBudget struct {
	maxTotalBytes int
	usedBytes     int64
}

func (b *uploadBudget) spend(n int64) error {
	b.usedBytes += n
	if b.usedBytes > int64(b.maxTotalBytes) {
		return fmt.Errorf("upload payload too large: max %d bytes total", b.maxTotalBytes)
	}
	return nil
}

// stageUploadFiles returns the decoded files followed by the sandbox paths, in
// the order they are attached.
func (h *Handlers) stageUploadFiles(req uploadRequest) (stagedUpload, *statusError) {
	if len(req.Files) == 0 && len(req.Paths) == 0 {
		return stagedUpload{}, &statusError{400, fmt.Errorf("either 'files' (base64) or 'paths' (sandbox paths) required")}
	}
	if maxFiles := h.Config.EffectiveUploadMaxFiles(); len(req.Files)+len(req.Paths) > maxFiles {
		return stagedUpload{}, &statusError{400, fmt.Errorf("too many files: max %d", maxFiles)}
	}

	uploadBase := filepath.Join(h.Config.StateDir, uploadSandboxDirName)
	maxFileBytes := h.Config.EffectiveUploadMaxFileBytes()
	budget := &uploadBudget{maxTotalBytes: h.Config.EffectiveUploadMaxTotalBytes()}

	sandboxPaths, err := resolveUploadSandboxPaths(uploadBase, req.Paths, maxFileBytes, budget)
	if err != nil {
		return stagedUpload{}, err
	}
	if len(req.Files) == 0 {
		return stagedUpload{paths: sandboxPaths}, nil
	}

	dir, err := h.makeUploadStagingDir(uploadBase)
	if err != nil {
		return stagedUpload{}, err
	}
	staged := stagedUpload{dir: dir}
	for i, f := range req.Files {
		path, err := writeStagedUploadFile(dir, i, f, uploadFileName(req.FileNames, i), maxFileBytes, budget)
		if err != nil {
			staged.discard()
			return stagedUpload{}, err
		}
		staged.paths = append(staged.paths, path)
	}
	staged.paths = append(staged.paths, sandboxPaths...)
	return staged, nil
}

func resolveUploadSandboxPaths(uploadBase string, rawPaths []string, maxFileBytes int, budget *uploadBudget) ([]string, *statusError) {
	safePaths := make([]string, 0, len(rawPaths))
	for _, p := range rawPaths {
		safe, size, err := validateUploadSandboxPath(uploadBase, p, maxFileBytes)
		if err != nil {
			return nil, &statusError{400, fmt.Errorf("invalid path: %w", err)}
		}
		if err := budget.spend(size); err != nil {
			return nil, &statusError{400, err}
		}
		safePaths = append(safePaths, safe)
	}
	return safePaths, nil
}

func (h *Handlers) makeUploadStagingDir(uploadBase string) (string, *statusError) {
	stageBase := os.TempDir()
	if strings.TrimSpace(h.Config.StateDir) != "" {
		if err := os.MkdirAll(uploadBase, 0o755); err != nil {
			return "", &statusError{500, fmt.Errorf("create upload dir: %w", err)}
		}
		stageBase = uploadBase
	}
	dir, err := os.MkdirTemp(stageBase, uploadStagedDirPrefix+"*")
	if err != nil {
		return "", &statusError{500, fmt.Errorf("create staged dir: %w", err)}
	}
	return dir, nil
}

func writeStagedUploadFile(dir string, i int, encoded, name string, maxFileBytes int, budget *uploadBudget) (string, *statusError) {
	data, ext, err := decodeFileData(encoded)
	if err != nil {
		return "", &statusError{400, fmt.Errorf("file[%d]: %w", i, err)}
	}
	if len(data) > maxFileBytes {
		return "", &statusError{400, fmt.Errorf("file[%d] exceeds max size %d bytes", i, maxFileBytes)}
	}
	if err := budget.spend(int64(len(data))); err != nil {
		return "", &statusError{400, err}
	}
	path, err := stageUploadPath(dir, i, name, ext)
	if err != nil {
		return "", &statusError{400, fmt.Errorf("file[%d]: %w", i, err)}
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", &statusError{500, fmt.Errorf("write staged file: %w", err)}
	}
	return path, nil
}

const defaultUploadSelector = "input[type=file]"

type fileInputCandidate struct {
	Label    string `json:"label"`
	Selector string `json:"selector"`
}

const fileInputCandidatesExpr = `(() => {
  const matchesOnlyOne = (sel) => document.querySelectorAll(sel).length === 1;
  return Array.from(document.querySelectorAll('input[type=file]')).map((el, i) => {
    const byID = el.id ? '#' + CSS.escape(el.id) : '';
    const byName = el.name ? 'input[type=file][name=' + JSON.stringify(el.name) + ']' : '';
    const byIndex = "xpath:(//input[translate(@type,'FILE','file')='file'])[" + (i + 1) + ']';
    return {
      label: ((el.labels && el.labels[0] && el.labels[0].textContent) || el.getAttribute('aria-label') || '').trim(),
      selector: byID && matchesOnlyOne(byID) ? byID : byName && matchesOnlyOne(byName) ? byName : byIndex,
    };
  });
})()`

func (h *Handlers) fileInputCandidates(ctx context.Context, tabID string) ([]fileInputCandidate, error) {
	var candidates []fileInputCandidate
	if err := h.evaluateCount(ctx, tabID, fileInputCandidatesExpr, &candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func ambiguousUploadMessage(candidates []fileInputCandidate) string {
	described := make([]string, len(candidates))
	for i, c := range candidates {
		described[i] = strings.TrimSpace(fmt.Sprintf("%q %s", c.Label, c.Selector))
	}
	return fmt.Sprintf("upload: %d file inputs on the page and no selector given; pass one: %s", len(candidates), strings.Join(described, ", "))
}

// @Endpoint POST /tabs/{id}/upload
func (h *Handlers) HandleTabUpload(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleUpload)
}

// uploadFileName returns the caller-supplied name for file i, or "" when none
// was sent. Names are index-aligned with files and the list may be shorter.
func uploadFileName(names []string, i int) string {
	if i < 0 || i >= len(names) {
		return ""
	}
	return strings.TrimSpace(names[i])
}

// stagedUploadName reduces a caller-supplied name to the single filesystem
// element it will be staged as, falling back to the generated upload-<i><ext>
// when the caller sent nothing usable.
//
// Containment is a property of the RETURN VALUE rather than a second check after
// the fact: the result is always one path element — never empty, never "." or
// "..", never carrying a separator — so joining it under a directory cannot
// leave that directory. A re-resolution after this would be unreachable by
// construction, and an unreachable guard is one no test can hold to account.
//
// Reducing rather than refusing is what a browser does: it sends the basename of
// whatever the user picked and never the directory above it.
func stagedUploadName(name string, i int, ext string) string {
	generated := fmt.Sprintf("upload-%d%s", i, ext)

	// Reduce on BOTH separators whatever the host is. filepath.Base cannot: on a
	// Linux server its separator is "/", so a Windows caller's "C:\dir\data.csv"
	// contains no separator at all and survives whole. The cost is that a literal
	// backslash in a genuine POSIX filename is treated as a separator too, which is
	// the same trade a browser makes — it sends the final component and nothing else.
	trimmed := strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	if strings.HasSuffix(trimmed, "/") {
		return generated
	}
	base := filepath.Base(trimmed)
	switch {
	case base == "", base == ".", base == "..", strings.ContainsRune(base, 0):
		return generated
	}
	return base
}

// stageUploadPath returns the path to write file i to, under its own numbered
// subdirectory of stagedDir.
//
// The subdirectory is what keeps the caller's real basename usable: a page reads
// file.name from the basename of the path CDP was handed, so the presented name
// and the on-disk name are the same string by construction, and two files sent in
// one request under the same name (a/data.csv and b/data.csv) would otherwise
// resolve to one path that os.WriteFile truncates — losing a file silently. The
// index used to live in the filename and provided that uniqueness; moving it to
// the directory keeps it while freeing the name.
func stageUploadPath(stagedDir string, i int, name, ext string) (string, error) {
	fileDir := filepath.Join(stagedDir, strconv.Itoa(i))
	if err := os.MkdirAll(fileDir, 0o700); err != nil {
		return "", fmt.Errorf("create staged dir: %w", err)
	}
	path, err := httpx.SafeCreatePath(fileDir, stagedUploadName(name, i, ext))
	if err != nil {
		return "", fmt.Errorf("resolve staged file path: %w", err)
	}
	return path, nil
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
		parts := strings.SplitN(input, ",", 2)
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid data URL")
		}
		b64 = parts[1]
		meta := strings.TrimPrefix(parts[0], "data:")
		mime := strings.SplitN(meta, ";", 2)[0]
		ext = mimeToExt(mime)
	} else {
		b64 = input
	}

	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
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
