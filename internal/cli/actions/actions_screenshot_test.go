package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
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
	cmd.Flags().String("scale", "0.5", "")
	cmd.Flags().String("tab", "", "")
	Screenshot(apiclient.Conn{Client: client, Base: m.base()}, cmd)
	if m.lastPath != "/screenshot" {
		t.Errorf("expected /screenshot, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "quality=50") {
		t.Errorf("expected quality=50, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "selector=%23target") {
		t.Errorf("expected selector query, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "scale=0.5") {
		t.Errorf("expected scale=0.5, got %s", m.lastQuery)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "FAKEJPEGDATA" {
		t.Errorf("unexpected content: %s", string(data))
	}
}

func TestScreenshotInfersPNGFromOutputExtension(t *testing.T) {
	m := newMockServer()
	m.response = "FAKEPNGDATA"
	defer m.close()

	outFile := filepath.Join(t.TempDir(), "screenshot.png")
	cmd := &cobra.Command{}
	cmd.Flags().String("output", outFile, "")
	cmd.Flags().String("format", "", "")
	Screenshot(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, cmd)

	if !strings.Contains(m.lastQuery, "format=png") {
		t.Fatalf("query = %q, want format=png inferred from output extension", m.lastQuery)
	}
}

func TestScreenshotAutoNamedLandsInTempDirWithAbsolutePath(t *testing.T) {
	m := newMockServer()
	defer m.close()
	m.response = "rawimagebytes"
	assertAutoNamedOutsideCwd(t, "screenshot-", ".jpg", func() string {
		return captureStdout(t, func() { Screenshot(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, screenshotCmd()) })
	})
}
