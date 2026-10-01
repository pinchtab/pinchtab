package observe

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func launchBrowser(t *testing.T) context.Context {
	t.Helper()
	chromePath := testbrowser.Path(t)
	profile := testbrowser.ProfileDir(t)
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	if err := chromedp.Run(ctx); err != nil {
		cancelBrowser()
		cancelAlloc()
		t.Fatalf("launch browser: %v", err)
	}
	t.Cleanup(func() {
		cancelBrowser()
		cancelAlloc()
		_ = os.RemoveAll(profile)
	})
	return ctx
}

func openTab(t *testing.T, browserCtx context.Context, html string) context.Context {
	t.Helper()
	ctx, cancel := chromedp.NewContext(browserCtx)
	t.Cleanup(cancel)
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html,"+html)); err != nil {
		t.Fatalf("open tab: %v", err)
	}
	return ctx
}

// Two page states the counters must tell apart, or nothing proves a reading was
// taken from the target rather than invented.
func TestPageCountersMoveBetweenATrivialAndADOMHeavyPage(t *testing.T) {
	browser := launchBrowser(t)
	trivial := openTab(t, browser, "<html><body>hi</body></html>")
	heavy := openTab(t, browser, "<html><body>"+strings.Repeat("<div><span>x</span></div>", 3000)+"<script>for(let i=0;i<500;i++)document.addEventListener('x'+i,()=>{})</script></body></html>")

	light, err := ReadTargetMetrics(trivial)
	if err != nil {
		t.Fatalf("read trivial: %v", err)
	}
	dense, err := ReadTargetMetrics(heavy)
	if err != nil {
		t.Fatalf("read heavy: %v", err)
	}
	if dense.Nodes < light.Nodes+5000 {
		t.Errorf("nodes: heavy %d vs trivial %d, want the 6000 extra elements to show", dense.Nodes, light.Nodes)
	}
	if dense.JSEventListeners < light.JSEventListeners+500 {
		t.Errorf("listeners: heavy %d vs trivial %d, want the 500 added listeners to show", dense.JSEventListeners, light.JSEventListeners)
	}
	if light.JSHeapUsedMB <= 0 || light.JSHeapTotalMB < light.JSHeapUsedMB || light.Documents < 1 || light.Frames < 1 {
		t.Errorf("trivial reading = %+v, want a live heap and at least one document and frame", light)
	}

	aggregate, err := GetAggregatedMemoryMetrics(browser, map[string]context.Context{"a": trivial, "b": heavy, "dead": cancelledContext()})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if aggregate.Page == nil || aggregate.Page.Targets != 2 || aggregate.UnreadableTargets != 1 {
		t.Fatalf("aggregate = %+v / unreadable %d, want two targets summed and the dead one counted", aggregate.Page, aggregate.UnreadableTargets)
	}
	if aggregate.Page.Nodes < light.Nodes+dense.Nodes-50 || aggregate.Page.Nodes > light.Nodes+dense.Nodes+50 {
		t.Errorf("aggregate nodes %d, want about the sum of the two readings %d", aggregate.Page.Nodes, light.Nodes+dense.Nodes)
	}
	if aggregate.MemoryMB <= 0 {
		t.Errorf("memoryMB = %v, want the process tree measured beside the page counters", aggregate.MemoryMB)
	}
}

// The per-poll cost the Settings toggle gates, measured rather than asserted: the
// figures are logged so the card can carry them, and the bounds pinned are
// that some poll reads every tab and the fastest poll stays under the dashboard's
// 5s emit interval: host load stretches the mean and can push a single target past
// its read timeout, neither of which is the cost of the poll itself.
func TestPerPollCostAcrossSeveralTabsIsRecorded(t *testing.T) {
	browser := launchBrowser(t)
	const tabs = 5
	targets := map[string]context.Context{}
	for i := range tabs {
		targets[fmt.Sprint(i)] = openTab(t, browser, "<html><body>"+strings.Repeat("<p>x</p>", 200)+"</body></html>")
	}
	const polls = 10
	start := time.Now()
	for range polls {
		if _, err := processTreeMemory(browser); err != nil {
			t.Fatal(err)
		}
	}
	rssOnly := time.Since(start) / polls
	complete := 0
	start = time.Now()
	for range polls {
		if page, unreadable := readTargets(targets); page != nil && page.Targets == tabs && unreadable == 0 {
			complete++
		}
	}
	allTargets := time.Since(start) / polls
	if complete == 0 {
		t.Fatalf("none of %d polls read all %d tabs", polls, tabs)
	}
	var total, fastest time.Duration
	for i := range polls {
		start = time.Now()
		if _, err := GetAggregatedMemoryMetrics(browser, targets); err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		total += elapsed
		if i == 0 || elapsed < fastest {
			fastest = elapsed
		}
	}
	full := total / polls
	t.Logf("per poll with %d tabs: %v total = %v process-tree RSS walk + %v for all target reads (~%v per target), fastest %v, %d/%d target polls complete", tabs, full, rssOnly, allTargets, allTargets/tabs, fastest, complete, polls)
	if fastest > 5*time.Second {
		t.Errorf("the fastest of %d polls over %d tabs took %v, longer than the dashboard's 5s emit interval", polls, tabs, fastest)
	}
}
