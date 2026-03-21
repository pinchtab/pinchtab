package safelog

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestHandlerRedactsAndSanitizesStringAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(slog.NewJSONHandler(&buf, nil)))

	logger.Info("hello\x1b[31mworld", "token", "secret-token", "path", "/Users/tester/private.txt\x00")

	out := buf.String()
	if strings.Contains(out, "secret-token") {
		t.Fatalf("expected token to be redacted, got %q", out)
	}
	if strings.Contains(out, "\x1b") {
		t.Fatalf("expected ANSI escapes to be stripped, got %q", out)
	}
	if strings.Contains(out, "\x00") {
		t.Fatalf("expected null bytes to be stripped, got %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected redacted marker, got %q", out)
	}
}

func TestHandlerTruncatesOversizedStrings(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&buf, nil)))

	logger.Info("msg", "payload", strings.Repeat("x", MaxStringValueBytes+512))

	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected log output")
	}
	if strings.Contains(out, strings.Repeat("x", MaxStringValueBytes+128)) {
		t.Fatalf("expected oversized value to be truncated, got %q", out)
	}
}
