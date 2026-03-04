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

// TestCLI_Open runs `pinchtab open <url>` then `pinchtab close <id>`.
func TestCLI_Open(t *testing.T) {
	bin := pinchtabBinary(t)
	cmd := exec.Command(bin, "open", "https://example.com")
	cmd.Env = append(cmd.Environ(),
		"PINCHTAB_URL="+serverURL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pinchtab open failed: %v\n%s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "tab_") {
		t.Fatalf("expected tab ID in output, got: %s", output)
	}

	// Extract tab ID from "📑 Opened [tab_abc123] → ..."
	tabID := extractTabID(output)
	if tabID == "" {
		t.Fatalf("could not extract tab ID from: %s", output)
	}

	// Close it
	cmd = exec.Command(bin, "close", tabID)
	cmd.Env = append(cmd.Environ(),
		"PINCHTAB_URL="+serverURL,
	)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pinchtab close failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "Closed") {
		t.Errorf("expected 'Closed' in output, got: %s", string(out))
	}
}

// TestCLI_Tabs runs `pinchtab tabs` via CLI.
func TestCLI_Tabs(t *testing.T) {
	navigate(t, "https://example.com")

	bin := pinchtabBinary(t)
	cmd := exec.Command(bin, "tabs")
	cmd.Env = append(cmd.Environ(),
		"PINCHTAB_URL="+serverURL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pinchtab tabs failed: %v\n%s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "Tabs") && !strings.Contains(output, "tab") {
		t.Errorf("expected tabs listing, got: %s", output)
	}
	// Should show example.com somewhere in the output
	if !strings.Contains(strings.ToLower(output), "example") {
		t.Errorf("expected 'example' in tabs output, got: %s", output)
	}

	closeCurrentTab(t)
}

// TestDeleteTab verifies DELETE /tabs/{id} closes a tab.
func TestDeleteTab(t *testing.T) {
	navigate(t, "https://example.com")

	tabID := currentTabID
	if tabID == "" {
		t.Skip("no current tab ID")
	}

	code, body := httpDelete(t, "/tabs/"+tabID)
	if code != 200 {
		t.Fatalf("DELETE /tabs/%s: expected 200, got %d: %s", tabID, code, string(body))
	}

	currentTabID = ""
}

func extractTabID(s string) string {
	// Look for tab_XXXXXXXX pattern
	idx := strings.Index(s, "tab_")
	if idx < 0 {
		return ""
	}
	end := idx + 4
	for end < len(s) && s[end] != ']' && s[end] != ' ' && s[end] != '\n' {
		end++
	}
	return s[idx:end]
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
