package bridge

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
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

// newBackgroundTab opens url the way the tab manager does, with
// Target.createTarget(background=true), behind a foreground tab it also returns:
// a tab that was never activated is the state the callers regress on.
func newBackgroundTab(t *testing.T, url string) (front, background context.Context) {
	t.Helper()
	front = testbrowser.Tab(t)
	if err := chromedp.Run(front, page.BringToFront()); err != nil {
		t.Fatalf("bring opener tab to front: %v", err)
	}
	exec, err := browserExecutorContext(front)
	if err != nil {
		t.Fatal(err)
	}
	id, err := target.CreateTarget(url).WithBackground(true).Do(exec)
	if err != nil {
		t.Fatalf("create background target: %v", err)
	}
	background, cancel := chromedp.NewContext(front, chromedp.WithTargetID(id))
	t.Cleanup(cancel)
	var hidden bool
	if err := chromedp.Run(background, chromedp.Evaluate(`document.hidden`, &hidden)); err != nil {
		t.Fatalf("attach to background target: %v", err)
	}
	if !hidden {
		t.Fatal("precondition: the background tab is visible, so the test would not exercise a hidden tab")
	}
	return front, background
}
