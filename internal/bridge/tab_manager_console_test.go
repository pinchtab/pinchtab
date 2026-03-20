package bridge

import (
	"testing"

	"github.com/chromedp/cdproto/runtime"
)

func TestStackTraceSourceUsesFirstFrameURL(t *testing.T) {
	trace := &runtime.StackTrace{
		CallFrames: []*runtime.CallFrame{
			{URL: ""},
			{URL: "https://example.com/app.js"},
		},
	}

	if got := stackTraceSource(trace); got != "https://example.com/app.js" {
		t.Fatalf("stackTraceSource() = %q, want %q", got, "https://example.com/app.js")
	}
}

func TestStackTraceSourceFallsBackToParentTrace(t *testing.T) {
	trace := &runtime.StackTrace{
		CallFrames: []*runtime.CallFrame{{URL: ""}},
		Parent: &runtime.StackTrace{
			CallFrames: []*runtime.CallFrame{{URL: "https://example.com/parent.js"}},
		},
	}

	if got := stackTraceSource(trace); got != "https://example.com/parent.js" {
		t.Fatalf("stackTraceSource() = %q, want %q", got, "https://example.com/parent.js")
	}
}

func TestExecutionContextSourcePrefersOrigin(t *testing.T) {
	ctx := &runtime.ExecutionContextDescription{
		Origin: "https://example.com",
		Name:   "isolated-world",
	}

	if got := executionContextSource(ctx); got != "https://example.com" {
		t.Fatalf("executionContextSource() = %q, want %q", got, "https://example.com")
	}
}

func TestExceptionSourcePrefersURL(t *testing.T) {
	details := &runtime.ExceptionDetails{
		URL: "https://example.com/app.js",
		StackTrace: &runtime.StackTrace{
			CallFrames: []*runtime.CallFrame{{URL: "https://example.com/fallback.js"}},
		},
	}

	if got := exceptionSource(details); got != "https://example.com/app.js" {
		t.Fatalf("exceptionSource() = %q, want %q", got, "https://example.com/app.js")
	}
}

func TestIsInternalConsoleSource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{source: "chrome-extension://abc123/content.js", want: true},
		{source: "devtools://devtools/bundled/entrypoints/main/main.js", want: true},
		{source: "about:blank", want: true},
		{source: "https://example.com/app.js", want: false},
		{source: "", want: false},
	}

	for _, tt := range tests {
		if got := isInternalConsoleSource(tt.source); got != tt.want {
			t.Fatalf("isInternalConsoleSource(%q) = %v, want %v", tt.source, got, tt.want)
		}
	}
}
