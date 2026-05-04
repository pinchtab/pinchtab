package main

import (
	"encoding/json"
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
