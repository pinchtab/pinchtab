package report

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/pinchtab/pinchtab/internal/audit"
)

// FormatPDF selects PDF export on the audit/compare CLIs. Rendering goes
// through a PDFPrinter (a browser tab printing the HTML report), so it is
// wired by the caller rather than Render.
const FormatPDF = "pdf"

// PDFPrinter renders self-contained HTML in a browser and returns the PDF
// bytes. The CLI wires this through the running pinchtab server: a fresh
// tab, content injected via the evaluate capability, printed with GET /pdf.
type PDFPrinter func(html []byte) ([]byte, error)

var pdfMagic = []byte("%PDF")

// BuildPDFHTML renders the report HTML for printing. Inline screenshots are
// embedded as data: URIs so the printed document needs no filesystem or
// network access. The input report is not mutated.
func BuildPDFHTML(r audit.AuditReport) ([]byte, error) {
	pages := make([]audit.PageResult, len(r.Pages))
	copy(pages, r.Pages)
	for i := range pages {
		if pages[i].Screenshot != "" {
			pages[i].Browser.ScreenshotPath = "data:image/png;base64," + pages[i].Screenshot
		}
	}
	r.Pages = pages
	return Render(r, FormatHTML)
}

// ExportPDF builds the printable HTML report and prints it through printer.
// Graceful-degradation contract (documented on the CLI help): callers write
// report.json before attempting the PDF, surface any error here as a
// warning, and exit non-zero only when pdf was the sole requested format.
func ExportPDF(r audit.AuditReport, printer PDFPrinter) ([]byte, error) {
	if printer == nil {
		return nil, errors.New("no PDF printer available")
	}
	html, err := BuildPDFHTML(r)
	if err != nil {
		return nil, err
	}
	pdf, err := printer(html)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(pdf, pdfMagic) {
		return nil, fmt.Errorf("printer returned %d bytes that are not a PDF document", len(pdf))
	}
	return pdf, nil
}
