package bridge

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func TestMain(m *testing.M) { os.Exit(testbrowser.Main(m)) }

// newActionTab brings its shared-browser tab to the front because a background
// tab defers pointer, scroll and input dispatch.
func newActionTab(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(testbrowser.Tab(t), timeout)
	t.Cleanup(cancel)
	if err := chromedp.Run(ctx, page.BringToFront()); err != nil {
		t.Fatalf("bring test tab to front: %v", err)
	}
	return ctx
}
