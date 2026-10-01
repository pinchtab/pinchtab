package actions

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestDownloadTruncatesPrintedBase64(t *testing.T) {
	payload := strings.Repeat("A", 400)
	m := newMockServer()
	m.response = `{"data":"` + payload + `","contentType":"image/png","size":123}`
	defer m.close()

	output := captureStdout(t, func() {
		Download(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, []string{"https://example.com/image.png"}, "", "")
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

func TestDownloadWithOutputConfirmsTheFileAndPrintsNoEnvelope(t *testing.T) {
	data := []byte(strings.Repeat("image-bytes-", 40))
	encoded := base64.StdEncoding.EncodeToString(data)

	m := newMockServer()
	m.response = `{"data":"` + encoded + `","contentType":"image/png","size":480}`
	defer m.close()

	outFile := filepath.Join(t.TempDir(), "image.bin")
	output := captureStdout(t, func() {
		Download(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, []string{"https://example.com/image.png"}, outFile, "")
	})

	written, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("expected output file to be written: %v", err)
	}
	if string(written) != string(data) {
		t.Fatal("the saved file is not byte-exact against the payload")
	}

	if !strings.Contains(output, "Saved "+outFile) {
		t.Errorf("output %q does not name the file it saved", output)
	}
	if !strings.Contains(output, fmt.Sprintf("(%d bytes)", len(data))) {
		t.Errorf("output %q does not carry the byte count", output)
	}
	for _, banned := range []string{`"data"`, `"dataTruncated"`, `"dataLength"`, `"contentType"`, "(truncated)"} {
		if strings.Contains(output, banned) {
			t.Errorf("output %q still carries the envelope field %s", output, banned)
		}
	}
	if strings.Contains(output, encoded[:downloadDataPreviewLimit]) {
		t.Errorf("output %q still carries base64 from the payload", output)
	}
	if lines := strings.Count(strings.TrimSpace(output), "\n"); lines != 0 {
		t.Errorf("output is %d lines, want the single confirmation:\n%s", lines+1, output)
	}
}

func TestTheSaveConfirmationIsTheSameSentenceEverywhere(t *testing.T) {
	dir := t.TempDir()

	download := func() string {
		payload := []byte("0123456789")
		m := newMockServer()
		defer m.close()
		m.response = `{"data":"` + base64.StdEncoding.EncodeToString(payload) + `"}`
		return captureStdout(t, func() {
			Download(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, []string{"https://example.com/f.bin"}, filepath.Join(dir, "d.bin"), "")
		})
	}
	screenshot := func() string {
		m := newMockServer()
		defer m.close()
		m.response = "0123456789"
		cmd := screenshotCmd()
		_ = cmd.Flags().Set("output", filepath.Join(dir, "s.jpg"))
		return captureStdout(t, func() { Screenshot(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, cmd) })
	}
	pdf := func() string {
		m := newMockServer()
		defer m.close()
		m.response = "0123456789"
		cmd := pdfCmd()
		_ = cmd.Flags().Set("output", filepath.Join(dir, "p.pdf"))
		return captureStdout(t, func() { PDF(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, cmd) })
	}

	want := "Saved " + filepath.Join(dir, "d.bin") + " (10 bytes)"
	if got := strings.TrimSpace(download()); got != want {
		t.Errorf("download printed %q, want %q", got, want)
	}
	for name, got := range map[string]string{
		"screenshot": strings.TrimSpace(screenshot()),
		"pdf":        strings.TrimSpace(pdf()),
	} {
		if !strings.HasPrefix(got, "Saved ") || !strings.HasSuffix(got, " (10 bytes)") {
			t.Errorf("%s printed %q, which is not the shared sentence", name, got)
		}
	}
}

func TestTheSaveConfirmationHasOneOwner(t *testing.T) {
	const sentence = `"Saved %s (%d bytes)"`

	var owner string
	var others []string
	for name, body := range srccensus.Load(t, ".", 40).Sources() {
		if !strings.Contains(body, sentence) {
			continue
		}
		if strings.Contains(body, "func printSaved(") {
			owner = name
			continue
		}
		others = append(others, name)
	}

	if owner == "" {
		t.Fatalf("no file declares printSaved alongside %s; the sentence has no owner, so this census guards nothing", sentence)
	}
	if len(others) > 0 {
		slices.Sort(others)
		t.Errorf("%s is open-coded in %v as well as its owner %s; route those through printSaved", sentence, others, owner)
	}
}

func TestDownloadTabScopedPath(t *testing.T) {
	m := newMockServer()
	m.response = `{"data":"","contentType":"text/plain","size":0}`
	defer m.close()

	captureStdout(t, func() {
		Download(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, []string{"https://example.com/f.txt"}, "", "tab1")
	})

	if m.lastPath != "/tabs/tab1/download" {
		t.Fatalf("expected /tabs/tab1/download, got %s", m.lastPath)
	}
}
