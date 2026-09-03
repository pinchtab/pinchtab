package bridge

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

func newRefCacheTabManager(t *testing.T) *TabManager {
	t.Helper()
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil, nil)
	tm.SetRefCache("tab1", &RefCache{DomEpoch: "tok", Refs: map[string]int64{"e1": 42}})
	tm.SetFrameScope("tab1", FrameScope{FrameID: "f1"})
	return tm
}

func TestOnTabNavigationCrossDocumentInvalidatesRefCache(t *testing.T) {
	tm := newRefCacheTabManager(t)

	tm.onTabNavigation("tab1", context.Background(), &page.EventFrameNavigated{
		Frame: &cdp.Frame{URL: "https://www.iana.org/"},
	})

	if tm.GetRefCache("tab1") != nil {
		t.Fatal("cross-document navigation left the ref cache resolvable; a stale ref would actuate the new page")
	}
	if _, ok := tm.GetFrameScope("tab1"); ok {
		t.Fatal("cross-document navigation left the frame scope active")
	}
}

func TestOnTabNavigationWithinDocumentKeepsRefCache(t *testing.T) {
	tm := newRefCacheTabManager(t)

	tm.onTabNavigation("tab1", context.Background(), &page.EventNavigatedWithinDocument{URL: "https://example.com/#section"})

	if tm.GetRefCache("tab1") == nil {
		t.Fatal("a same-document fragment change invalidated the ref cache; refs from the current snapshot must stay valid")
	}
}

func TestOnTabNavigationSubframeKeepsRefCache(t *testing.T) {
	tm := newRefCacheTabManager(t)

	tm.onTabNavigation("tab1", context.Background(), &page.EventFrameNavigated{
		Frame: &cdp.Frame{URL: "https://ads.example/", ParentID: "parent"},
	})

	if tm.GetRefCache("tab1") == nil {
		t.Fatal("a subframe navigation invalidated the top-document ref cache")
	}
}

func TestStartTabPolicyWatcherAttachesWithoutIDPI(t *testing.T) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil, nil)
	if tm.idpiDomainPolicyActive() {
		t.Fatal("default config unexpectedly activates the IDPI domain policy; this test asserts the non-IDPI path")
	}

	tm.RegisterTab("tab1", ctx)

	tm.mu.RLock()
	watching := tm.tabs["tab1"] != nil && tm.tabs["tab1"].Watching
	tm.mu.RUnlock()
	if !watching {
		t.Fatal("the navigation watcher did not attach on a default (non-IDPI) config; the invalidation listener would be dead for most installs")
	}
}
