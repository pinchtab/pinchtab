package actions

import (
	"io"
	"os"
	"strings"
	"testing"
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

	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	_ = writer.Close()
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	_ = reader.Close()
	return string(out)
}
