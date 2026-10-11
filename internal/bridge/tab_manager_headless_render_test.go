package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// A tab created with focus=false is a background tab: Chromium reports
// document.hidden and runs no requestAnimationFrame callbacks, so a motion
// exit animation (AnimatePresence mode="wait") never finishes and a multi-step
// form never advances. Measured before the fix in a headless instance:
// hidden=true and one rAF callback in a second.
const rafFixtureHTML = `<!doctype html><html><body><script>
window.rafCount = 0;
(function tick() { window.rafCount++; requestAnimationFrame(tick); })();
</script></body></html>`

type tabRenderState struct {
	Hidden bool `json:"hidden"`
	Raf    int  `json:"raf"`
}

func readTabRenderState(t *testing.T, tabCtx context.Context) tabRenderState {
	t.Helper()
	ctx, cancel := context.WithTimeout(tabCtx, 10*time.Second)
	defer cancel()
	var got tabRenderState
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({hidden: document.hidden, raf: window.rafCount})`, &got)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return got
}

func TestCreateTabRendersNewTabsInHeadless(t *testing.T) {
	chromePath := testbrowser.Path(t)
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(rafFixtureHTML))

	for _, tc := range []struct {
		name       string
		cfg        *config.RuntimeConfig
		wantHidden bool
	}{
		{name: "headless", cfg: &config.RuntimeConfig{Headless: true}, wantHidden: false},
		// A human may be looking at a headed window; its pages keep real focus.
		{name: "headed", cfg: &config.RuntimeConfig{Headless: false}, wantHidden: true},
		// An attached browser belongs to someone else, whatever mode it reports.
		{name: "attached", cfg: &config.RuntimeConfig{Headless: true, CDPAttachURL: "ws://127.0.0.1:1"}, wantHidden: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
				chromedp.DefaultExecAllocatorOptions[:],
				chromedp.ExecPath(chromePath),
				chromedp.Flag("headless", true),
				chromedp.Flag("no-sandbox", true),
			)...)
			defer cancelAlloc()
			browserCtx, cancelBrowser := chromedp.NewContext(alloc)
			defer cancelBrowser()
			if err := chromedp.Run(browserCtx); err != nil {
				t.Fatalf("start browser: %v", err)
			}

			// Bridge.tabSetup does this first; without it the new target never runs script.
			tm := NewTabManager(browserCtx, tc.cfg, nil, nil, func(ctx context.Context, _ string) error {
				return preventDebuggerPauses(ctx)
			})
			// Two tabs, because activating one tab would hide the other: both
			// must keep rendering for agents driving tabs in parallel.
			_, firstCtx, cancelFirst, err := tm.CreateTab(dataURL)
			if err != nil {
				t.Fatalf("CreateTab: %v", err)
			}
			defer cancelFirst()
			_, secondCtx, cancelSecond, err := tm.CreateTab(dataURL)
			if err != nil {
				t.Fatalf("CreateTab: %v", err)
			}
			defer cancelSecond()

			// Agents reuse a tab for the next nav, so the state must outlive the first page.
			if err := chromedp.Run(firstCtx, chromedp.Navigate(dataURL)); err != nil {
				t.Fatalf("navigate first tab again: %v", err)
			}

			time.Sleep(time.Second)
			for name, tabCtx := range map[string]context.Context{"first": firstCtx, "second": secondCtx} {
				got := readTabRenderState(t, tabCtx)
				if got.Hidden != tc.wantHidden {
					t.Fatalf("%s tab: document.hidden = %v, want %v (rAF callbacks in 1s: %d)", name, got.Hidden, tc.wantHidden, got.Raf)
				}
				if !tc.wantHidden && got.Raf < 10 {
					t.Fatalf("%s tab ran %d rAF callbacks in 1s, want an animating page", name, got.Raf)
				}
			}
		})
	}
}
