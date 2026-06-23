package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

var pdfQueryParams = map[string]struct{}{
	"landscape":               {},
	"preferCSSPageSize":       {},
	"displayHeaderFooter":     {},
	"generateTaggedPDF":       {},
	"generateDocumentOutline": {},
	"scale":                   {},
	"paperWidth":              {},
	"paperHeight":             {},
	"marginTop":               {},
	"marginBottom":            {},
	"marginLeft":              {},
	"marginRight":             {},
	"pageRanges":              {},
	"headerTemplate":          {},
	"footerTemplate":          {},
	"output":                  {},
	"path":                    {},
	"raw":                     {},
}

var pdfActiveTemplatePattern = regexp.MustCompile(`(?i)<\s*script\b|javascript\s*:|\bon[a-z]+\s*=`)

// HandlePDF generates a PDF of the current tab.
//
// @Endpoint GET /pdf
func (h *Handlers) HandlePDF(w http.ResponseWriter, r *http.Request) {
	headerTemplate := r.URL.Query().Get("headerTemplate")
	footerTemplate := r.URL.Query().Get("footerTemplate")
	if err := validatePDFTemplate(headerTemplate); err != nil {
		httpx.Error(w, http.StatusBadRequest, err)
		return
	}
	if err := validatePDFTemplate(footerTemplate); err != nil {
		httpx.Error(w, http.StatusBadRequest, err)
		return
	}

	if !h.ensureBrowserOrRespond(w, h.Config) {
		return
	}

	tabID := r.URL.Query().Get("tabId")
	output := r.URL.Query().Get("output")
	h.recordReadRequest(r, "pdf", tabID)

	_, tCtx, cancel, ok := h.resolveBinaryReadContext(w, r, tabID, h.Config.ActionTimeout)
	if !ok {
		return
	}
	defer cancel()

	landscape := r.URL.Query().Get("landscape") == "true"
	preferCSSPageSize := r.URL.Query().Get("preferCSSPageSize") == "true"
	displayHeaderFooter := r.URL.Query().Get("displayHeaderFooter") == "true"
	generateTaggedPDF := r.URL.Query().Get("generateTaggedPDF") == "true"
	generateDocumentOutline := r.URL.Query().Get("generateDocumentOutline") == "true"

	scale := 1.0
	if s := r.URL.Query().Get("scale"); s != "" {
		if sn, err := strconv.ParseFloat(s, 64); err == nil && sn > 0 {
			scale = sn
		}
	}

	paperWidth := 8.5 // Default letter width in inches
	if w := r.URL.Query().Get("paperWidth"); w != "" {
		if wn, err := strconv.ParseFloat(w, 64); err == nil && wn > 0 {
			paperWidth = wn
		}
	}

	paperHeight := 11.0 // Default letter height in inches
	if h := r.URL.Query().Get("paperHeight"); h != "" {
		if hn, err := strconv.ParseFloat(h, 64); err == nil && hn > 0 {
			paperHeight = hn
		}
	}

	marginTop := 0.4 // Default margins in inches (1cm)
	if m := r.URL.Query().Get("marginTop"); m != "" {
		if mn, err := strconv.ParseFloat(m, 64); err == nil && mn >= 0 {
			marginTop = mn
		}
	}

	marginBottom := 0.4
	if m := r.URL.Query().Get("marginBottom"); m != "" {
		if mn, err := strconv.ParseFloat(m, 64); err == nil && mn >= 0 {
			marginBottom = mn
		}
	}

	marginLeft := 0.4
	if m := r.URL.Query().Get("marginLeft"); m != "" {
		if mn, err := strconv.ParseFloat(m, 64); err == nil && mn >= 0 {
			marginLeft = mn
		}
	}

	marginRight := 0.4
	if m := r.URL.Query().Get("marginRight"); m != "" {
		if mn, err := strconv.ParseFloat(m, 64); err == nil && mn >= 0 {
			marginRight = mn
		}
	}

	pageRanges := r.URL.Query().Get("pageRanges") // e.g., "1-3,5"
	// IDPI: scan page title, URL, and body text for injection patterns before
	// rendering to PDF. PDF output is opaque binary — any signal is conveyed
	// via response headers. The scan timeout is taken from IDPI config so
	// operators can tune it without recompiling.
	if h.Config.IDPI.Enabled && h.Config.IDPI.ScanContent {
		scanTimeout := time.Duration(h.Config.IDPI.ScanTimeoutSec) * time.Second
		if scanTimeout <= 0 {
			scanTimeout = 5 * time.Second
		}
		scanCtx, scanCancel := context.WithTimeout(tCtx, scanTimeout)
		defer scanCancel()
		pageTitle, _ := h.Bridge.CurrentTitle(scanCtx)
		pageURL, _ := h.Bridge.CurrentURL(scanCtx)
		var pageText string
		_ = h.Bridge.Evaluate(scanCtx, `document.body ? document.body.innerText : ""`, &pageText, bridge.EvalOpts{})
		corpus := pageTitle + "\n" + pageURL + "\n" + pageText
		scanResult := h.ContentGuard.ScanOnly(corpus)
		if scanResult.Blocked {
			httpx.Error(w, http.StatusForbidden, fmt.Errorf("idpi: %s", scanResult.BlockReason))
			return
		}
		scanResult.SetHeaders(w)
	}

	buf, err := h.Bridge.PrintToPDF(tCtx, bridge.PDFParams{
		Landscape:               landscape,
		PrintBackground:         true,
		Scale:                   scale,
		PaperWidth:              paperWidth,
		PaperHeight:             paperHeight,
		MarginTop:               marginTop,
		MarginBottom:            marginBottom,
		MarginLeft:              marginLeft,
		MarginRight:             marginRight,
		PageRanges:              pageRanges,
		PreferCSSPageSize:       preferCSSPageSize,
		DisplayHeaderFooter:     displayHeaderFooter,
		GenerateTaggedPDF:       generateTaggedPDF,
		GenerateDocumentOutline: generateDocumentOutline,
		HeaderTemplate:          headerTemplate,
		FooterTemplate:          footerTemplate,
	})
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("pdf: %w", err))
		return
	}

	if output == "file" {
		savePath := r.URL.Query().Get("path")
		if savePath == "" {
			p, _, err := saveBinaryToStateDir(h.Config.StateDir, "pdfs", "page", ".pdf", buf)
			if err != nil {
				httpx.Error(w, 500, fmt.Errorf("write pdf: %w", err))
				return
			}
			savePath = p
		} else {
			safe, err := httpx.SafeCreatePath(h.Config.StateDir, savePath)
			if err != nil {
				httpx.Error(w, 400, fmt.Errorf("invalid path: %w", err))
				return
			}
			absBase, _ := filepath.Abs(h.Config.StateDir)
			absPath, err := filepath.Abs(safe)
			if err != nil || !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
				httpx.Error(w, 400, fmt.Errorf("invalid output path"))
				return
			}
			savePath = absPath
			if err := os.MkdirAll(filepath.Dir(savePath), 0750); err != nil {
				httpx.Error(w, 500, fmt.Errorf("create dir: %w", err))
				return
			}
			if err := os.WriteFile(savePath, buf, 0600); err != nil {
				httpx.Error(w, 500, fmt.Errorf("write pdf: %w", err))
				return
			}
		}

		httpx.JSON(w, 200, map[string]any{
			"path": savePath,
			"size": len(buf),
		})
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		writeRawImage(w, buf, "application/pdf", "pdf write")
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"format": "pdf",
		"base64": base64.StdEncoding.EncodeToString(buf),
	})
}

func validatePDFTemplate(template string) error {
	if template == "" {
		return nil
	}
	if pdfActiveTemplatePattern.MatchString(template) {
		return fmt.Errorf("invalid pdf template")
	}
	return nil
}

// HandleTabPDF generates a PDF for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/pdf
// @Endpoint POST /tabs/{id}/pdf
func (h *Handlers) HandleTabPDF(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	q := r.URL.Query()
	if r.Method == http.MethodPost {
		var body map[string]any
		if r.ContentLength > 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&body); err != nil {
				httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
				return
			}
			for key, value := range body {
				if _, ok := pdfQueryParams[key]; !ok {
					continue
				}
				switch v := value.(type) {
				case string:
					q.Set(key, v)
				case bool:
					q.Set(key, strconv.FormatBool(v))
				case float64:
					q.Set(key, strconv.FormatFloat(v, 'f', -1, 64))
				default:
					httpx.Error(w, 400, fmt.Errorf("invalid %s type", key))
					return
				}
			}
		}
	}
	q.Set("tabId", tabID)

	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u

	h.HandlePDF(w, req)
}
