package bridge

import (
	"context"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

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
