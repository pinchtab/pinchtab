package actions

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadTruncatesPrintedBase64(t *testing.T) {
	payload := strings.Repeat("A", 400)
	m := newMockServer()
	m.response = `{"data":"` + payload + `","contentType":"image/png","size":123}`
	defer m.close()

	output := captureStdout(t, func() {
		Download(m.server.Client(), m.base(), "", []string{"https://example.com/image.png"}, "")
	})

	if !strings.Contains(output, `"dataTruncated": true`) {
		t.Fatalf("expected output to mark truncated data, got %q", output)
	}
	if !strings.Contains(output, `"dataLength": 400`) {
		t.Fatalf("expected output to include original data length, got %q", output)
	}
	if strings.Contains(output, `"`+payload+`"`) {
		t.Fatalf("expected output to avoid printing the full payload")
	}
	if !strings.Contains(output, "... (truncated)") {
		t.Fatalf("expected output to include truncation suffix, got %q", output)
	}
}

func TestDownloadWithOutputSavesDecodedFileAndTruncatesPrintedBase64(t *testing.T) {
	data := []byte(strings.Repeat("image-bytes-", 40))
	encoded := base64.StdEncoding.EncodeToString(data)

	m := newMockServer()
	m.response = `{"data":"` + encoded + `","contentType":"image/png","size":` + `480` + `}`
	defer m.close()

	outFile := filepath.Join(t.TempDir(), "image.bin")
	output := captureStdout(t, func() {
		Download(m.server.Client(), m.base(), "", []string{"https://example.com/image.png"}, outFile)
	})

	written, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("expected output file to be written: %v", err)
	}
	if string(written) != string(data) {
		t.Fatalf("unexpected file content")
	}
	if !strings.Contains(output, `"dataTruncated": true`) {
		t.Fatalf("expected truncated preview in output, got %q", output)
	}
	if !strings.Contains(output, "Saved "+outFile) {
		t.Fatalf("expected save confirmation, got %q", output)
	}
	if strings.Contains(output, `"`+encoded+`"`) {
		t.Fatalf("expected output to avoid printing the full base64 payload")
	}
}
