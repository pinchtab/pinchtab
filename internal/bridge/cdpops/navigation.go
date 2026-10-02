package cdpops

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const TargetTypePage = "page"

func NavigatePage(ctx context.Context, url string) error {
	replaceInitialBlank, _ := shouldReplaceInitialBlankNavigation(ctx)
	_, err := navigateAndWait(ctx, url, replaceInitialBlank)
	return err
}

// DispatchNavigation wakes a headed background renderer without activating its
// OS window, sends one navigation command, and returns as soon as the target
// accepts it. Callers retain the tab context and poll readiness separately.
// Focus emulation is intentionally fail-closed: falling back to Page.bringToFront
// would violate the background-open contract.
func DispatchNavigation(ctx context.Context, url string) error {
	replaceInitialBlank, err := shouldReplaceInitialBlankNavigation(ctx)
	if err != nil {
		return fmt.Errorf("inspect initial navigation: %w", err)
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(execCtx context.Context) error {
		return dispatchBackgroundNavigation(execCtx, url, replaceInitialBlank)
	}))
}

func dispatchBackgroundNavigation(ctx context.Context, url string, replaceInitialBlank bool) error {
	if err := emulation.SetFocusEmulationEnabled(true).Do(ctx); err != nil {
		return fmt.Errorf("enable background focus emulation: %w", err)
	}
	if err := SetPageFrozen(ctx, false); err != nil {
		return fmt.Errorf("activate background web lifecycle: %w", err)
	}
	return startNavigation(ctx, url, replaceInitialBlank)
}

var ErrTooManyRedirects = fmt.Errorf("too many redirects")

type RedirectLimit struct {
	max     int
	count   atomic.Int32
	blocked atomic.Bool
}

func NewRedirectLimit(max int) *RedirectLimit {
	return &RedirectLimit{max: max}
}

func (l *RedirectLimit) Admit(e *fetch.EventRequestPaused) bool {
	if e.RedirectedRequestID == "" {
		return true
	}
	if int(l.count.Add(1)) > l.max {
		l.blocked.Store(true)
		return false
	}
	return true
}

func (l *RedirectLimit) Err() error {
	if !l.blocked.Load() {
		return nil
	}
	return fmt.Errorf("%w: got %d, max %d", ErrTooManyRedirects, l.count.Load(), l.max)
}

type RedirectInterceptor interface {
	BeginRedirectLimit(ctx context.Context, limit *RedirectLimit) (end func(), err error)
}

// NavigatePageWithRedirectLimit returns the main document's HTTP status, or 0
// when the navigation committed without a network response (same-document,
// file:, data:, about:).
func NavigatePageWithRedirectLimit(ctx context.Context, url string, maxRedirects int, interceptor RedirectInterceptor) (int, error) {
	replaceInitialBlank, _ := shouldReplaceInitialBlankNavigation(ctx)

	if maxRedirects < 0 {
		return navigateAndWait(ctx, url, replaceInitialBlank)
	}
	if interceptor == nil {
		return 0, fmt.Errorf("redirect limit requested but no request interceptor is available")
	}

	limit := NewRedirectLimit(maxRedirects)
	end, err := interceptor.BeginRedirectLimit(ctx, limit)
	if err != nil {
		return 0, err
	}
	defer end()

	waiter, err := newNavigationLifecycleWaiter(ctx)
	if err != nil {
		return 0, err
	}
	defer waiter.close()

	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return startNavigation(ctx, url, replaceInitialBlank)
	}))
	if limitErr := limit.Err(); limitErr != nil {
		return 0, limitErr
	}
	if err != nil {
		return 0, err
	}
	status, waitErr := waiter.wait(ctx)
	if limitErr := limit.Err(); limitErr != nil {
		return 0, limitErr
	}
	return status, waitErr
}

// ShouldReplaceBlankHistoryEntry reports whether the first navigation should replace an untouched about:blank entry.
func ShouldReplaceBlankHistoryEntry(curURL string, cur int64, entryCount int) bool {
	return curURL == "about:blank" && cur == 0 && entryCount == 1
}

func shouldReplaceInitialBlankNavigation(ctx context.Context) (bool, error) {
	var (
		cur     int64
		entries []*page.NavigationEntry
		curURL  string
	)

	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cur, entries, err = page.GetNavigationHistory().Do(ctx)
			return err
		}),
		chromedp.Location(&curURL),
	); err != nil {
		return false, err
	}

	return ShouldReplaceBlankHistoryEntry(curURL, cur, len(entries)), nil
}

func startNavigation(ctx context.Context, url string, replaceInitialBlank bool) error {
	if !replaceInitialBlank {
		_, _, _, _, err := page.Navigate(url).Do(ctx)
		return err
	}

	encodedURL, err := json.Marshal(url)
	if err != nil {
		return fmt.Errorf("encode navigation url: %w", err)
	}

	return chromedp.Evaluate("window.location.replace("+string(encodedURL)+")", nil).Do(ctx)
}

func navigateAndWait(ctx context.Context, url string, replaceInitialBlank bool) (int, error) {
	waiter, err := newNavigationLifecycleWaiter(ctx)
	if err != nil {
		return 0, err
	}
	defer waiter.close()

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return startNavigation(ctx, url, replaceInitialBlank)
	})); err != nil {
		return 0, err
	}

	return waiter.wait(ctx)
}

type navigationLifecycleWaiter struct {
	mu             sync.Mutex
	mainFrame      cdp.FrameID
	targetID       string
	navigationSeen bool
	documentStatus int
	lastEvent      string
	ready          chan struct{}
	readyOnce      sync.Once
	cancel         context.CancelFunc
}

func newNavigationLifecycleWaiter(ctx context.Context) (*navigationLifecycleWaiter, error) {
	var tree *page.FrameTree
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(execCtx context.Context) error {
		var err error
		tree, err = page.GetFrameTree().Do(execCtx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("inspect navigation target: %w", err)
	}
	if tree == nil || tree.Frame == nil {
		return nil, fmt.Errorf("inspect navigation target: main frame unavailable")
	}

	w := &navigationLifecycleWaiter{
		mainFrame: tree.Frame.ID,
		ready:     make(chan struct{}),
	}
	if c := chromedp.FromContext(ctx); c != nil && c.Target != nil {
		w.targetID = string(c.Target.TargetID)
	}

	listenCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	chromedp.ListenTarget(listenCtx, w.onEvent)
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(execCtx context.Context) error {
		if err := network.Enable().Do(execCtx); err != nil {
			return err
		}
		return page.SetLifecycleEventsEnabled(true).Do(execCtx)
	})); err != nil {
		cancel()
		return nil, fmt.Errorf("enable navigation lifecycle events: %w", err)
	}
	return w, nil
}

func (w *navigationLifecycleWaiter) onEvent(event any) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch e := event.(type) {
	case *network.EventResponseReceived:
		if e.Type != network.ResourceTypeDocument || e.FrameID != w.mainFrame || e.Response == nil {
			return
		}
		w.documentStatus = int(e.Response.Status)
	case *page.EventFrameNavigated:
		if e.Frame == nil || e.Frame.ParentID != "" {
			return
		}
		w.mainFrame = e.Frame.ID
		w.navigationSeen = true
		w.lastEvent = "frameNavigated"
	case *page.EventNavigatedWithinDocument:
		if e.FrameID != w.mainFrame {
			return
		}
		w.navigationSeen = true
		w.lastEvent = "navigatedWithinDocument"
		w.markReady()
	case *page.EventLifecycleEvent:
		if e.FrameID != w.mainFrame {
			return
		}
		w.lastEvent = e.Name
		if w.navigationSeen && (e.Name == "DOMContentLoaded" || e.Name == "load") {
			w.markReady()
		}
	}
}

func (w *navigationLifecycleWaiter) markReady() {
	w.readyOnce.Do(func() { close(w.ready) })
}

func (w *navigationLifecycleWaiter) wait(ctx context.Context) (int, error) {
	select {
	case <-w.ready:
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.documentStatus, nil
	case <-ctx.Done():
		w.mu.Lock()
		mainFrame := w.mainFrame
		lastEvent := w.lastEvent
		w.mu.Unlock()
		return 0, fmt.Errorf("navigation target %s main frame %s did not reach DOMContentLoaded (last lifecycle event %q): %w",
			w.targetID, mainFrame, lastEvent, ctx.Err())
	}
}

func (w *navigationLifecycleWaiter) close() {
	if w.cancel != nil {
		w.cancel()
	}
}

func WaitForTitle(ctx context.Context, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		var title string
		if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
			return "", err
		}
		return title, nil
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline:
			var title string
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
				return "", err
			}
			return title, nil
		case <-ticker.C:
			var title string
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
				continue
			}
			if title != "" && title != "about:blank" {
				return title, nil
			}
		}
	}
}
