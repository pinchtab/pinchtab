package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestScreenshot(t *testing.T) {
	m := newMockServer()
	m.response = "FAKEJPEGDATA"
	defer m.close()
	client := m.server.Client()

	outFile := filepath.Join(t.TempDir(), "test.jpg")
	cmd := &cobra.Command{}
	cmd.Flags().String("output", outFile, "")
	cmd.Flags().String("quality", "50", "")
	cmd.Flags().String("selector", "#target", "")
	cmd.Flags().Bool("css-1x", true, "")
	cmd.Flags().String("tab", "", "")
	Screenshot(client, m.base(), "", cmd)
	if m.lastPath != "/screenshot" {
		t.Errorf("expected /screenshot, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "quality=50") {
		t.Errorf("expected quality=50, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "selector=%23target") {
		t.Errorf("expected selector query, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "css1x=true") {
		t.Errorf("expected css1x=true, got %s", m.lastQuery)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "FAKEJPEGDATA" {
		t.Errorf("unexpected content: %s", string(data))
	}
}
