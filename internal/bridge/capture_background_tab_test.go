package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
)

// TestCaptureScreenshotOfBackgroundTabCompletesQuickly is a real-Chromium
// regression test for the bug fixed by always calling Page.bringToFront in
// captureScreenshotWithoutActivation: a genuinely backgrounded tab's
// compositor never resumes painting from Emulation.setFocusEmulationEnabled
// alone, so Page.captureScreenshot previously hung until the caller's
// deadline. A mocked-CDP test can't catch this — the mock "succeeds" at
// focus emulation and returns from captureScreenshot immediately regardless
// of whether a real tab is actually painting.
func TestCaptureScreenshotOfBackgroundTabCompletesQuickly(t *testing.T) {
	html := `<h1>background tab</h1>`
	_, bgCtx := newBackgroundTab(t, "data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html)))

	// Tight but fair: production uses a 30s ActionTimeout, and a tab that
	// never resumes painting hangs for the whole window. 10s is generous for
	// a real capture to complete, but a regression (activation skipped)
	// fails this test loudly instead of quietly eating a 30s timeout.
	captureCtx, cancelCapture := context.WithTimeout(bgCtx, 10*time.Second)
	defer cancelCapture()

	buf, err := CaptureScreenshot(captureCtx, ScreenshotOpts{Format: page.CaptureScreenshotFormatPng})
	if err != nil {
		t.Fatalf("capture background tab: %v", err)
	}
	if len(buf) == 0 {
		t.Fatal("capture returned no image bytes")
	}
}
