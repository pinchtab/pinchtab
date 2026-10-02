package bridge

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

// The identity the endpoint hands out is only as good as what the browser puts on the
// wire, and the returned payload was already correct while the headers were not — so this
// reads the request Chrome actually made. Rotating used to set the UA and silence every
// Sec-CH-UA header for the life of the tab, which advertised a Windows Edge that sent none
// of the hints a Windows Edge sends.
func TestRotatedIdentitySendsClientHintsThatAgreeWithTheUserAgent(t *testing.T) {
	captured := newHeaderCapture()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.record(r)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<title>hints</title>"))
	}))
	t.Cleanup(server.Close)

	ctx := newActionTab(t, 40*time.Second)

	// Each load fetches its OWN path and reads back the headers recorded for that path, so a
	// row cannot be handed a request it did not cause.
	//
	// MEASURED, by logging every request this fixture causes: five arrive, only three of them
	// the test's own. Chrome fetches /favicon.ico after each navigation, and those two land
	// between the rows — at a position that moves from run to run. The slot this replaces was
	// written by every one of them, so a row could read a request it never made.
	//
	// The favicons carry the override in effect when they are SENT — they were seen with the
	// windows/edge UA, not a default one — so they are NOT exempt from
	// Network.setUserAgentOverride. What the old capture could hand a row was therefore the
	// PREVIOUS row's persona. Said explicitly because the opposite is easy to assume, and a
	// reader who believes favicons bypass the override will reason wrongly about what any
	// capture like this can see.
	//
	// The reported failure named the browser's own HeadlessChrome default, which can only come
	// from a request issued before ANY override — here, the baseline navigation. That exact
	// ordering was not reproduced, and this comment does not invent one. The structural fact is
	// enough and is what the fix rests on: three unrelated writers, and a capture with no way to
	// say which request it read.
	load := func(path string) http.Header {
		t.Helper()
		if err := chromedp.Run(ctx, chromedp.Navigate(server.URL+path)); err != nil {
			t.Fatal(err)
		}
		headers, ok := captured.forPath(path)
		if !ok {
			t.Fatalf("navigation to %s completed but no request for that path was recorded; the capture saw %v — this is a capture fault, not a header mismatch", path, captured.paths())
		}
		return headers
	}

	// An un-rotated tab is the baseline the fix must not spend: it sends the hints it sends
	// today, so nothing here can be satisfied by moving where they come from.
	baseline := load("/baseline")
	for _, header := range []string{"Sec-Ch-Ua", "Sec-Ch-Ua-Mobile", "Sec-Ch-Ua-Platform"} {
		if baseline.Get(header) == "" {
			t.Fatalf("an un-rotated tab sent no %s, so this browser does not emit client hints on %s and the test cannot tell the fix from the fixture", header, server.URL)
		}
	}

	b := &Bridge{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}
	reduced := stealth.ReducedBrowserVersion(b.Config.BrowserVersion)
	windowsChrome := stealth.ChromeUserAgent(stealth.PlatformWindows, reduced)

	for _, tc := range []struct {
		name         string
		userAgent    string
		platform     string
		wantBrand    string
		absentBrand  string
		wantPlatform string
	}{
		{
			name:         "windows/edge",
			userAgent:    stealth.EdgeUserAgent(windowsChrome, reduced),
			platform:     "Win32",
			wantBrand:    stealth.BrandEdge,
			absentBrand:  stealth.BrandChrome,
			wantPlatform: `"Windows"`,
		},
		{
			name:         "mac/chrome",
			userAgent:    stealth.ChromeUserAgent(stealth.PlatformMacOS, reduced),
			platform:     "MacIntel",
			wantBrand:    stealth.BrandChrome,
			absentBrand:  stealth.BrandEdge,
			wantPlatform: `"macOS"`,
		},
	} {
		if err := b.SetUserAgentOverride(ctx, UserAgentOverrideParams{
			UserAgent:      tc.userAgent,
			Platform:       tc.platform,
			AcceptLanguage: "en-US",
		}); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		headers := load("/" + tc.name)
		if got := headers.Get("User-Agent"); got != tc.userAgent {
			t.Errorf("%s: User-Agent = %q, want %q", tc.name, got, tc.userAgent)
		}
		brands := headers.Get("Sec-Ch-Ua")
		if brands == "" {
			t.Errorf("%s: no sec-ch-ua header; their total absence is a bot signal on its own", tc.name)
		}
		if !strings.Contains(brands, tc.wantBrand) {
			t.Errorf("%s: sec-ch-ua = %q, want the %q brand the UA claims", tc.name, brands, tc.wantBrand)
		}
		if strings.Contains(brands, tc.absentBrand) {
			t.Errorf("%s: sec-ch-ua = %q, want no %q brand; a UA and hints naming two browsers is a browser that cannot exist", tc.name, brands, tc.absentBrand)
		}
		if got := headers.Get("Sec-Ch-Ua-Platform"); got != tc.wantPlatform {
			t.Errorf("%s: sec-ch-ua-platform = %q, want %s", tc.name, got, tc.wantPlatform)
		}
		if got := headers.Get("Sec-Ch-Ua-Mobile"); got != "?0" {
			t.Errorf("%s: sec-ch-ua-mobile = %q, want ?0", tc.name, got)
		}
	}
}

// headerCapture records the request headers per PATH rather than keeping one last-seen
// slot. The slot it replaces is what made the test above intermittently assert on a
// request it never made.
type headerCapture struct {
	mu     sync.Mutex
	byPath map[string]http.Header
}

func newHeaderCapture() *headerCapture {
	return &headerCapture{byPath: map[string]http.Header{}}
}

func (c *headerCapture) record(r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byPath[r.URL.Path] = r.Header.Clone()
}

// forPath answers only for a path that was actually requested. Reporting not-found is the
// point: the alternative — handing back whatever else arrived — is the defect, and a caller
// that reads too early should hear about it rather than assert on a stranger's headers.
func (c *headerCapture) forPath(path string) (http.Header, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	headers, ok := c.byPath[path]
	return headers, ok
}

func (c *headerCapture) paths() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := make([]string, 0, len(c.byPath))
	for path := range c.byPath {
		seen = append(seen, path)
	}
	sort.Strings(seen)
	return seen
}

// The reported failure, reproduced without a browser: a favicon fetch arriving after a
// row's own navigation used to overwrite the single captured slot, so the row asserted on
// a request that never had any override applied and read back the browser's own
// HeadlessChrome default. This runs everywhere, including where no browser is installed
// and the test above skips.
func TestALaterRequestOnAnotherPathCannotOverwriteARowsHeaders(t *testing.T) {
	const rowUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"
	const faviconUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/150.0.0.0 Safari/537.36"

	captured := newHeaderCapture()
	captured.record(requestWithUA(t, "/mac/chrome", rowUA))
	captured.record(requestWithUA(t, "/favicon.ico", faviconUA))

	headers, ok := captured.forPath("/mac/chrome")
	if !ok {
		t.Fatal("the row's own request was not recorded")
	}
	if got := headers.Get("User-Agent"); got != rowUA {
		t.Errorf("the row reads User-Agent %q, want %q — a request on another path overwrote it, which is exactly the intermittent failure this capture exists to remove", got, rowUA)
	}
}

// The control the card asked for, and the one that separates the two causes it could not
// tell apart: reading before the row's own navigation must report NOTHING rather than the
// previous row's headers. A stale read then fails as a capture fault instead of passing
// silently, or failing as a header mismatch that sends the reader after a fingerprint
// regression that is not there.
func TestAPathThatWasNeverRequestedReportsNothingRatherThanTheLastRow(t *testing.T) {
	captured := newHeaderCapture()
	captured.record(requestWithUA(t, "/windows/edge", "edge-ua"))

	if _, ok := captured.forPath("/mac/chrome"); ok {
		t.Error("a path that was never requested answered with headers; the row would assert on the previous row's request")
	}
	if headers, ok := captured.forPath("/windows/edge"); !ok || headers.Get("User-Agent") != "edge-ua" {
		t.Error("the path that WAS requested must still answer, or this capture reports nothing for everything and the guard above passes vacuously")
	}
}

func requestWithUA(t *testing.T, path, userAgent string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("User-Agent", userAgent)
	return r
}
