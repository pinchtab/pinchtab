package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/web"
)

type uploadRequest struct {
	Selector string   `json:"selector"`
	Files    []string `json:"files"`
	Paths    []string `json:"paths"`
}

// HandleUpload sets files on an <input type="file"> element via CDP.
//
// POST /upload?tabId=<id>
//
//	{
//	  "selector": "input[type=file]",
//	  "files": ["data:image/png;base64,...", "base64:..."],
//	  "paths": ["/tmp/photo.jpg"]
//	}
//
// Either "files" (base64 data) or "paths" (local file paths) must be provided.
// Both can be combined. Files are written to a temp dir and passed to CDP.
func (h *Handlers) HandleUpload(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")

	var req uploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("invalid JSON body: %w", err))
		return
	}

	if req.Selector == "" {
		req.Selector = "input[type=file]"
	}

	if len(req.Files) == 0 && len(req.Paths) == 0 {
		web.Error(w, 400, fmt.Errorf("either 'files' (base64) or 'paths' (local paths) required"))
		return
	}

	// Validate local paths exist.
	for _, p := range req.Paths {
		if _, err := os.Stat(p); err != nil {
			web.Error(w, 400, fmt.Errorf("file not found: %s", p))
			return
		}
	}

	// Decode base64 files to temp dir.
	var tempFiles []string
	if len(req.Files) > 0 {
		tmpDir, err := os.MkdirTemp("", "pinchtab-upload-*")
		if err != nil {
			web.Error(w, 500, fmt.Errorf("create temp dir: %w", err))
			return
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		for i, f := range req.Files {
			data, ext, err := decodeFileData(f)
			if err != nil {
				web.Error(w, 400, fmt.Errorf("file[%d]: %w", i, err))
				return
			}
			path := fmt.Sprintf("%s/upload-%d%s", tmpDir, i, ext)
			if err := os.WriteFile(path, data, 0644); err != nil {
				web.Error(w, 500, fmt.Errorf("write temp file: %w", err))
				return
			}
			tempFiles = append(tempFiles, path)
		}
	}

	allPaths := append(tempFiles, req.Paths...)

	ctx, _, err := h.Bridge.TabContext(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	// Find the file input node and set files via CDP.
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Evaluate selector to get the DOM node.
			nodeID, err := resolveSelector(ctx, req.Selector)
			if err != nil {
				return fmt.Errorf("selector %q: %w", req.Selector, err)
			}
			return dom.SetFileInputFiles(allPaths).WithNodeID(nodeID).Do(ctx)
		}),
	); err != nil {
		web.Error(w, 500, fmt.Errorf("upload: %w", err))
		return
	}

	web.JSON(w, 200, map[string]any{
		"status": "ok",
		"files":  len(allPaths),
	})
}

// resolveSelector finds a DOM node by CSS selector and returns its NodeID.
func resolveSelector(ctx context.Context, selector string) (cdp.NodeID, error) {
	// Use Runtime.evaluate to get the remote object, then request the node.
	expr := fmt.Sprintf(`document.querySelector(%q)`, selector)
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
