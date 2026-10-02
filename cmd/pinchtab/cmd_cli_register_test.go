package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Guard for the registration refactor: every browser root command must be in
// the "browser" group, and the shared pointer-flag bundle (+ per-command extras)
// must survive the helper extraction.
func TestBrowserCommandRegistration(t *testing.T) {
	for _, c := range browserRootCommands() {
		if c.GroupID != "browser" {
			t.Errorf("command %q GroupID = %q, want %q", c.Name(), c.GroupID, "browser")
		}
	}

	for _, name := range []string{"css", "x", "y", "humanize", "wait-nav", "mode", "submit", "dismiss-known-interstitials"} {
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

func TestTextCommandRegistersMarkdownAndOutput(t *testing.T) {
	for _, name := range []string{"markdown", "output"} {
		if textCmd.Flags().Lookup(name) == nil {
			t.Errorf("textCmd missing flag %q", name)
		}
	}
	if f := textCmd.Flags().Lookup("output"); f != nil && f.Shorthand != "o" {
		t.Errorf("--output shorthand = %q, want o (the file-output idiom download/screenshot/capture use)", f.Shorthand)
	}
}

func TestTextMarkdownRefusesConflictingModesLocally(t *testing.T) {
	newTabStateHarness(t)
	defer resetTabFlag(textCmd)
	if textCmd.PreRunE == nil {
		t.Fatal("textCmd has no PreRunE, so --markdown --full costs a server round trip to discover")
	}
	defer func() {
		_ = textCmd.Flags().Set("markdown", "false")
		_ = textCmd.Flags().Set("full", "false")
		_ = textCmd.Flags().Set("raw", "false")
	}()

	for _, conflicting := range []string{"full", "raw"} {
		_ = textCmd.Flags().Set("markdown", "true")
		_ = textCmd.Flags().Set("full", "false")
		_ = textCmd.Flags().Set("raw", "false")
		if err := textCmd.Flags().Set(conflicting, "true"); err != nil {
			t.Fatal(err)
		}
		err := textCmd.PreRunE(textCmd, nil)
		if err == nil {
			t.Errorf("--markdown --%s was accepted; it must be refused with a usage error", conflicting)
		} else if !strings.Contains(err.Error(), "markdown") {
			t.Errorf("refusal = %v, want it to name --markdown", err)
		}
	}

	_ = textCmd.Flags().Set("full", "false")
	_ = textCmd.Flags().Set("raw", "false")
	_ = textCmd.Flags().Set("markdown", "true")
	if err := textCmd.PreRunE(textCmd, nil); err != nil {
		t.Errorf("--markdown alone was refused: %v", err)
	}
}

func resetTabFlag(cmd *cobra.Command) {
	if f := cmd.Flags().Lookup("tab"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
}

func TestEveryTabFlagMemberDefaultsTabFromStateInPreRunE(t *testing.T) {
	newTabStateHarness(t)
	WriteTabStateFile("TAB-A")

	if len(tabFlagCommands) == 0 {
		t.Fatal("no addTabFlag members were recorded")
	}
	for _, cmd := range tabFlagCommands {
		name := cmd.CommandPath()
		if cmd.PreRun != nil {
			t.Errorf("%s has a PreRun; cobra skips it whenever a PreRunE is also set, so the tab default must live in PreRunE", name)
		}
		if cmd.PreRunE == nil {
			t.Errorf("%s has no PreRunE, so --tab is never defaulted from the state file", name)
			continue
		}
		resetTabFlag(cmd)
		if err := cmd.PreRunE(cmd, nil); err != nil {
			t.Errorf("%s PreRunE: %v", name, err)
		}
		if got, _ := cmd.Flags().GetString("tab"); got != "TAB-A" {
			t.Errorf("%s --tab = %q after PreRunE, want the state file's TAB-A", name, got)
		}
		resetTabFlag(cmd)
	}
}

func TestMouseButtonValidationStillRunsAfterTheTabChain(t *testing.T) {
	newTabStateHarness(t)
	for _, cmd := range []*cobra.Command{mouseDownCmd, mouseUpCmd, dragCmd} {
		if err := cmd.Flags().Set("button", "bogus"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.PreRunE(cmd, nil); err == nil {
			t.Errorf("%s accepted --button bogus", cmd.CommandPath())
		}
		f := cmd.Flags().Lookup("button")
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
		resetTabFlag(cmd)
	}
}

func TestAddTabFlagLiftsPreRunAndWrapsPreRunE(t *testing.T) {
	newTabStateHarness(t)
	WriteTabStateFile("TAB-A")
	saved := tabFlagCommands
	t.Cleanup(func() { tabFlagCommands = saved })

	var order []string
	withPreRun := &cobra.Command{Use: "pre-run", PreRun: func(*cobra.Command, []string) { order = append(order, "prerun") }}
	refusal := errors.New("refused")
	withPreRunE := &cobra.Command{Use: "pre-run-e", PreRunE: func(c *cobra.Command, _ []string) error {
		got, _ := c.Flags().GetString("tab")
		order = append(order, "prerune:"+got)
		return refusal
	}}
	addTabFlag(withPreRun, withPreRunE)

	for _, cmd := range []*cobra.Command{withPreRun, withPreRunE} {
		if cmd.PreRun != nil {
			t.Errorf("%s kept a PreRun after addTabFlag", cmd.Name())
		}
		if cmd.PreRunE == nil {
			t.Fatalf("%s has no PreRunE after addTabFlag", cmd.Name())
		}
	}
	if err := withPreRun.PreRunE(withPreRun, nil); err != nil {
		t.Errorf("lifted PreRun chain returned %v", err)
	}
	if err := withPreRunE.PreRunE(withPreRunE, nil); !errors.Is(err, refusal) {
		t.Errorf("wrapped PreRunE error = %v, want the original refusal", err)
	}
	if strings.Join(order, ",") != "prerun,prerune:TAB-A" {
		t.Errorf("chain order = %v, want the lifted PreRun and the wrapped PreRunE to run after the tab default", order)
	}
}

func TestCaptureCommandRegistersTabFlag(t *testing.T) {
	if captureCmd.Flags().Lookup("tab") == nil {
		t.Fatal("captureCmd missing --tab flag")
	}
}

func TestNavigateCommandRegistersTimeoutInSeconds(t *testing.T) {
	flag := navCmd.Flags().Lookup("timeout")
	if flag == nil {
		t.Fatal("navCmd missing --timeout flag")
	}
	if flag.DefValue != "0" || flag.Usage != "Navigation timeout in seconds (max 120); overrides the 30s new-tab ceiling" {
		t.Fatalf("--timeout default/usage = %q / %q", flag.DefValue, flag.Usage)
	}
}

// TestPostActionFlagsBundle pins the exact usage strings the shared
// addPostActionFlags helper interpolates per verb, so a future verb edit cannot
// silently drift the --help text, and verifies the one no-text command omits it.
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
	wantUsage(clickCmd, "snap-diff", "Output snapshot diff after action (all nodes, marking [+] added, [~] changed and removed refs)")
	wantUsage(clickCmd, "text", "Output page text after action (for verification)")
	wantUsage(clickCmd, "submit", "Dispatch one DOM click and report bounded post-submit state")

	wantUsage(reloadCmd, "snap", "Output interactive snapshot after reload")
	wantUsage(reloadCmd, "snap-diff", "Output snapshot diff after reload (all nodes, marking [+] added, [~] changed and removed refs)")
	wantUsage(reloadCmd, "text", "Output page text after reload (for verification)")

	wantUsage(navCmd, "snap", "Output interactive snapshot after navigation")
	wantUsage(navCmd, "snap-diff", "Output snapshot diff after navigation (all nodes, marking [+] added, [~] changed and removed refs)")
	// nav has --text because landing on a page is exactly when reading it is
	// useful, and reload — which lands on a page the same way — always had it.
	wantUsage(navCmd, "text", "Output page text after navigation (for verification)")

	// scroll stays excluded: scrolling does not change which document is loaded,
	// so post-scroll text answers nothing --snap-diff does not answer better.
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
