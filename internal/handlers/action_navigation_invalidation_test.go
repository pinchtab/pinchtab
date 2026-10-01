package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const clickNavFirstPage = `<!doctype html><html><head><title>First</title></head><body>
<a id="go" href="/second">go to second</a>
<a id="slow" href="/slow-second">go slowly to second</a>
</body></html>`

const clickNavSecondPage = `<!doctype html><html><head><title>Second</title></head><body>
<h1>Second</h1>
</body></html>`

func clickNavHandlers(t *testing.T) (*Handlers, string) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/second", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(clickNavSecondPage))
	})
	mux.HandleFunc("/slow-second", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(clickNavSecondPage))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(clickNavFirstPage))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	t.Cleanup(cancelTimeout)

	if err := chromedp.Run(ctx, chromedp.Navigate(server.URL+"/"), chromedp.WaitVisible("#go", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{
		ActionTimeout:  10 * time.Second,
		DefaultBrowser: config.BrowserChrome,
		StateDir:       t.TempDir(),
	}
	b := browserBridge(ctx, cfg)
	const tabID = "tab-click-nav"
	b.RegisterTab(tabID, ctx)
	h := New(b, cfg, nil, nil, nil)
	// A missing ref is a hard ref_not_found here: recovery self-healing is a
	// separate path, and disabling it isolates the ref-cache invalidation this
	// test is about from a descriptor re-match that could dispatch on its own.
	h.Recovery = nil
	return h, tabID
}

func linkRef(t *testing.T, h *Handlers, tabID string) string {
	t.Helper()
	cache := h.Bridge.GetRefCache(tabID)
	if cache == nil {
		t.Fatal("no snapshot cache after snapshot")
	}
	for _, node := range cache.Nodes {
		if node.Role == "link" && node.NodeID != 0 {
			return node.Ref
		}
	}
	t.Fatal("no link node in the snapshot to click")
	return ""
}

func postClickAction(t *testing.T, h *Handlers, tabID, ref, vocabToken string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	body := `{"kind":"click","ref":"` + ref + `","tabId":"` + tabID + `"}`
	req := httptest.NewRequest("POST", "/action", strings.NewReader(body))
	if vocabToken != "" {
		req.Header.Set(vocabHeader, vocabToken)
	}
	w := httptest.NewRecorder()
	h.HandleAction(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func waitRefCacheCleared(h *Handlers, tabID string) bool {
	for range 60 {
		if h.Bridge.GetRefCache(tabID) == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return h.Bridge.GetRefCache(tabID) == nil
}

// The whole card in one end-to-end pass: snapshot the first page, click the link (which
// navigates cross-document), then re-issue the same ref. The re-issue must be refused with
// ref_not_found and — because recovery is off — never reach dispatch. A disciplined client
// echoing the vocabulary token the snapshot minted is refused the same way, since the guard
// the fix installs keys on the observed document change, not on snapshot renumbering.
func TestAClickInducedNavigationInvalidatesTheRefCache(t *testing.T) {
	h, tabID := clickNavHandlers(t)

	snapshotFor(t, h, tabID, "compact")
	ref := linkRef(t, h, tabID)
	token := ""
	if cache := h.Bridge.GetRefCache(tabID); cache != nil {
		token = cache.DomEpoch
	}

	w, body := postClickAction(t, h, tabID, ref, "")
	if w.Code != http.StatusOK {
		t.Fatalf("the causing click returned %d, want 200: %s", w.Code, w.Body.String())
	}
	result, _ := body["result"].(map[string]any)
	if result == nil || result[bridge.ResultNavigated] != true {
		t.Fatalf("the causing click did not report the navigation it triggered: %v", body)
	}
	if result[bridge.ResultLandedURL] == nil || result[bridge.ResultPreviousURL] == nil {
		t.Fatalf("the causing click dropped its own url/previousUrl result: %v", result)
	}

	if !waitRefCacheCleared(h, tabID) {
		t.Fatal("the ref cache survived a click-induced cross-document navigation")
	}

	staleW, staleBody := postClickAction(t, h, tabID, ref, "")
	if staleW.Code != http.StatusNotFound || staleBody["code"] != "ref_not_found" {
		t.Fatalf("naive re-click: status=%d code=%v, want 404 ref_not_found: %s", staleW.Code, staleBody["code"], staleW.Body.String())
	}

	tokenW, tokenBody := postClickAction(t, h, tabID, ref, token)
	if tokenW.Code != http.StatusNotFound || tokenBody["code"] != "ref_not_found" {
		t.Fatalf("token-echoing re-click: status=%d code=%v, want 404 ref_not_found: %s", tokenW.Code, tokenBody["code"], tokenW.Body.String())
	}
}

func postSelectorClick(t *testing.T, h *Handlers, tabID, selector string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	body := `{"kind":"click","selector":"` + selector + `","tabId":"` + tabID + `"}`
	w := httptest.NewRecorder()
	h.HandleAction(w, httptest.NewRequest("POST", "/action", strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func requireClickNavigatedTo(t *testing.T, w *httptest.ResponseRecorder, body map[string]any, path string) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("the navigating click returned %d, want 200: %s", w.Code, w.Body.String())
	}
	result, _ := body["result"].(map[string]any)
	if result == nil || result[bridge.ResultNavigated] != true || result["clicked"] != true {
		t.Fatalf("the navigating click did not report clicked+navigated: %v", body)
	}
	if landed, _ := result[bridge.ResultLandedURL].(string); !strings.HasSuffix(landed, path) {
		t.Fatalf("the navigating click landed on %q, want a url ending %q", landed, path)
	}
}

// Stands in for a timed-out trusted click whose JS fallback runs after its own navigation.
func TestAClickWhoseNavigationDestroysTheContextReportsTheNavigation(t *testing.T) {
	h, tabID := clickNavHandlers(t)
	h.Bridge.(*bridge.Bridge).Actions[bridge.ActionClick] = func(ctx context.Context, _ bridge.ActionRequest) (map[string]any, error) {
		var link *runtime.RemoteObject
		if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById('go')`, &link)); err != nil {
			return nil, err
		}
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(`document.getElementById('go').click()`, nil),
			chromedp.WaitVisible("h1", chromedp.ByQuery),
		); err != nil {
			return nil, err
		}
		return nil, chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.CallFunctionOn(`function() { this.click(); }`).WithObjectID(link.ObjectID).Do(ctx)
			return err
		}))
	}

	w, body := postSelectorClick(t, h, tabID, "#go")
	requireClickNavigatedTo(t, w, body, "/second")
}

// The browser holds DevTools commands during a navigation it has started, so a live
// URL read waits it out; a starved browser starts late and the old document answers.
// Reading the last committed URL makes that race deterministic.
func TestAClickWhoseNavigationCommitsLateReportsTheNavigation(t *testing.T) {
	h, tabID := clickNavHandlers(t)
	b := h.Bridge.(*bridge.Bridge)
	tabCtx, _, err := b.TabContext(tabID)
	if err != nil {
		t.Fatal(err)
	}
	var start string
	if err := chromedp.Run(tabCtx, chromedp.Location(&start)); err != nil {
		t.Fatal(err)
	}
	var committed atomic.Value
	committed.Store(start)
	chromedp.ListenTarget(tabCtx, func(ev any) {
		if nav, ok := ev.(*page.EventFrameNavigated); ok && nav.Frame != nil && nav.Frame.ParentID == "" {
			committed.Store(nav.Frame.URL)
		}
	})
	b.URLReader = func(context.Context) (string, error) {
		return committed.Load().(string), nil
	}
	b.Actions[bridge.ActionClick] = func(ctx context.Context, _ bridge.ActionRequest) (map[string]any, error) {
		return map[string]any{"clicked": true}, chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById('slow').click()`, nil))
	}

	w, body := postSelectorClick(t, h, tabID, "#slow")
	requireClickNavigatedTo(t, w, body, "/slow-second")
}
