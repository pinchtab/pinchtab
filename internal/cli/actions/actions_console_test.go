package actions

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func TestSanitizeTerminalText(t *testing.T) {
	input := "hello\x1b[31mred\x1b[0m\r\nnext\tline\a"
	got := sanitizeTerminalText(input)
	want := "hello[31mred[0m\\r\\nnext\\tline"
	if got != want {
		t.Fatalf("sanitizeTerminalText() = %q, want %q", got, want)
	}
}

func TestPrintConsoleLogs_SanitizesTerminalOutput(t *testing.T) {
	output := captureStdout(t, func() {
		printConsoleLogs([]byte(`{"tabId":"tab1","console":[{"timestamp":"2026-03-19T12:00:00Z","level":"log","message":"hello\u001b[31m\r\nworld\t\u0007"}]}`))
	})

	if strings.ContainsRune(output, '\x1b') {
		t.Fatalf("expected output to strip escape characters, got %q", output)
	}
	if !strings.Contains(output, "hello[31m\\r\\nworld\\t") {
		t.Fatalf("expected sanitized output, got %q", output)
	}
}

func TestPrintErrorLogs_SanitizesTerminalOutput(t *testing.T) {
	output := captureStdout(t, func() {
		printErrorLogs([]byte(`{"tabId":"tab1","errors":[{"timestamp":"2026-03-19T12:00:00Z","message":"boom\u001b[2J","url":"https://example.com/\u001b]52;c;secret\u0007","line":1,"column":2}]}`))
	})

	if strings.ContainsRune(output, '\x1b') {
		t.Fatalf("expected output to strip escape characters, got %q", output)
	}
	if !strings.Contains(output, "boom[2J") {
		t.Fatalf("expected sanitized message, got %q", output)
	}
	if !strings.Contains(output, "https://example.com/]52;c;secret") {
		t.Fatalf("expected sanitized url, got %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = writer

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, reader)
		_ = reader.Close()
		done <- err
	}()

	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	_ = writer.Close()
	if err := <-done; err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String()
}

func logCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "log"}
	cmd.Flags().Bool("clear", false, "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("limit", "", "")
	if err := cmd.Flags().Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	return cmd
}

func TestConsoleAndErrors_JSONPrintsBodyOnce(t *testing.T) {
	bodies := map[string]string{
		"/console": `{"tabId":"tab1","console":[{"timestamp":"2026-03-19T12:00:00Z","level":"warning","message":"careful","source":"console-api"}]}`,
		"/errors":  `{"tabId":"tab1","errors":[{"timestamp":"2026-03-19T12:00:00Z","message":"boom","url":"https://example.com/a.js","line":2,"column":23,"stack":"ReferenceError: boom"}]}`,
	}
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, bodies[r.URL.Path])
	}))
	defer srv.Close()

	run := map[string]func(apiclient.Conn, *cobra.Command){"/console": Console, "/errors": Errors}
	for path, action := range run {
		hits = nil
		output := captureStdout(t, func() {
			action(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, logCommand(t, "--json", "--tab", "tab1", "--limit", "5"))
		})
		var got, want map[string]any
		if err := json.Unmarshal([]byte(output), &got); err != nil {
			t.Fatalf("%s --json output is not one JSON document: %v\n%s", path, err, output)
		}
		_ = json.Unmarshal([]byte(bodies[path]), &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s --json printed %v, want the response body %v", path, got, want)
		}
		if len(hits) != 1 || !strings.Contains(hits[0], "tabId=tab1") || !strings.Contains(hits[0], "limit=5") {
			t.Errorf("%s --json requests = %v, want one carrying tab and limit", path, hits)
		}
	}

	terse := captureStdout(t, func() {
		Errors(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, logCommand(t))
	})
	if strings.Contains(terse, "{") || !strings.Contains(terse, "12:00:00 [ERROR] boom") {
		t.Errorf("terse errors output changed: %q", terse)
	}
}
