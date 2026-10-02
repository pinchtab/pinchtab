package cdpops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
)

func TestNavigationLifecycleWaiterUsesMainFrameNavigation(t *testing.T) {
	w := &navigationLifecycleWaiter{
		mainFrame: "main",
		ready:     make(chan struct{}),
	}

	w.onEvent(&page.EventLifecycleEvent{FrameID: "main", Name: "DOMContentLoaded"})
	w.onEvent(&page.EventFrameNavigated{Frame: &cdp.Frame{ID: "child", ParentID: "main"}})
	w.onEvent(&page.EventLifecycleEvent{FrameID: "child", Name: "load"})
	select {
	case <-w.ready:
		t.Fatal("child-frame lifecycle marked the navigation ready")
	default:
	}

	w.onEvent(&page.EventFrameNavigated{Frame: &cdp.Frame{ID: "main"}})
	w.onEvent(&page.EventLifecycleEvent{FrameID: "main", Name: "DOMContentLoaded"})
	select {
	case <-w.ready:
	default:
		t.Fatal("main-frame DOMContentLoaded did not mark the navigation ready")
	}
}

func TestNavigationLifecycleWaiterAcceptsSameDocumentNavigation(t *testing.T) {
	w := &navigationLifecycleWaiter{
		mainFrame: "main",
		ready:     make(chan struct{}),
	}
	w.onEvent(&page.EventNavigatedWithinDocument{FrameID: "main"})

	select {
	case <-w.ready:
	default:
		t.Fatal("same-document navigation did not mark the navigation ready")
	}
}

func TestNavigationLifecycleWaiterReportsLastTargetState(t *testing.T) {
	w := &navigationLifecycleWaiter{
		mainFrame: "frame-1",
		targetID:  "target-1",
		lastEvent: "init",
		ready:     make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := w.wait(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context cancellation", err)
	}
	for _, detail := range []string{"target-1", "frame-1", `last lifecycle event "init"`} {
		if !strings.Contains(err.Error(), detail) {
			t.Fatalf("wait error %q does not contain %q", err, detail)
		}
	}
}
