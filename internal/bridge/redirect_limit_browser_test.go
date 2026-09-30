package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	bridgeruntime "github.com/pinchtab/pinchtab/internal/bridge/runtime"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const (
	redirectProxyUser = "probe-user"
	redirectProxyPass = "probe-pass"
)

func newRedirectOrigin(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
			_, _ = io.WriteString(w, "<title>slow</title>slow")
			return
		}
		if hops, ok := strings.CutPrefix(r.URL.Path, "/r/"); ok {
			n, _ := strconv.Atoi(hops)
			if n > 0 {
				http.Redirect(w, r, fmt.Sprintf("/r/%d", n-1), http.StatusFound)
				return
			}
			_, _ = io.WriteString(w, "<title>landed</title>landed")
			return
		}
		_, _ = io.WriteString(w, "<title>origin</title>origin")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newAuthProxy(t *testing.T, authorized *atomic.Int32) *httptest.Server {
	t.Helper()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(redirectProxyUser+":"+redirectProxyPass))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != want {
			w.Header().Set("Proxy-Authenticate", `Basic realm="probe"`)
			w.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
		authorized.Add(1)
		out, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		out.Header = r.Header.Clone()
		out.Header.Del("Proxy-Authorization")
		resp, err := http.DefaultTransport.RoundTrip(out)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		maps.Copy(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newRedirectTab(t *testing.T, proxyURL string) (context.Context, string) {
	t.Helper()
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(testbrowser.Path(t)),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)
	if proxyURL != "" {
		opts = append(opts,
			chromedp.ProxyServer(proxyURL),
			chromedp.Flag("proxy-bypass-list", "<-loopback>"),
		)
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	})
	if err := chromedp.Run(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, string(chromedp.FromContext(ctx).Target.TargetID)
}

func bodyText(t *testing.T, ctx context.Context) string {
	t.Helper()
	var text string
	if err := chromedp.Run(ctx, chromedp.Text("body", &text, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(text)
}

func TestRedirectLimitedNavigationKeepsProxyAuthWorking(t *testing.T) {
	var authorized atomic.Int32
	origin := newRedirectOrigin(t)
	proxy := newAuthProxy(t, &authorized)
	ctx, tabID := newRedirectTab(t, proxy.URL)

	var suppressed atomic.Bool
	proxyCfg := config.BrowserProxyConfig{Server: proxy.URL, Username: redirectProxyUser, Password: redirectProxyPass}
	if err := bridgeruntime.EnableProxyAuth(ctx, proxyCfg, &suppressed); err != nil {
		t.Fatal(err)
	}
	rm := NewRouteManager(nil)
	rm.SetFetchAuthCoordination(func() bool { return true }, func(_ string, v bool) { suppressed.Store(v) })

	if err := bridgecdpops.NavigatePageWithRedirectLimit(ctx, origin.URL+"/r/2", 3, rm); err != nil {
		t.Fatalf("redirect-limited navigation through the authenticated proxy: %v", err)
	}
	if got := bodyText(t, ctx); got != "landed" {
		t.Fatalf("after redirects body = %q, want landed", got)
	}

	before := authorized.Load()
	if err := bridgecdpops.NavigatePage(ctx, origin.URL+"/next"); err != nil {
		t.Fatalf("follow-up navigation: %v", err)
	}
	if got := bodyText(t, ctx); got != "origin" {
		t.Fatalf("follow-up body = %q, want origin; proxy auth stopped answering after the redirect-limited navigation", got)
	}
	if authorized.Load() <= before {
		t.Fatal("follow-up navigation never reached the proxy with credentials")
	}
	if suppressed.Load() {
		t.Fatal("proxy-auth pause dispatch left suppressed after the redirect limit ended")
	}
	if rm.redirectLimitFor(tabID) != nil {
		t.Fatal("redirect limit still installed after the navigation returned")
	}
}

func TestRedirectLimitRefusesTheHopPastTheLimit(t *testing.T) {
	origin := newRedirectOrigin(t)
	ctx, _ := newRedirectTab(t, "")
	rm := NewRouteManager(nil)

	err := bridgecdpops.NavigatePageWithRedirectLimit(ctx, origin.URL+"/r/5", 2, rm)
	if !errors.Is(err, bridgecdpops.ErrTooManyRedirects) {
		t.Fatalf("err = %v, want ErrTooManyRedirects", err)
	}
}

func TestRouteRulesSurviveARedirectLimitedNavigation(t *testing.T) {
	origin := newRedirectOrigin(t)
	ctx, tabID := newRedirectTab(t, "")
	rm := NewRouteManager(nil)

	rule := RouteRule{Pattern: "*/routed*", Action: RouteActionFulfill, Body: "fulfilled", ContentType: "text/plain"}
	if err := rm.AddRule(ctx, tabID, rule); err != nil {
		t.Fatal(err)
	}
	if err := bridgecdpops.NavigatePageWithRedirectLimit(ctx, origin.URL+"/r/1", 3, rm); err != nil {
		t.Fatalf("redirect-limited navigation with a live route: %v", err)
	}
	if err := bridgecdpops.NavigatePage(ctx, origin.URL+"/routed"); err != nil {
		t.Fatalf("navigation to the routed URL: %v", err)
	}
	if got := bodyText(t, ctx); got != "fulfilled" {
		t.Fatalf("routed body = %q, want fulfilled; the route stopped firing after the redirect-limited navigation", got)
	}
	if n := len(rm.List(tabID)); n != 1 {
		t.Fatalf("rules after navigation = %d, want 1", n)
	}
}

func TestRedirectLimitReleasesInterceptionOnReturn(t *testing.T) {
	origin := newRedirectOrigin(t)
	ctx, tabID := newRedirectTab(t, "")
	rm := NewRouteManager(nil)

	if err := bridgecdpops.NavigatePageWithRedirectLimit(ctx, origin.URL+"/r/1", 3, rm); err != nil {
		t.Fatal(err)
	}
	rm.mu.Lock()
	_, stillTracked := rm.perTab[tabID]
	rm.mu.Unlock()
	if stillTracked {
		t.Fatal("tab interception state (listener, Fetch claim) survived the navigation")
	}
	if err := bridgecdpops.NavigatePage(ctx, origin.URL+"/plain"); err != nil {
		t.Fatalf("navigation after release: %v", err)
	}
	if got := bodyText(t, ctx); got != "origin" {
		t.Fatalf("body = %q, want origin", got)
	}
}

func TestTimedOutRedirectLimitedNavigationLeavesTabUsable(t *testing.T) {
	origin := newRedirectOrigin(t)
	ctx, tabID := newRedirectTab(t, "")
	rm := NewRouteManager(nil)

	navCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	err := bridgecdpops.NavigatePageWithRedirectLimit(navCtx, origin.URL+"/slow", 3, rm)
	cancel()
	if err == nil {
		t.Fatal("navigation to a 3s page under a 500ms deadline succeeded")
	}
	rm.mu.Lock()
	_, stillTracked := rm.perTab[tabID]
	rm.mu.Unlock()
	if stillTracked {
		t.Fatal("tab interception state survived the timed-out navigation")
	}
	if err := bridgecdpops.NavigatePage(ctx, origin.URL+"/plain"); err != nil {
		t.Fatalf("navigation after a timed-out redirect-limited navigation: %v", err)
	}
	if got := bodyText(t, ctx); got != "origin" {
		t.Fatalf("body = %q, want origin", got)
	}
}
