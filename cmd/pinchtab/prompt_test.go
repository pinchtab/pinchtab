package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestPromptInputHiddenDefaultKeepsDefaultWithoutPrintingIt(t *testing.T) {
	origStdin := os.Stdin
	origStdout := os.Stdout
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(stdin) error = %v", err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(stdout) error = %v", err)
	}

	os.Stdin = stdinR
	os.Stdout = stdoutW
	t.Cleanup(func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
		_ = stdinR.Close()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
	})

	go func() {
		_, _ = stdinW.WriteString("\n")
		_ = stdinW.Close()
	}()

	value, err := promptInputHiddenDefault("Set server.token:", "very-secret-token-value")
	if err != nil {
		t.Fatalf("promptInputHiddenDefault() error = %v", err)
	}
	if err := stdoutW.Close(); err != nil {
		t.Fatalf("close stdout writer error = %v", err)
	}
	output, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if value != "very-secret-token-value" {
		t.Fatalf("value = %q, want default", value)
	}
	if strings.Contains(string(output), "very-secret-token-value") {
		t.Fatalf("expected hidden default to stay out of prompt, got %q", string(output))
	}
}
