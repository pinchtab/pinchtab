package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Regression guard for the lost-deadline bug: DownloadURL must fail fast on a
// dead browser context instead of blocking past its caps (it once navigated
// on an unbounded tab context and could hang until Chrome's own timeouts).
func TestDownloadURL_FailsFastOnDeadBrowserContext(t *testing.T) {
	parent, cancel := chromedp.NewContext(context.Background())
	cancel()

	b := &Bridge{BrowserCtx: parent}
	start := time.Now()
	_, err := b.DownloadURL(context.Background(), "https://example.com/file", DownloadOpts{})
	if err == nil {
		t.Fatal("expected error on dead browser context")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("DownloadURL blocked for %v on a dead context", elapsed)
	}
}
