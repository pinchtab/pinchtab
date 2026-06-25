package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// The current-tab existence probe must be debounced: a freshly written/validated
// state file is trusted within tabProbeTTL so back-to-back commands skip the HTTP probe.
func TestTabStateRecentlyValidated(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path := tabStateFile()

	if tabStateRecentlyValidated() {
		t.Fatal("recentlyValidated true with no state file")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("tab_x\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !tabStateRecentlyValidated() {
		t.Fatal("recentlyValidated false right after write")
	}

	// Backdate past the TTL → stale, probe should run.
	old := time.Now().Add(-2 * tabProbeTTL)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if tabStateRecentlyValidated() {
		t.Fatal("recentlyValidated true after backdating past TTL")
	}

	// Touch (post-validation) → fresh again.
	touchTabStateFile()
	if !tabStateRecentlyValidated() {
		t.Fatal("recentlyValidated false after touch")
	}
}

// Guard for the registration refactor: every browser root command must be in
// the "browser" group, and the shared pointer-flag bundle (+ per-command extras)
// must survive the helper extraction.
func TestBrowserCommandRegistration(t *testing.T) {
	for _, c := range browserRootCommands() {
		if c.GroupID != "browser" {
			t.Errorf("command %q GroupID = %q, want %q", c.Name(), c.GroupID, "browser")
		}
	}

	for _, name := range []string{"css", "x", "y", "humanize", "wait-nav", "mode"} {
		if clickCmd.Flags().Lookup(name) == nil {
			t.Errorf("clickCmd missing flag %q", name)
		}
	}
	// Pointer commands keep their action-specific extras alongside the bundle.
	if mouseDownCmd.Flags().Lookup("button") == nil {
		t.Error("mouseDownCmd missing button flag")
	}
	for _, name := range []string{"css", "x", "y", "humanize"} {
		if hoverCmd.Flags().Lookup(name) == nil {
			t.Errorf("hoverCmd missing flag %q", name)
		}
	}
}

// TestPostActionFlagsBundle pins the exact usage strings the shared
// addPostActionFlags helper interpolates per verb, so a future verb edit cannot
// silently drift the --help text, and verifies the no-text commands omit --text.
func TestPostActionFlagsBundle(t *testing.T) {
	wantUsage := func(cmd *cobra.Command, flag, want string) {
		f := cmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("%s missing flag %q", cmd.Name(), flag)
			return
		}
		if f.Usage != want {
			t.Errorf("%s --%s usage = %q, want %q", cmd.Name(), flag, f.Usage, want)
		}
	}

	wantUsage(clickCmd, "snap", "Output interactive snapshot after action")
	wantUsage(clickCmd, "snap-diff", "Output snapshot diff after action (changes only)")
	wantUsage(clickCmd, "text", "Output page text after action (for verification)")

	wantUsage(reloadCmd, "snap", "Output interactive snapshot after reload")
	wantUsage(reloadCmd, "snap-diff", "Output snapshot diff after reload (changes only)")
	wantUsage(reloadCmd, "text", "Output page text after reload (for verification)")

	wantUsage(navCmd, "snap", "Output interactive snapshot after navigation")
	wantUsage(navCmd, "snap-diff", "Output snapshot diff after navigation (changes only)")

	// nav and scroll have no post-action --text flag.
	if navCmd.Flags().Lookup("text") != nil {
		t.Error("navCmd should not register a post-action --text flag")
	}
	if scrollCmd.Flags().Lookup("text") != nil {
		t.Error("scrollCmd should not register a post-action --text flag")
	}
}

// --css-1x was removed in favor of --scale; it must remain registered as a
// hidden, deprecated no-op so old scripts get a notice instead of a hard
// "unknown flag" error.
func TestScreenshotCSS1xDeprecatedShim(t *testing.T) {
	f := screenshotCmd.Flags().Lookup("css-1x")
	if f == nil {
		t.Fatal("css-1x flag should still be registered as a deprecated shim (else old scripts hard-error)")
	}
	if f.Deprecated == "" {
		t.Error("css-1x should be marked deprecated")
	}
	if !f.Hidden {
		t.Error("deprecated css-1x should be hidden from --help")
	}
}
