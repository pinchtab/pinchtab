package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newPDFCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("landscape", false, "")
	cmd.Flags().String("scale", "", "")
	cmd.Flags().String("paper-width", "", "")
	cmd.Flags().String("paper-height", "", "")
	cmd.Flags().String("margin-top", "", "")
	cmd.Flags().String("margin-bottom", "", "")
	cmd.Flags().String("margin-left", "", "")
	cmd.Flags().String("margin-right", "", "")
	cmd.Flags().String("page-ranges", "", "")
	cmd.Flags().Bool("prefer-css-page-size", false, "")
	cmd.Flags().Bool("display-header-footer", false, "")
	cmd.Flags().String("header-template", "", "")
	cmd.Flags().String("footer-template", "", "")
	cmd.Flags().Bool("generate-tagged-pdf", false, "")
	cmd.Flags().Bool("generate-document-outline", false, "")
	cmd.Flags().Bool("file-output", false, "")
	cmd.Flags().String("path", "", "")
	return cmd
}

func TestPDF(t *testing.T) {
	m := newMockServer()
	m.response = "FAKEPDFDATA"
	defer m.close()
	client := m.server.Client()

	outFile := filepath.Join(t.TempDir(), "test.pdf")
	cmd := newPDFCmd()
	_ = cmd.Flags().Set("output", outFile)
	_ = cmd.Flags().Set("tab", "tab-abc")
	_ = cmd.Flags().Set("landscape", "true")
	_ = cmd.Flags().Set("scale", "0.8")
	PDF(apiclient.Conn{Client: client, Base: m.base()}, cmd)
	if m.lastPath != "/tabs/tab-abc/pdf" {
		t.Errorf("expected /tabs/tab-abc/pdf, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "landscape=true") {
		t.Errorf("expected landscape=true, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "scale=0.8") {
		t.Errorf("expected scale=0.8, got %s", m.lastQuery)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "FAKEPDFDATA" {
		t.Errorf("unexpected content: %s", string(data))
	}
}

func TestPDFAllOptions(t *testing.T) {
	m := newMockServer()
	m.response = "FAKEPDFDATA"
	defer m.close()
	client := m.server.Client()

	outFile := filepath.Join(t.TempDir(), "test.pdf")
	cmd := newPDFCmd()
	_ = cmd.Flags().Set("output", outFile)
	_ = cmd.Flags().Set("tab", "tab-123")
	_ = cmd.Flags().Set("landscape", "true")
	_ = cmd.Flags().Set("scale", "1.5")
	_ = cmd.Flags().Set("paper-width", "11")
	_ = cmd.Flags().Set("paper-height", "8.5")
	_ = cmd.Flags().Set("margin-top", "1")
	_ = cmd.Flags().Set("margin-bottom", "1")
	_ = cmd.Flags().Set("margin-left", "0.5")
	_ = cmd.Flags().Set("margin-right", "0.5")
	_ = cmd.Flags().Set("page-ranges", "1-3,5")
	_ = cmd.Flags().Set("prefer-css-page-size", "true")
	_ = cmd.Flags().Set("display-header-footer", "true")
	_ = cmd.Flags().Set("header-template", "<span class='title'></span>")
	_ = cmd.Flags().Set("footer-template", "<span class='pageNumber'></span>")
	_ = cmd.Flags().Set("generate-tagged-pdf", "true")
	_ = cmd.Flags().Set("generate-document-outline", "true")

	PDF(apiclient.Conn{Client: client, Base: m.base()}, cmd)
	if m.lastPath != "/tabs/tab-123/pdf" {
		t.Errorf("expected /tabs/tab-123/pdf, got %s", m.lastPath)
	}

	expectedParams := []string{
		"landscape=true",
		"scale=1.5",
		"paperWidth=11",
		"paperHeight=8.5",
		"marginTop=1",
		"marginBottom=1",
		"marginLeft=0.5",
		"marginRight=0.5",
		"preferCSSPageSize=true",
		"displayHeaderFooter=true",
		"generateTaggedPDF=true",
		"generateDocumentOutline=true",
		"raw=true",
	}

	for _, expected := range expectedParams {
		if !strings.Contains(m.lastQuery, expected) {
			t.Errorf("expected %s in query, got %s", expected, m.lastQuery)
		}
	}
}
