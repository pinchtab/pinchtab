package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const freezeProbePage = `<button id="b" onclick="document.title='clicked'">go</button>
<script>
window.__froze = false; window.__resumed = false;
document.addEventListener('freeze', () => { window.__froze = true; });
document.addEventListener('resume', () => { window.__resumed = true; });
</script>`

func TestFreezeIdleFreezesARealTabAndAnActionUnfreezesIt(t *testing.T) {
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(testbrowser.Path(t)),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()
	if err := chromedp.Run(browserCtx); err != nil {
		t.Fatalf("start browser: %v", err)
	}

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(freezeProbePage))
	var raw target.ID
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		id, err := target.CreateTarget(dataURL).WithBackground(true).Do(ctx)
		raw = id
		return err
	})); err != nil {
		t.Fatalf("create background target: %v", err)
	}
	tabCtx, cancelTab := chromedp.NewContext(browserCtx, chromedp.WithTargetID(raw))
	defer cancelTab()
	if err := chromedp.Run(tabCtx); err != nil {
		t.Fatalf("attach: %v", err)
	}

	tm := NewTabManager(browserCtx, &config.RuntimeConfig{TabLifecyclePolicy: "freeze_idle", TabCloseDelay: 50 * time.Millisecond}, nil, nil)
	tabID := string(raw)
	tm.tabs[tabID] = &TabEntry{Ctx: tabCtx, CDPID: tabID, CreatedAt: time.Now(), LastUsed: time.Now()}

	tm.ScheduleIdleLifecycle(tabID)
	deadline := time.Now().Add(5 * time.Second)
	for !tm.TabFrozen(tabID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !tm.TabFrozen(tabID) {
		t.Fatal("tab was not frozen after closeDelay")
	}

	ctx, _, err := tm.TabContext(tabID)
	if err != nil {
		t.Fatalf("tab lookup: %v", err)
	}
	actCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var title string
	var froze, resumed bool
	if err := chromedp.Run(actCtx,
		chromedp.Click("#b", chromedp.ByQuery),
		chromedp.Title(&title),
		chromedp.Evaluate(`window.__froze`, &froze),
		chromedp.Evaluate(`window.__resumed`, &resumed),
	); err != nil {
		t.Fatalf("action on a frozen tab: %v", err)
	}
	if !froze || !resumed {
		t.Fatalf("page lifecycle froze=%v resumed=%v, want both", froze, resumed)
	}
	if title != "clicked" {
		t.Fatalf("title = %q, want clicked", title)
	}
}
