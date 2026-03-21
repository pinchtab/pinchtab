package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestValidatePDFTemplate_RejectsActiveContent(t *testing.T) {
	tests := []string{
		`<script>alert(1)</script>`,
		`<span onclick="alert(1)">x</span>`,
		`<a href="javascript:alert(1)">x</a>`,
	}

	for _, template := range tests {
		if err := validatePDFTemplate(template); err == nil {
			t.Fatalf("validatePDFTemplate(%q) should reject active content", template)
		}
	}
}

func TestValidatePDFTemplate_AllowsPlaceholderMarkup(t *testing.T) {
	template := `<span class="pageNumber"></span> / <span class="totalPages"></span>`
	if err := validatePDFTemplate(template); err != nil {
		t.Fatalf("validatePDFTemplate() error = %v", err)
	}
}

func TestHandlePDF_InvalidTemplateRejectedBeforeTabLookup(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", `/pdf?headerTemplate=<script>alert(1)</script>`, nil)
	w := httptest.NewRecorder()
	h.HandlePDF(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabPDF_MissingTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//pdf", nil)
	w := httptest.NewRecorder()
	h.HandleTabPDF(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabPDF_NoTab_WithTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/nonexistent/pdf", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	h.HandleTabPDF(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
