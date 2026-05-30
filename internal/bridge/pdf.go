package bridge

import (
	"context"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// PDFParams holds parameters for PDF generation.
type PDFParams struct {
	Landscape               bool
	PrintBackground         bool
	Scale                   float64
	PaperWidth              float64
	PaperHeight             float64
	MarginTop               float64
	MarginBottom            float64
	MarginLeft              float64
	MarginRight             float64
	PageRanges              string
	PreferCSSPageSize       bool
	DisplayHeaderFooter     bool
	GenerateTaggedPDF       bool
	GenerateDocumentOutline bool
	HeaderTemplate          string
	FooterTemplate          string
}

// PrintToPDF generates a PDF of the current page via CDP.
func (b *Bridge) PrintToPDF(ctx context.Context, params PDFParams) ([]byte, error) {
	var buf []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		p := page.PrintToPDF().
			WithPrintBackground(params.PrintBackground).
			WithScale(params.Scale).
			WithLandscape(params.Landscape).
			WithPaperWidth(params.PaperWidth).
			WithPaperHeight(params.PaperHeight).
			WithMarginTop(params.MarginTop).
			WithMarginBottom(params.MarginBottom).
			WithMarginLeft(params.MarginLeft).
			WithMarginRight(params.MarginRight).
			WithPreferCSSPageSize(params.PreferCSSPageSize).
			WithDisplayHeaderFooter(params.DisplayHeaderFooter).
			WithGenerateTaggedPDF(params.GenerateTaggedPDF).
			WithGenerateDocumentOutline(params.GenerateDocumentOutline)

		if params.PageRanges != "" {
			p = p.WithPageRanges(params.PageRanges)
		}
		if params.HeaderTemplate != "" {
			p = p.WithHeaderTemplate(params.HeaderTemplate)
		}
		if params.FooterTemplate != "" {
			p = p.WithFooterTemplate(params.FooterTemplate)
		}

		var err error
		buf, _, err = p.Do(ctx)
		return err
	}))
	return buf, err
}
