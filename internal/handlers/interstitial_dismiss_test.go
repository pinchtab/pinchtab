package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestKnownInterstitialDismissalIsCatalogAndModalScoped(t *testing.T) {
	chromePath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("case") {
		case "known":
			fmt.Fprint(w, `<div id="payg" role="dialog" aria-modal="true" style="position:fixed;inset:10px;background:white">Pay-as-you-go billing<button id="dismiss" onclick="window.dismissals++;this.parentElement.remove()">Not now</button></div>`)
		case "missing-control":
			fmt.Fprint(w, `<div role="dialog" aria-modal="true" style="position:fixed;inset:10px;background:white">Pay as you go billing<button onclick="window.dismissals++">Continue</button></div>`)
		default:
			fmt.Fprint(w, `<div role="dialog" aria-modal="true" style="position:fixed;inset:10px;background:white">Ordinary wizard<button onclick="window.dismissals++">Continue</button></div><button id="outside" onclick="window.dismissals++">Not now</button>`)
		}
		fmt.Fprint(w, `<script>window.dismissals=0</script>`)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := os.MkdirTemp("", "pinchtab-interstitial-")
	if err != nil {
		t.Fatal(err)
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("host-resolver-rules", "MAP purview.microsoft.com 127.0.0.1"),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 20*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
		_ = os.RemoveAll(profile)
	})

	cfg := &config.RuntimeConfig{ActionTimeout: 5 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := bridge.New(context.Background(), ctx, cfg)
	b.TabManager.RegisterTab("tab-interstitial", ctx)
	h := New(b, cfg, nil, nil, nil)
	pageURL := func(testCase string) string {
		return "http://purview.microsoft.com:" + serverURL.Port() + "/?case=" + testCase
	}

	if err := chromedp.Run(ctx, chromedp.Navigate(pageURL("known"))); err != nil {
		t.Fatal(err)
	}
	result, err := h.dismissKnownInterstitials(ctx, "tab-interstitial")
	if err != nil || result.Action != "dismissed" || result.CatalogID != "m365_purview_pay_as_you_go" {
		t.Fatalf("known result=%+v error=%v", result, err)
	}
	var dismissals int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.dismissals`, &dismissals)); err != nil {
		t.Fatal(err)
	}
	if dismissals != 1 {
		t.Fatalf("known interstitial clicks = %d, want exactly 1", dismissals)
	}

	if err := chromedp.Run(ctx, chromedp.Navigate(pageURL("ordinary"))); err != nil {
		t.Fatal(err)
	}
	result, err = h.dismissKnownInterstitials(ctx, "tab-interstitial")
	if err != nil || result.Action != "none" {
		t.Fatalf("ordinary result=%+v error=%v", result, err)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.dismissals`, &dismissals)); err != nil {
		t.Fatal(err)
	}
	if dismissals != 0 {
		t.Fatalf("ordinary page clicks = %d, want 0", dismissals)
	}

	if err := chromedp.Run(ctx, chromedp.Navigate(pageURL("missing-control"))); err != nil {
		t.Fatal(err)
	}
	result, err = h.dismissKnownInterstitials(ctx, "tab-interstitial")
	if err == nil || result.Action != "blocked" || !strings.Contains(err.Error(), "could not be dismissed") {
		t.Fatalf("missing-control result=%+v error=%v", result, err)
	}
}

func TestKnownInterstitialScriptNeverHardRemovesOrClicksGenericDialogs(t *testing.T) {
	for _, forbidden := range []string{".remove()", `labels.some`, `document.querySelectorAll('button`} {
		if strings.Contains(knownInterstitialDismissJS, forbidden) {
			t.Fatalf("known interstitial script contains generic behavior %q", forbidden)
		}
	}
	for _, required := range []string{"purview.microsoft.com", "compliance.microsoft.com", "pay-as-you-go", "not now"} {
		if !strings.Contains(knownInterstitialDismissJS, required) {
			t.Fatalf("known interstitial script missing catalog key %q", required)
		}
	}
}
