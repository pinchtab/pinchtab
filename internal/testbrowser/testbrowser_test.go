package testbrowser

import (
	"slices"
	"strings"
	"testing"
)

func TestFindHonoursTheBinaryOverride(t *testing.T) {
	t.Setenv(BinaryEnv, "/opt/custom/chrome")

	binary, probed := Find()
	if binary != "/opt/custom/chrome" {
		t.Fatalf("binary = %q, want the override", binary)
	}
	if len(probed) != 1 || !strings.Contains(probed[0], BinaryEnv) {
		t.Fatalf("probed = %v, want the override to be named as the source", probed)
	}
}

// A real absence must be actionable: the message has to say what was searched
// and how to point the tests at a binary, not "chromium not installed".
func TestNotFoundMessageNamesTheSearchAndTheOverride(t *testing.T) {
	msg := notFoundMessage([]string{"/usr/bin/chromium", "/Applications/Google Chrome.app"})

	for _, want := range []string{"/usr/bin/chromium", "/Applications/Google Chrome.app", BinaryEnv, "chrome"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not mention %q: %s", want, msg)
		}
	}
}

func TestNotFoundMessageWithoutProbedPaths(t *testing.T) {
	msg := notFoundMessage(nil)
	if !strings.Contains(msg, BinaryEnv) {
		t.Errorf("message does not name the override: %s", msg)
	}
}

func TestSkipRequested(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "", want: false},
		{value: "0", want: false},
		{value: "false", want: false},
		{value: "1", want: true},
		{value: "yes", want: true},
	}
	for _, tt := range tests {
		t.Run("value="+tt.value, func(t *testing.T) {
			t.Setenv(SkipEnv, tt.value)
			if got := skipRequested(); got != tt.want {
				t.Fatalf("skipRequested() = %v, want %v", got, tt.want)
			}
		})
	}
}

// chrome is tried first: these tests assert plain Chromium behaviour, and every
// registered provider must appear so the search is exhaustive.
func TestProviderIDsPrefersChromeAndCoversTheRegistry(t *testing.T) {
	ids := providerIDs()
	if len(ids) == 0 || ids[0] != "chrome" {
		t.Fatalf("providerIDs() = %v, want chrome first", ids)
	}
	for _, want := range []string{"chrome", "cloak", "ghost-chrome"} {
		found := slices.Contains(ids, want)
		if !found {
			t.Errorf("providerIDs() = %v, missing %q", ids, want)
		}
	}
}
