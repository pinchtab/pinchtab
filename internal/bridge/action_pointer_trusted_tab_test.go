package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const trustedClickFixtureHTML = `<!doctype html><html><body>
<button id="ok" style="width:200px;height:60px"
  onclick="window.clicks=(window.clicks||0)+1;window.clickTrusted=event.isTrusted"
  ondblclick="window.dblclicks=(window.dblclicks||0)+1;window.dblTrusted=event.isTrusted">OK</button>
</body></html>`

func TestTrustedClickLandsOnATabManagerCreatedTab(t *testing.T) {
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(testbrowser.Path(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()
	if err := chromedp.Run(browserCtx); err != nil {
		t.Fatal(err)
	}

	tm := NewTabManager(browserCtx, &config.RuntimeConfig{}, nil, func(context.Context, string) error { return nil })
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(trustedClickFixtureHTML))
	_, tabCtx, cancelTab, err := tm.CreateTab(dataURL)
	if err != nil {
		t.Fatalf("CreateTab: %v", err)
	}
	defer cancelTab()

	var nodeID int64
	lookupCtx, cancelLookup := context.WithTimeout(tabCtx, 5*time.Second)
	defer cancelLookup()
	if err := chromedp.Run(lookupCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		obj, _, err := runtime.Evaluate(`document.querySelector('#ok')`).Do(ctx)
		if err != nil {
			return err
		}
		node, err := dom.DescribeNode().WithObjectID(obj.ObjectID).Do(ctx)
		if err != nil {
			return err
		}
		nodeID = int64(node.BackendNodeID)
		return nil
	})); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		click   func(context.Context, int64) error
		counter string
		trusted string
	}{
		{"click", bridgecdpops.ClickByNodeID, "window.clicks||0", "!!window.clickTrusted"},
		{"double click", bridgecdpops.DoubleClickByNodeID, "window.dblclicks||0", "!!window.dblTrusted"},
	} {
		stepCtx, cancel := context.WithTimeout(tabCtx, trustedClickStepTimeout)
		start := time.Now()
		err := tc.click(stepCtx, nodeID)
		elapsed := time.Since(start)
		cancel()
		if err != nil {
			t.Fatalf("%s: trusted dispatch = %v after %v, want it to complete within %v", tc.name, err, elapsed, trustedClickStepTimeout)
		}

		var count int
		var trusted bool
		if err := chromedp.Run(tabCtx, chromedp.Evaluate(tc.counter, &count), chromedp.Evaluate(tc.trusted, &trusted)); err != nil {
			t.Fatal(err)
		}
		if count != 1 || !trusted {
			t.Fatalf("%s: handler ran %d times, isTrusted=%v; want exactly one trusted event", tc.name, count, trusted)
		}
	}
}
