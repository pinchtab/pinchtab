package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleSecurityCommandDefaultConfigSkipsEmptySections(t *testing.T) {
	cfg := testRuntimeConfig()

	output := captureStdout(t, func() {
		handleSecurityCommand(cfg)
	})

	required := []string{
		"Security",
		"All recommended security defaults are active.",
	}
	for _, needle := range required {
		if !strings.Contains(output, needle) {
			t.Fatalf("expected output to contain %q\n%s", needle, output)
		}
	}

	unwanted := []string{
		"Security posture",
		"Warnings",
		"Recommended security defaults",
		"Recommended defaults",
		"Restore recommended security defaults in config?",
		"Interactive restore skipped because stdin/stdout is not a terminal.",
	}
	for _, needle := range unwanted {
		if strings.Contains(output, needle) {
			t.Fatalf("expected output to skip %q\n%s", needle, output)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = orig
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer error = %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader error = %v", err)
	}
	return string(data)
}

func testRuntimeConfig() *config.RuntimeConfig {
	return &config.RuntimeConfig{
		Bind:               "127.0.0.1",
		Token:              "abcd1234efgh5678",
		AllowEvaluate:      false,
		AllowMacro:         false,
		AllowScreencast:    false,
		AllowDownload:      false,
		AllowUpload:        false,
		AttachEnabled:      false,
		AttachAllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
		AttachAllowSchemes: []string{"ws", "wss"},
		IDPI: config.IDPIConfig{
			Enabled:        true,
			AllowedDomains: []string{"127.0.0.1", "localhost", "::1"},
			StrictMode:     true,
			ScanContent:    true,
			WrapContent:    true,
		},
	}
}
