package bridge

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func TestAttachIndicatorNamesPortPersistsAndClears(t *testing.T) {
	chromePath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}
	profile, err := os.MkdirTemp("", "pinchtab-attach-indicator-")
	if err != nil {
		t.Fatal(err)
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 20*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
		_ = os.RemoveAll(profile)
	})
	pageURL := func(title string) string {
		html := "<title>" + title + "</title><p>fixture</p>"
		return "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(pageURL("Original"))); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{Port: "9876"}
	b := New(context.Background(), ctx, cfg)
	b.stealthLaunchMode = stealth.LaunchModeAttached
	execCtx := cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Target)
	b.TabManager.RegisterTab("tab-indicator", execCtx)
	if err := b.tabSetup(execCtx, "tab-indicator"); err != nil {
		t.Fatal(err)
	}
	// Repeated setup for the same bridge/tab is idempotent and must not
	// strand a second new-document script.
	if err := b.tabSetup(execCtx, "tab-indicator"); err != nil {
		t.Fatal(err)
	}
	if got := len(b.attachIndicators); got != 1 {
		t.Fatalf("attach indicator registrations = %d, want 1", got)
	}
	readTitle := func() string {
		var title string
		if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
			t.Fatal(err)
		}
		return title
	}
	if got := readTitle(); got != "[PinchTab :9876] Original" {
		t.Fatalf("installed title = %q", got)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.title = "Updated"`, nil), chromedp.Sleep(50*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if got := readTitle(); got != "[PinchTab :9876] Updated" {
		t.Fatalf("maintained title = %q", got)
	}

	b2 := New(context.Background(), ctx, &config.RuntimeConfig{Port: "9877"})
	b2.stealthLaunchMode = stealth.LaunchModeAttached
	b2.TabManager.RegisterTab("tab-indicator", execCtx)
	if err := b2.tabSetup(execCtx, "tab-indicator"); err != nil {
		t.Fatal(err)
	}
	if got := readTitle(); got != "[PinchTab :9876,:9877] Updated" {
		t.Fatalf("two-owner title = %q", got)
	}

	b.Cleanup()
	if got := readTitle(); got != "[PinchTab :9877] Updated" {
		t.Fatalf("first-owner cleanup title = %q", got)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.title = "Second owner"`, nil), chromedp.Sleep(50*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if got := readTitle(); got != "[PinchTab :9877] Second owner" {
		t.Fatalf("remaining-owner maintained title = %q", got)
	}
	b2.Cleanup()
	if got := readTitle(); got != "Second owner" {
		t.Fatalf("final cleared title = %q", got)
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(pageURL("After detach"))); err != nil {
		t.Fatal(err)
	}
	if got := readTitle(); got != "After detach" {
		t.Fatalf("new-document script survived detach: %q", got)
	}
}

func TestAttachIndicatorRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"", "0", "65536", "not-a-port"} {
		if _, err := attachIndicatorScript(port); err == nil {
			t.Fatalf("port %q accepted", port)
		}
	}
	script, err := attachIndicatorScript(" 9867 ")
	if err != nil || !strings.Contains(script, `const port = 9867`) {
		t.Fatalf("valid port script error=%v", err)
	}
	if _, err := clearAttachIndicatorScript(" 9867 "); err != nil {
		t.Fatalf("valid cleanup script error=%v", err)
	}
}
