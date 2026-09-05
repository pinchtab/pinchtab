package bridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

type offlineFixture struct {
	t      *testing.T
	ctx    context.Context
	b      *Bridge
	tabID  string
	srv    *httptest.Server
	mu     sync.Mutex
	served map[string]string
}

func newOfflineFixture(t *testing.T) *offlineFixture {
	chromePath := testbrowser.Path(t)
	f := &offlineFixture{t: t, served: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.served[r.URL.Path] = r.Header.Get("X-Pinchtab-Probe")
		f.mu.Unlock()
		if hops, ok := strings.CutPrefix(r.URL.Path, "/hop/"); ok && hops != "0" {
			n, _ := strconv.Atoi(hops)
			http.Redirect(w, r, fmt.Sprintf("/hop/%d", n-1), http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<title>page " + r.URL.Path + "</title><body>ok</body>"))
	}))
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
		f.srv.Close()
	})
	if err := chromedp.Run(ctx); err != nil {
		t.Fatal(err)
	}
	f.ctx = ctx
	f.tabID = string(chromedp.FromContext(ctx).Target.TargetID)
	tm := NewTabManager(ctx, &config.RuntimeConfig{}, nil, nil, nil)
	rm := NewRouteManager(nil)
	tm.SetRouteManager(rm)
	tm.RegisterTab(f.tabID, ctx)
	f.b = &Bridge{TabManager: tm, routeMgr: rm}
	return f
}

func (f *offlineFixture) navigate(path string) error {
	return f.navigateLimited(path, -1)
}

func (f *offlineFixture) navigateLimited(path string, maxRedirects int) error {
	ctx, cancel := context.WithTimeout(f.ctx, 8*time.Second)
	defer cancel()
	_, err := f.b.Navigate(ctx, f.srv.URL+path, NavigateParams{MaxRedirects: maxRedirects})
	return err
}

func (f *offlineFixture) fetch(path string) string {
	var out string
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	expr := `fetch("` + path + `?" + Math.random()).then(r => "ok-" + r.status).catch(() => "fail")`
	err := chromedp.Run(ctx, chromedp.Evaluate(expr, &out, func(p *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams {
		return p.WithAwaitPromise(true)
	}))
	if err != nil {
		return "error: " + err.Error()
	}
	return out
}

func (f *offlineFixture) onLine() bool {
	var online bool
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Evaluate(`navigator.onLine`, &online)); err != nil {
		f.t.Fatalf("read navigator.onLine: %v", err)
	}
	return online
}

func (f *offlineFixture) waitOnLine(want bool) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if f.onLine() == want {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func (f *offlineFixture) servedHeader(path string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.served[path]
	return v, ok
}

func (f *offlineFixture) setOffline(offline bool) error {
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	return f.b.SetNetworkConditions(ctx, f.tabID, NetworkConditions{Offline: offline, DownloadThroughput: -1, UploadThroughput: -1})
}

// The defect: offline reported applied while a fetch() still succeeded and a
// navigation still rendered, and the state vanished on the next navigation.
func TestOfflineBlocksTrafficAndSurvivesNavigation(t *testing.T) {
	f := newOfflineFixture(t)
	if err := f.navigate("/start"); err != nil {
		t.Fatal(err)
	}
	if err := f.setOffline(true); err != nil {
		t.Fatal(err)
	}
	if got := f.fetch("/sub"); got != "fail" {
		t.Fatalf("fetch while offline = %q, want the request to fail", got)
	}
	if f.onLine() {
		t.Error("navigator.onLine is true while offline")
	}

	navErr := f.navigate("/fresh")
	if _, hit := f.servedHeader("/fresh"); hit {
		t.Fatalf("navigation while offline reached the server (nav err %v)", navErr)
	}
	if !f.b.routeMgr.Offline(f.tabID) {
		t.Fatal("offline state was dropped by the navigation")
	}
	if got := f.fetch("/sub2"); got != "fail" {
		t.Fatalf("fetch after navigation while offline = %q, want the request to fail", got)
	}
	if !f.waitOnLine(false) {
		t.Error("navigator.onLine did not return to false after the navigation")
	}

	if err := f.setOffline(false); err != nil {
		t.Fatal(err)
	}
	if err := f.navigate("/back"); err != nil {
		t.Fatalf("navigation after going back online: %v", err)
	}
	if _, hit := f.servedHeader("/back"); !hit {
		t.Fatal("navigation after going back online never reached the server")
	}
	if got := f.fetch("/sub3"); got != "ok-200" {
		t.Fatalf("fetch after going back online = %q, want ok-200", got)
	}
	if f.b.routeMgr.Offline(f.tabID) {
		t.Error("offline state lingers after being turned off")
	}
	if !f.waitOnLine(true) {
		t.Error("navigator.onLine stayed false after going back online")
	}
}

// Offline and a route rule share one Fetch enable: offline wins while on, and
// the rule is back in effect, alone, once offline is turned off.
func TestOfflineComposesWithRouteRules(t *testing.T) {
	f := newOfflineFixture(t)
	if err := f.navigate("/start"); err != nil {
		t.Fatal(err)
	}
	if err := f.b.AddRouteRule(f.tabID, RouteRule{Pattern: "*/blocked.txt*", Action: RouteActionAbort}); err != nil {
		t.Fatal(err)
	}
	if got := f.fetch("/blocked.txt"); got != "fail" {
		t.Fatalf("aborted route = %q, want fail", got)
	}
	if got := f.fetch("/open.txt"); got != "ok-200" {
		t.Fatalf("unrouted fetch with a rule active = %q, want ok-200", got)
	}

	if err := f.setOffline(true); err != nil {
		t.Fatal(err)
	}
	if got := f.fetch("/open.txt"); got != "fail" {
		t.Fatalf("offline did not win over the route rules: unrouted fetch = %q", got)
	}

	if err := f.setOffline(false); err != nil {
		t.Fatal(err)
	}
	if got := f.fetch("/blocked.txt"); got != "fail" {
		t.Fatalf("route rule not back in effect after offline: %q", got)
	}
	if got := f.fetch("/open.txt"); got != "ok-200" {
		t.Fatalf("unrouted fetch after offline = %q, want ok-200", got)
	}

	if _, err := f.b.RemoveRouteRule(f.tabID, ""); err != nil {
		t.Fatal(err)
	}
	if got := f.fetch("/blocked.txt"); got != "ok-200" {
		t.Fatalf("fetch after removing every rule = %q, want ok-200", got)
	}
}

// Extra headers ride Network.setExtraHTTPHeaders, which Chrome keeps across
// navigations; pinned so the sibling of offline cannot regress silently.
func TestExtraHeadersSurviveNavigation(t *testing.T) {
	f := newOfflineFixture(t)
	if err := f.navigate("/h0"); err != nil {
		t.Fatal(err)
	}
	if err := f.b.SetExtraHTTPHeaders(f.ctx, map[string]string{"X-Pinchtab-Probe": "yes"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/h1", "/h2"} {
		if err := f.navigate(path); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.fetch("/h2sub"); got != "ok-200" {
		t.Fatalf("fetch = %q", got)
	}
	for _, path := range []string{"/h1", "/h2", "/h2sub"} {
		if v, _ := f.servedHeader(path); v != "yes" {
			t.Errorf("%s arrived with X-Pinchtab-Probe=%q, want yes", path, v)
		}
	}
	if v, _ := f.servedHeader("/h0"); v != "" {
		t.Errorf("/h0 was served before the header was set yet carried %q", v)
	}
}

func TestSetOfflineOffWithoutStateIsANoop(t *testing.T) {
	rm := NewRouteManager(nil)
	if err := rm.SetOffline(t.Context(), "tab1", false); err != nil {
		t.Fatal(err)
	}
	if rm.Offline("tab1") {
		t.Fatal("offline reported for a tab that was never set")
	}
	if _, ok := rm.perTab["tab1"]; ok {
		t.Fatal("turning offline off created tab state")
	}
}

func TestRemovingEveryRuleKeepsAnOfflineTab(t *testing.T) {
	rm := NewRouteManager(nil)
	rm.mu.Lock()
	rm.perTab["tab1"] = &tabRouteState{offline: true, rules: []RouteRule{{Pattern: "a", Action: RouteActionAbort}}}
	rm.mu.Unlock()
	if _, err := rm.Remove(t.Context(), "tab1", ""); err != nil {
		t.Fatal(err)
	}
	if !rm.Offline("tab1") {
		t.Fatal("removing the rules dropped the offline state that still owned the tab")
	}
	verdict, active := rm.match("tab1", "https://x/", "fetch", "GET")
	if !active || !verdict.offline {
		t.Fatalf("offline tab with no rules dispatches as %+v active=%v", verdict, active)
	}
}

// The redirect limiter used to enable and disable the Fetch domain itself, letting
// the navigation through an offline tab and dropping offline afterwards while the
// state still reported it.
func TestOfflineSurvivesANavigateUnderARedirectLimit(t *testing.T) {
	f := newOfflineFixture(t)
	if err := f.navigate("/start"); err != nil {
		t.Fatal(err)
	}
	if err := f.setOffline(true); err != nil {
		t.Fatal(err)
	}
	navErr := f.navigateLimited("/limited", 3)
	if _, hit := f.servedHeader("/limited"); hit {
		t.Fatalf("a navigate under a redirect limit reached the server on an offline tab (nav err %v)", navErr)
	}
	if !f.b.routeMgr.Offline(f.tabID) {
		t.Fatal("offline state was dropped by the navigate")
	}
	// The tab now sits on Chrome's error page, whose opaque origin fails every
	// fetch on its own, so the probe that proves the domain survived is a second
	// navigation: it reaches the server only if the first navigate dropped Fetch.
	navErr = f.navigateLimited("/second", 3)
	if _, hit := f.servedHeader("/second"); hit {
		t.Fatalf("the navigate after the first dropped offline: the server was reached (nav err %v)", navErr)
	}
}

func TestARouteRuleIsAppliedDuringANavigateUnderARedirectLimit(t *testing.T) {
	f := newOfflineFixture(t)
	if err := f.navigate("/start"); err != nil {
		t.Fatal(err)
	}
	if err := f.b.AddRouteRule(f.tabID, RouteRule{Pattern: "*/blocked-nav*", Action: RouteActionAbort}); err != nil {
		t.Fatal(err)
	}
	navErr := f.navigateLimited("/blocked-nav", 3)
	if _, hit := f.servedHeader("/blocked-nav"); hit {
		t.Fatalf("an abort rule was continued during the navigate (nav err %v)", navErr)
	}
	if err := f.navigateLimited("/open-nav", 3); err != nil {
		t.Fatalf("an unrouted navigate under a redirect limit failed: %v", err)
	}
	if got := f.fetch("/blocked-nav.txt"); got != "fail" {
		t.Fatalf("the rule stopped firing after the navigate: %q", got)
	}
	if rules := f.b.routeMgr.List(f.tabID); len(rules) != 1 {
		t.Fatalf("rules after the navigate = %+v", rules)
	}
}

func TestTheRedirectLimitStillRefusesLongChains(t *testing.T) {
	f := newOfflineFixture(t)
	if err := f.navigate("/start"); err != nil {
		t.Fatal(err)
	}
	if err := f.navigateLimited("/hop/2", 5); err != nil {
		t.Fatalf("a two-hop chain under a limit of five failed: %v", err)
	}
	if _, hit := f.servedHeader("/hop/0"); !hit {
		t.Fatal("the chain never reached its end")
	}
	err := f.navigateLimited("/hop/2", 1)
	if !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("a two-hop chain under a limit of one returned %v, want ErrTooManyRedirects", err)
	}
	if err := f.navigateLimited("/hop/1", 0); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("maxRedirects 0 allowed a redirect: %v", err)
	}
	if err := f.navigateLimited("/hop/0", 0); err != nil {
		t.Fatalf("maxRedirects 0 refused a navigation with no redirects: %v", err)
	}
	if f.b.routeMgr.Offline(f.tabID) || len(f.b.routeMgr.List(f.tabID)) != 0 {
		t.Fatal("a redirect-limited navigate left routing state behind")
	}
}

func TestAThrottleSurvivesNavigation(t *testing.T) {
	tm := NewTabManager(context.Background(), nil, nil, nil, nil)
	tm.SetNetworkConditions("t", NetworkConditions{Latency: 200, DownloadThroughput: -1, UploadThroughput: -1})
	if _, ok := tm.NetworkConditions("t"); !ok {
		t.Fatal("a pure throttle was discarded, so the next navigation silently drops it")
	}
	tm.SetNetworkConditions("t", NetworkConditions{DownloadThroughput: -1, UploadThroughput: -1})
	if _, ok := tm.NetworkConditions("t"); ok {
		t.Fatal("clearing every condition left a stored entry behind")
	}
}

// The honesty guard: with no interception behind it there is nothing to block
// traffic, so offline must fail rather than flip navigator.onLine and answer
// "offline". The browser is real here on purpose — the emulation call succeeds,
// so only the guard can make the request fail.
func TestSetOfflineFailsWhenTheInterceptionCannotBeInstalled(t *testing.T) {
	f := newOfflineFixture(t)
	if err := f.navigate("/start"); err != nil {
		t.Fatal(err)
	}
	f.b.routeMgr = nil
	if err := f.setOffline(true); err == nil {
		t.Fatal("offline was reported applied with no interception behind it")
	}
	if _, ok := f.b.NetworkConditions(f.tabID); ok {
		t.Error("a failed offline request stored conditions to re-apply after navigation")
	}
	if got := f.fetch("/honesty"); got != "ok-200" {
		t.Errorf("fetch after the refused offline = %q, want ok-200", got)
	}
}
