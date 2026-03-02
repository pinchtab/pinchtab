package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/web"
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

// HandlePDF generates a PDF of the current tab.
//
// @Endpoint GET /pdf
func (h *Handlers) HandlePDF(w http.ResponseWriter, r *http.Request) {
	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	tabID := r.URL.Query().Get("tabId")
	output := r.URL.Query().Get("output")

	ctx, _, err := h.Bridge.TabContext(tabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	// Parse PDF parameters from PrintToPDFParams
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
	headerTemplate := r.URL.Query().Get("headerTemplate")
	footerTemplate := r.URL.Query().Get("footerTemplate")

	var buf []byte
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			p := page.PrintToPDF().
				WithPrintBackground(true).
				WithScale(scale).
				WithLandscape(landscape).
				WithPaperWidth(paperWidth).
				WithPaperHeight(paperHeight).
				WithMarginTop(marginTop).
				WithMarginBottom(marginBottom).
				WithMarginLeft(marginLeft).
				WithMarginRight(marginRight).
				WithPreferCSSPageSize(preferCSSPageSize).
				WithDisplayHeaderFooter(displayHeaderFooter).
				WithGenerateTaggedPDF(generateTaggedPDF).
				WithGenerateDocumentOutline(generateDocumentOutline)

			if pageRanges != "" {
				p = p.WithPageRanges(pageRanges)
			}
			if headerTemplate != "" {
				p = p.WithHeaderTemplate(headerTemplate)
			}
			if footerTemplate != "" {
				p = p.WithFooterTemplate(footerTemplate)
			}

			buf, _, err = p.Do(ctx)
			return err
		}),
	); err != nil {
		web.Error(w, 500, fmt.Errorf("pdf: %w", err))
		return
	}

	if output == "file" {
		savePath := r.URL.Query().Get("path")
		if savePath == "" {
			pdfDir := filepath.Join(h.Config.StateDir, "pdfs")
			if err := os.MkdirAll(pdfDir, 0750); err != nil {
				web.Error(w, 500, fmt.Errorf("create pdf dir: %w", err))
				return
			}
			timestamp := time.Now().Format("20060102-150405")
			savePath = filepath.Join(pdfDir, fmt.Sprintf("page-%s.pdf", timestamp))
		} else {
			safe, err := web.SafePath(h.Config.StateDir, savePath)
			if err != nil {
				web.Error(w, 400, fmt.Errorf("invalid path: %w", err))
				return
			}
			savePath = safe
			if err := os.MkdirAll(filepath.Dir(savePath), 0750); err != nil {
				web.Error(w, 500, fmt.Errorf("create dir: %w", err))
				return
			}
		}

		if err := os.WriteFile(savePath, buf, 0600); err != nil {
			web.Error(w, 500, fmt.Errorf("write pdf: %w", err))
			return
		}

		web.JSON(w, 200, map[string]any{
			"path": savePath,
			"size": len(buf),
		})
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", "application/pdf")
		if _, err := w.Write(buf); err != nil {
			slog.Error("pdf write", "err", err)
		}
		return
	}

	web.JSON(w, 200, map[string]any{
		"format": "pdf",
		"base64": base64.StdEncoding.EncodeToString(buf),
	})
}

// HandleTabPDF generates a PDF for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/pdf
// @Endpoint POST /tabs/{id}/pdf
func (h *Handlers) HandleTabPDF(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	q := r.URL.Query()
	if r.Method == http.MethodPost {
		var body map[string]any
		if r.ContentLength > 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&body); err != nil {
				web.Error(w, 400, fmt.Errorf("decode: %w", err))
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
					web.Error(w, 400, fmt.Errorf("invalid %s type", key))
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
