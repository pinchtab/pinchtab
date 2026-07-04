package report

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildPDFHTMLEmbedsScreenshots(t *testing.T) {
	r := sampleReport()
	r.Pages[0].Screenshot = "aW1hZ2U="
	r.Pages[0].Browser.ScreenshotPath = "screenshots/page-001.png"

	html, err := BuildPDFHTML(r)
	if err != nil {
		t.Fatalf("BuildPDFHTML: %v", err)
	}
	if !strings.Contains(string(html), `src="data:image/png;base64,aW1hZ2U="`) {
		t.Error("screenshot should be embedded as a data URI")
	}
	if r.Pages[0].Browser.ScreenshotPath != "screenshots/page-001.png" {
		t.Error("BuildPDFHTML must not mutate the input report")
	}
}

func TestExportPDFSuccess(t *testing.T) {
	var printed []byte
	pdf, err := ExportPDF(sampleReport(), func(html []byte) ([]byte, error) {
		printed = html
		return []byte("%PDF-1.7 fake"), nil
	})
	if err != nil {
		t.Fatalf("ExportPDF: %v", err)
	}
	if len(pdf) == 0 || !strings.HasPrefix(string(pdf), "%PDF") {
		t.Errorf("pdf = %q", pdf)
	}
	if !strings.Contains(string(printed), "<h1>Site Audit Report</h1>") {
		t.Error("printer should receive the rendered HTML report")
	}
}

func TestExportPDFFailurePaths(t *testing.T) {
	if _, err := ExportPDF(sampleReport(), nil); err == nil {
		t.Error("nil printer should error")
	}

	wantErr := errors.New("evaluate capability disabled")
	if _, err := ExportPDF(sampleReport(), func([]byte) ([]byte, error) { return nil, wantErr }); !errors.Is(err, wantErr) {
		t.Errorf("printer error = %v, want %v", err, wantErr)
	}

	if _, err := ExportPDF(sampleReport(), func([]byte) ([]byte, error) { return []byte("<html>not a pdf"), nil }); err == nil {
		t.Error("non-PDF bytes should error")
	} else if !strings.Contains(err.Error(), "not a PDF") {
		t.Errorf("magic-bytes error should be clear, got %v", err)
	}
}
