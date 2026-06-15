package main

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestPrintDaemonStatusJSONShape(t *testing.T) {
	output := captureStdout(t, func() {
		printDaemonStatusJSON()
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}
	for _, key := range []string{"installed", "running"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("expected key %q in JSON output, got %s", key, output)
		}
	}
}

func TestPrintDaemonOverviewIncludesStatusAndHints(t *testing.T) {
	output := captureStdout(t, func() {
		printDaemonOverview()
	})

	if runtime.GOOS == "windows" {
		for _, needle := range []string{
			"Daemon",
			"supported on macOS and Linux",
			"current OS is windows",
		} {
			if !strings.Contains(output, needle) {
				t.Fatalf("expected output to contain %q\n%s", needle, output)
			}
		}
		return
	}

	for _, needle := range []string{
		"Daemon",
		"service",
		"state",
		"Manage daemon:",
		"pinchtab daemon --json",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("expected output to contain %q\n%s", needle, output)
		}
	}
}
