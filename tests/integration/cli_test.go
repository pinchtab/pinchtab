//go:build integration

package integration

import (
	"os/exec"
	"strings"
	"testing"
)

// TestCLI_Find runs `pinchtab find` via CLI against the test server.
func TestCLI_Find(t *testing.T) {
	// Navigate first so there's a page to search.
	navigate(t, "https://example.com")

	bin := pinchtabBinary(t)
	cmd := exec.Command(bin, "find", "More information", "--top", "5")
	cmd.Env = append(cmd.Environ(),
		"PINCHTAB_URL="+serverURL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pinchtab find failed: %v\n%s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "e") {
		t.Errorf("expected ref in output, got: %s", output)
	}
	// example.com has "Learn more" or "More information" depending on version.
	lower := strings.ToLower(output)
	if !strings.Contains(lower, "more") && !strings.Contains(lower, "learn") {
		t.Errorf("expected match containing 'more' or 'learn', got: %s", output)
	}

	closeCurrentTab(t)
}

// TestCLI_Snap runs `pinchtab snap` via CLI after navigating.
func TestCLI_Snap(t *testing.T) {
	navigate(t, "https://example.com")

	bin := pinchtabBinary(t)
	cmd := exec.Command(bin, "snap", "-i")
	cmd.Env = append(cmd.Environ(),
		"PINCHTAB_URL="+serverURL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pinchtab snap failed: %v\n%s", err, string(out))
	}

	output := string(out)
	if len(output) < 50 {
		t.Errorf("snapshot output too short: %d bytes", len(output))
	}

	closeCurrentTab(t)
}

// TestCLI_Text runs `pinchtab text` via CLI after navigating.
func TestCLI_Text(t *testing.T) {
	navigate(t, "https://example.com")

	bin := pinchtabBinary(t)
	cmd := exec.Command(bin, "text")
	cmd.Env = append(cmd.Environ(),
		"PINCHTAB_URL="+serverURL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pinchtab text failed: %v\n%s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(strings.ToLower(output), "example") {
		t.Errorf("expected 'example' in text output, got: %s", output)
	}

	closeCurrentTab(t)
}

// TestCLI_Nav runs `pinchtab nav <url>` via CLI.
func TestCLI_Nav(t *testing.T) {
	bin := pinchtabBinary(t)
	cmd := exec.Command(bin, "nav", "https://example.com")
	cmd.Env = append(cmd.Environ(),
		"PINCHTAB_URL="+serverURL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pinchtab nav failed: %v\n%s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "tab_") {
		t.Errorf("expected tab ID in output, got: %s", output)
	}

	closeCurrentTab(t)
}

// pinchtabBinary returns the path to the test binary built by TestMain.
func pinchtabBinary(t *testing.T) string {
	t.Helper()
	const bin = "/tmp/pinchtab-test"
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("pinchtab test binary not found at " + bin)
	}
	return bin
}
