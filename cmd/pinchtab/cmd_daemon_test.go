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

func TestCollectDaemonStatusInvariants(t *testing.T) {
	st := collectDaemonStatus()

	// PID and ServicePath are only meaningful when running/installed; they must
	// stay empty otherwise so the renderers don't print stale rows.
	if !st.Running && st.PID != "" {
		t.Fatalf("PID should be empty when not running, got %q", st.PID)
	}
	if !st.Installed && st.ServicePath != "" {
		t.Fatalf("ServicePath should be empty when not installed, got %q", st.ServicePath)
	}
	// A manager error short-circuits collection before any probing fields are set.
	if st.ManagerError != "" {
		if st.PID != "" || st.ServicePath != "" || st.PreflightError != "" {
			t.Fatalf("expected no probe fields when ManagerError set, got %+v", st)
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
