package cdptk

import (
	"strings"
	"testing"
)

// The persistent interactive overlay must use a DOM id distinct from the
// transient screenshot overlay, otherwise taking an annotated screenshot would
// remove a human-facing overlay a user injected with `annotate` (and vice
// versa).
func TestInteractiveOverlayRootID_DistinctFromScreenshotOverlay(t *testing.T) {
	if InteractiveOverlayRootID == OverlayRootID {
		t.Fatalf("interactive and screenshot overlays share id %q; they must be distinct", OverlayRootID)
	}
}

// Guard the interactive overlay's behavioural contract: it must be clickable,
// copy a reference to the clipboard, and compute a CSS/XPath path for the
// clicked element. These are the properties the LLM-fix workflow depends on.
func TestInteractiveOverlayScript_Contract(t *testing.T) {
	for _, needle := range []string{
		"{{DATA}}",                 // item payload placeholder
		"{{ROOT}}",                 // root id placeholder
		"addEventListener('click'", // labels are clickable
		"clipboard",                // copies to the clipboard
		"__ptLastCopied",           // stages copied text for verification
		"function cssPath",         // computes a CSS selector
		"function xPath",           // computes an XPath
		"pointerEvents = 'auto'",   // labels receive pointer events
	} {
		if !strings.Contains(interactiveOverlayScript, needle) {
			t.Errorf("interactiveOverlayScript missing %q", needle)
		}
	}
}
