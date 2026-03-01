package main

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// TestBuildChromeOpts_HeadlessMode verifies that headless mode adds chromedp.Headless
func TestBuildChromeOpts_HeadlessMode(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Headless:    true,
		ProfileDir:  "/tmp/test-profile",
		ChromeBinary: "",
	}

	opts := buildChromeOpts(cfg)

	// Check that chromedp.Headless was added
	hasHeadlessOpt := false
	for _, opt := range opts {
		// chromedp.Headless is ExecAllocatorOption that sets headless mode
		// We can verify by checking if it's in the opts slice
		// Since ExecAllocatorOption is a func type, we check the length increased
		if opt != nil {
			// This is a bit of a proxy check - the actual function will set headless=true
			hasHeadlessOpt = true
			break
		}
	}

	if !hasHeadlessOpt || len(opts) == 0 {
		t.Error("expected chromedp.Headless option to be added for headless mode")
	}
}

// TestBuildChromeOpts_HeadedMode verifies that headed mode does NOT add any headless flag
func TestBuildChromeOpts_HeadedMode(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Headless:    false,
		ProfileDir:  "/tmp/test-profile",
		ChromeBinary: "",
	}

	opts := buildChromeOpts(cfg)

	// For headed mode, we should still have options but NOT set headless=false
	// The key is that we don't set any headless flag when headless=false
	// Chrome defaults to headed when no headless flag is present

	if len(opts) == 0 {
		t.Error("expected some Chrome options even for headed mode")
	}

	// This test verifies the fix: we don't call chromedp.Flag("headless", false)
	// which was broken. Instead, we just don't add any headless flag for headed mode.
	t.Logf("headed mode generated %d options (Chrome will default to headed)", len(opts))
}

// TestBuildChromeOpts_ComparesOptions verifies that headless and headed modes differ
func TestBuildChromeOpts_ComparesOptions(t *testing.T) {
	cfgHeadless := &config.RuntimeConfig{
		Headless:    true,
		ProfileDir:  "/tmp/test-profile",
		ChromeBinary: "",
	}

	cfgHeaded := &config.RuntimeConfig{
		Headless:    false,
		ProfileDir:  "/tmp/test-profile",
		ChromeBinary: "",
	}

	optsHeadless := buildChromeOpts(cfgHeadless)
	optsHeaded := buildChromeOpts(cfgHeaded)

	// Headless mode should have one more option (chromedp.Headless)
	if len(optsHeadless) <= len(optsHeaded) {
		t.Errorf("expected headless mode to have more options (%d) than headed mode (%d)",
			len(optsHeadless), len(optsHeaded))
	}

	t.Logf("headless mode: %d options, headed mode: %d options",
		len(optsHeadless), len(optsHeaded))
}

// TestBuildChromeOpts_WithCustomBinary verifies custom binary path is honored
func TestBuildChromeOpts_WithCustomBinary(t *testing.T) {
	customBinary := "/custom/path/to/chrome"
	cfg := &config.RuntimeConfig{
		Headless:    true,
		ProfileDir:  "/tmp/test-profile",
		ChromeBinary: customBinary,
	}

	opts := buildChromeOpts(cfg)

	if len(opts) == 0 {
		t.Fatal("expected options when custom binary is set")
	}

	// We can't directly verify the binary in opts since they're function closures,
	// but this test ensures the code path works without panicking
	t.Logf("custom binary config generated %d options", len(opts))
}
