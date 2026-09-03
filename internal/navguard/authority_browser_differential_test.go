package navguard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/security"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
	"github.com/pinchtab/pinchtab/internal/urls"
)

// The differential this card turns on: the guard's host and the BROWSER's host for
// the same string. They disagreed for every slash-and-backslash spelling, and the
// disagreement is the vulnerability — a guard that reads no host does not check, and
// a browser that reads one navigates. This drives a real Chrome at each spelling and
// asserts the request the browser actually made names the host the guard extracted,
// so a future parser change on either side reds here instead of drifting unmeasured.
//
// CI has no browser, so this skips there; TestTheGuardAgreesWithTheRecordedBrowserAnswers
// is its browserless twin over the same generated table.
func TestTheBrowserResolvesTheHostTheGuardExtracts(t *testing.T) {
	chromePath := testbrowser.Path(t)

	var mu sync.Mutex
	requested := map[string]bool{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requested[r.Host+r.URL.Path] = true
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<h1>ok</h1>"))
	}))
	defer srv.Close()

	// A TLS server so every generated spelling can use one scheme: the scheme-less
	// rows normalize to https://, and a plain-http server would fail those on TLS
	// rather than on the parser question this test is about.
	authority := strings.TrimPrefix(srv.URL, "https://")

	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("ignore-certificate-errors", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 120*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	})

	for _, spelling := range authoritySpellings(authority + "/probe") {
		t.Run(spelling.raw, func(t *testing.T) {
			mu.Lock()
			requested = map[string]bool{}
			mu.Unlock()

			// The browser is handed what PinchTab hands it: validateNavigateTargets
			// normalizes and navigates the normalized target, so that is the string
			// whose browser answer has to match the guard's.
			navigated := urls.EnsureScheme(spelling.raw)
			if err := chromedp.Run(ctx, chromedp.Navigate(navigated)); err != nil {
				t.Fatalf("navigate %q (from %q, %s): %v", navigated, spelling.raw, spelling.note, err)
			}

			mu.Lock()
			reached := requested[authority+"/probe"]
			mu.Unlock()

			host := security.ExtractHost(spelling.raw)
			if !reached {
				t.Fatalf("the browser did not reach the probe for %q (from %q, %s)", navigated, spelling.raw, spelling.note)
			}
			if host != "127.0.0.1" {
				t.Errorf("the browser resolved this and reached the server, but the guard extracted host %q (%s) — a host the guard cannot read is a check it does not run", host, spelling.note)
			}
		})
	}
}

// The browserless twin, recording what the browser answered so the rule is checkable
// where no browser runs: every generated spelling of a special-scheme authority names
// the same host to the guard as the correctly-spelled form does.
func TestTheGuardAgreesWithTheRecordedBrowserAnswers(t *testing.T) {
	for _, spelling := range authoritySpellings("127.0.0.1:9999/probe") {
		t.Run(spelling.raw, func(t *testing.T) {
			if host := security.ExtractHost(spelling.raw); host != "127.0.0.1" {
				t.Errorf("host = %q, want 127.0.0.1 (%s) — the browser reaches this authority, so the guard must read it too", host, spelling.note)
			}
		})
	}
}

// Measured, not assumed: a raw protocol-relative string is not navigable through CDP
// at all — Page.navigate needs an absolute URL and has no base to resolve one
// against. So the protocol-relative bypass never ran through Chrome's parser; it ran
// through PinchTab's own normalization, which turned "//10.0.0.5/x" into the
// scheme-ful "https:////10.0.0.5/x" that Chrome does navigate and the guard read as
// hostless. That is why the fix belongs in the normaliser.
func TestARawProtocolRelativeTargetIsNotNavigableWithoutABase(t *testing.T) {
	chromePath := testbrowser.Path(t)

	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	})

	if err := chromedp.Run(ctx, chromedp.Navigate("//127.0.0.1:9/probe")); err == nil {
		t.Error("a raw protocol-relative target navigated; the note above about where the bypass came from is wrong and should be corrected")
	}
}
