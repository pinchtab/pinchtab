package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

const freezeProbePage = `<button id="b" onclick="document.title='clicked'">go</button>
<script>
window.__froze = false; window.__resumed = false;
document.addEventListener('freeze', () => { window.__froze = true; });
document.addEventListener('resume', () => { window.__resumed = true; });
</script>`

func TestFreezeIdleFreezesARealTabAndAnActionUnfreezesIt(t *testing.T) {
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(freezeProbePage))
	browserCtx, tabCtx := newBackgroundTab(t, dataURL)

	tm := NewTabManager(browserCtx, &config.RuntimeConfig{TabLifecyclePolicy: "freeze_idle", TabCloseDelay: 50 * time.Millisecond}, nil, nil)
	tabID := string(chromedp.FromContext(tabCtx).Target.TargetID)
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
