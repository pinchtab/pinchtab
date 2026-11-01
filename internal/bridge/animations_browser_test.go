package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// DisableAnimationsCSS runs through AddScriptToEvaluateOnNewDocument, which fires
// before the document has a root element. It must neither throw there nor lose the
// style: a page loaded with it injected has the style and no running animation.
func TestDisableAnimationsCSSAppliesOnNewDocument(t *testing.T) {
	chromePath := testbrowser.Path(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><head><style>@keyframes fade{to{opacity:0}} #a{animation:fade 5s infinite}</style></head><body><div id="a">x</div></body></html>`))
	}))
	defer srv.Close()

	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	})

	var mu sync.Mutex
	var thrown []string
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok {
			mu.Lock()
			thrown = append(thrown, e.ExceptionDetails.Error())
			mu.Unlock()
		}
	})

	var styled bool
	var animation string
	if err := chromedp.Run(ctx,
		runtime.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(DisableAnimationsCSS).Do(ctx)
			return err
		}),
		chromedp.Navigate(srv.URL),
		chromedp.WaitReady("#a", chromedp.ByQuery),
		chromedp.Evaluate(`!!document.querySelector('style[data-pinchtab="no-animations"]')`, &styled),
		chromedp.Evaluate(`getComputedStyle(document.getElementById('a')).animationName`, &animation),
	); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(thrown) > 0 {
		t.Errorf("injected script threw: %v", thrown)
	}
	if !styled {
		t.Error("no-animations style missing from the loaded page")
	}
	if animation != "none" {
		t.Errorf("animation still running: animationName=%q", animation)
	}
}
