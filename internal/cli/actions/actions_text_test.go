package actions

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newTextCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("raw", false, "")
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().Bool("markdown", false, "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("frame", "", "")
	cmd.Flags().String("selector", "", "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func TestText(t *testing.T) {
	m := newMockServer()
	m.response = `{"url":"https://pinchtab.com","title":"Example","text":"Hello"}`
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	Text(apiclient.Conn{Client: client, Base: m.base()}, cmd, nil)
	if m.lastPath != "/text" {
		t.Errorf("expected /text, got %s", m.lastPath)
	}
}

func TestTextRaw(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	_ = cmd.Flags().Set("raw", "true")
	Text(apiclient.Conn{Client: client, Base: m.base()}, cmd, nil)
	if !strings.Contains(m.lastQuery, "mode=raw") {
		t.Errorf("expected mode=raw, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "format=text") {
		t.Errorf("expected format=text, got %s", m.lastQuery)
	}
}

func TestTextFull(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	_ = cmd.Flags().Set("full", "true")
	Text(apiclient.Conn{Client: client, Base: m.base()}, cmd, nil)
	if !strings.Contains(m.lastQuery, "mode=raw") {
		t.Errorf("expected --full to set mode=raw, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "format=text") {
		t.Errorf("expected --full to set format=text, got %s", m.lastQuery)
	}
}

func TestTextMarkdown(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	_ = cmd.Flags().Set("markdown", "true")
	Text(apiclient.Conn{Client: client, Base: m.base()}, cmd, nil)
	if !strings.Contains(m.lastQuery, "mode=markdown") {
		t.Errorf("expected mode=markdown, got %s", m.lastQuery)
	}
	if strings.Contains(m.lastQuery, "format=text") {
		t.Errorf("markdown must not force format=text (the JSON envelope carries .text): %s", m.lastQuery)
	}
}

func TestTextMarkdownOutputWritesFileAndPrintsConfirmation(t *testing.T) {
	m := newMockServer()
	m.response = `{"url":"https://pinchtab.com","title":"Example","text":"# Title\n\nBody"}`
	defer m.close()

	out := t.TempDir() + "/page.md"
	cmd := newTextCmd()
	_ = cmd.Flags().Set("markdown", "true")
	_ = cmd.Flags().Set("output", out)

	stdout := captureStdout(t, func() {
		Text(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, cmd, nil)
	})

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if string(data) != "# Title\n\nBody" {
		t.Errorf("file body = %q, want the Markdown text", string(data))
	}
	if strings.Contains(stdout, "# Title") {
		t.Errorf("stdout carried the body instead of a one-line confirmation: %q", stdout)
	}
	if !strings.Contains(stdout, out) {
		t.Errorf("stdout = %q, want a one-line confirmation naming %s", stdout, out)
	}
}

func TestTextFrame(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	_ = cmd.Flags().Set("frame", "FRAME123")
	Text(apiclient.Conn{Client: client, Base: m.base()}, cmd, nil)
	if !strings.Contains(m.lastQuery, "frameId=FRAME123") {
		t.Errorf("expected frameId=FRAME123, got %s", m.lastQuery)
	}
}

func TestTextTab(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	_ = cmd.Flags().Set("tab", "TAB1")
	Text(apiclient.Conn{Client: client, Base: m.base()}, cmd, nil)
	if !strings.Contains(m.lastQuery, "tabId=TAB1") {
		t.Errorf("expected tabId=TAB1, got %s", m.lastQuery)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = writer

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, reader)
		_ = reader.Close()
		done <- copyErr
	}()

	defer func() { os.Stderr = oldStderr }()

	fn()

	_ = writer.Close()
	if err := <-done; err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String()
}

func runTextAgainst(t *testing.T, response string) (stdout, stderr string) {
	t.Helper()
	m := newMockServer()
	m.response = response
	defer m.close()

	stderr = captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			Text(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, newTextCmd(), nil)
		})
	})
	return stdout, stderr
}

func TestTextFallbackNotesOnStderrAndKeepsStdoutClean(t *testing.T) {
	page := "PinchTab — browser control for AI agents\nInstall: curl -fsSL https://pinchtab.com/install.sh | sh"
	body, err := json.Marshal(map[string]any{
		"text":       page,
		"extraction": "readability_fallback",
		"textLength": len(page),
		"rawLength":  len(page),
	})
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr := runTextAgainst(t, string(body))

	if stdout != page+"\n" {
		t.Errorf("stdout = %q, want the page text alone", stdout)
	}
	if !strings.Contains(stderr, "--full") {
		t.Errorf("stderr = %q, want a note pointing at --full", stderr)
	}
}

func TestTextNoNoteWhenReadabilityHeld(t *testing.T) {
	body := `{"text":"Article body","extraction":"readability","textLength":12,"rawLength":13}`

	stdout, stderr := runTextAgainst(t, body)

	if stdout != "Article body\n" {
		t.Errorf("stdout = %q, want the article text", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want no note when readability held", stderr)
	}
}
