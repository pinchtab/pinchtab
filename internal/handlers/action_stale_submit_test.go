package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
	"github.com/pinchtab/semantic"
	"github.com/pinchtab/semantic/recovery"
)

// orderFixture serves a form whose button posts an order, and counts the posts. The counter
// is the instrument this card turns on: a response saying the ref was not found is only
// wrong if an order was placed anyway, and no assertion on the response text can show that.
type orderFixture struct {
	server *httptest.Server
	orders *int64
}

func newOrderFixture(t *testing.T) orderFixture {
	t.Helper()
	var orders int64
	mux := http.NewServeMux()
	mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			atomic.AddInt64(&orders, 1)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<h1>Order placed</h1>`))
	})
	mux.HandleFunc("/terms", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<h1>Terms</h1>`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// The link is what the navigation half needs: a click that moves the page and
		// posts nothing, so the two halves of this card can be told apart on one fixture.
		// It moves with history.pushState rather than an href, because the guard reads the
		// URL as soon as the click returns and a real navigation is still in flight then —
		// an href would make this test depend on that race rather than on the guard.
		_, _ = w.Write([]byte(`<h1>Checkout</h1>
			<form action="/order" method="post"><button id="b">Place order</button></form>
			<a id="t" href="#" onclick="history.pushState({}, '', '/terms'); return false;">Read the terms</a>`))
	})
	mux.HandleFunc("/renumber", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<script>window.clicked = []</script>` + renumberPage(false)))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return orderFixture{server: srv, orders: &orders}
}

// renumberPage is the three-control checkout. Rendered with the catalogue button first,
// the ref the first render gave "Apply coupon" names "Browse catalogue" in the next epoch.
func renumberPage(catalogueFirst bool) string {
	coupon := `<button id="a" type="button" onclick="window.clicked.push('a')">Apply coupon</button>`
	catalogue := `<button id="b" type="button" onclick="window.clicked.push('b')">Browse catalogue</button>`
	order := `<form action="/order" method="post"><button id="p">Place order</button></form>`
	if catalogueFirst {
		return `<h1>Checkout</h1>` + catalogue + coupon + order
	}
	return `<h1>Checkout</h1>` + coupon + catalogue + order
}

func (f orderFixture) placed() int64 { return atomic.LoadInt64(f.orders) }

// settleForOrder waits for a post that may still be in flight. A form submission is
// dispatched by the browser and arrives at the server after the call returns, so reading
// the counter immediately would let a placed order pass as none — the exact failure this
// fixture exists to detect.
func (f orderFixture) settleForOrder() int64 {
	for i := 0; i < 40 && f.placed() == 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	return f.placed()
}

// staleSubmitHandlers stands up a real browser on the checkout page with handlers whose
// recovery engine is the production one, and records the intent a snapshot would have
// cached for the button's ref. That intent is what recovery matches on, so this reproduces
// the state the card describes: a ref the cache no longer resolves, whose remembered
// descriptor still names the submit button.
func staleSubmitHandlers(t *testing.T, f orderFixture) (*Handlers, context.Context, string) {
	t.Helper()
	h, ctx, tabID := checkoutHandlers(t, f, "/")
	h.Recovery.RecordIntent(tabID, "e2", recovery.IntentEntry{
		Descriptor: semantic.ElementDescriptor{Ref: "e2", Role: "button", Name: "Place order"},
		CachedAt:   time.Now(),
	})
	h.Recovery.RecordIntent(tabID, "e3", recovery.IntentEntry{
		Descriptor: semantic.ElementDescriptor{Ref: "e3", Role: "link", Name: "Read the terms"},
		CachedAt:   time.Now(),
	})
	return h, ctx, tabID
}

func checkoutHandlers(t *testing.T, f orderFixture, path string) (*Handlers, context.Context, string) {
	t.Helper()
	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	t.Cleanup(cancelTimeout)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(f.server.URL+path),
		chromedp.WaitVisible("#b", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{
		ActionTimeout:  10 * time.Second,
		DefaultBrowser: config.BrowserChrome,
		StateDir:       t.TempDir(),
	}
	b := browserBridge(ctx, cfg)
	const tabID = "tab-stale-submit"
	b.RegisterTab(tabID, ctx)
	return New(b, cfg, nil, nil, nil), ctx, tabID
}

func staleClick() *bridge.ActionRequest {
	return &bridge.ActionRequest{Kind: bridge.ActionClick, Ref: "e2"}
}

// The defect: the click was DISPATCHED at a node recovery matched, the form posted, and the
// caller was told the ref could not be found — so the only sane reaction, retry after a
// re-snapshot, places a second order. The refusal has to come before the dispatch, which is
// why the counter rather than the message is the assertion.
func TestAStaleRefOnASubmitControlIsRefusedBeforeAnyOrderIsPlaced(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := staleSubmitHandlers(t, f)

	_, _, rr, err := h.executeActionResilient(ctx, staleClick(), "", h.Config, tabID, true)

	if !errors.Is(err, ErrStaleSubmitTarget) {
		t.Fatalf("error = %v, want the stale-submit refusal", err)
	}
	if placed := f.settleForOrder(); placed != 0 {
		t.Errorf("%d order(s) placed by a call that refused; the refusal must precede the dispatch", placed)
	}
	if rr != nil {
		t.Errorf("a recovery block was published for a call that never re-resolved anything: %+v", rr)
	}
	if got := err.Error(); !strings.Contains(got, "/snapshot") {
		t.Errorf("refusal %q does not tell the caller to re-snapshot", got)
	}
}

// A ref that is stale on a page with nothing to submit keeps the ordinary recovery path:
// this refusal is scoped to submit controls, not to stale refs at large.
func TestAStaleRefWithNoSubmitControlIsNotRefusedByTheSubmitGuard(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := staleSubmitHandlers(t, f)

	if err := chromedp.Run(ctx, chromedp.Navigate(f.server.URL+"/order")); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := h.executeActionResilient(ctx, staleClick(), "", h.Config, tabID, true)

	if errors.Is(err, ErrStaleSubmitTarget) {
		t.Errorf("a page with no form answered the submit refusal: %v", err)
	}
	if placed := f.settleForOrder(); placed != 0 {
		t.Errorf("%d order(s) placed on a page with no form", placed)
	}
}

// The guard is what stops the order, not the wording of the refusal: with the DOM check
// stubbed to the answer it gave before this card — no, this is not a submit — the identical
// call dispatches at the node recovery MATCHED and the form posts. That is the reported
// defect reproduced in the suite, so the refusal above cannot be weakened without this
// turning red.
func TestTheSubmitCheckIsWhatStopsTheOrder(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := staleSubmitHandlers(t, f)

	original := submitControlJS
	t.Cleanup(func() { submitControlJS = original })
	submitControlJS = `function() { return false; }`

	_, _, _, err := h.executeActionResilient(ctx, staleClick(), "", h.Config, tabID, true)

	if errors.Is(err, ErrStaleSubmitTarget) {
		t.Fatalf("the stubbed check still refused, so the refusal is not coming from the DOM check: %v", err)
	}
	if placed := f.settleForOrder(); placed != 1 {
		t.Fatalf("orders placed without the check = %d, want 1: recovery no longer reaches the button, so this fixture has stopped reproducing the defect the guard prevents", placed)
	}
}

// A stale ref whose recovered click NAVIGATES ran the action, so it reports success:
// the click landed, the page moved, and the caller is told where it landed. The
// substitution is the one thing this must not hide — the dispatch went to whatever
// recovery matched, not to the ref the caller named.
func TestAStaleRefWhoseRecoveredClickNavigatesSucceedsAndDisclosesTheSubstitution(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := staleSubmitHandlers(t, f)

	result, _, rr, err := h.executeActionResilient(ctx, &bridge.ActionRequest{Kind: bridge.ActionClick, Ref: "e3"}, "", h.Config, tabID, true)

	if err != nil {
		t.Fatalf("a recovered click that navigated reported failure: %v", err)
	}
	if result[bridge.ResultNavigated] != true {
		t.Errorf("result does not report the navigation: %v", result)
	}
	if landed, _ := result[bridge.ResultLandedURL].(string); !strings.Contains(landed, "/terms") {
		t.Errorf("landed url = %v, want the link's own target", result[bridge.ResultLandedURL])
	}
	if result[bridge.ResultRefsStale] != true {
		t.Errorf("result does not say the caller's refs are dead: %v", result)
	}
	if rr == nil {
		t.Fatal("no recovery record published for a dispatched click, so nothing says which element was clicked")
	}
	if rr.NewRef == "" {
		t.Errorf("recovery record names no matched ref: %+v", rr)
	}
}

// The refusal is the opposite case and keeps the opposite rule: nothing was dispatched, so
// a record reading recovered:true would suggest a click landed. Absence cannot be misread.
func TestTheRefusalPublishesNoRecoveryRecordWhileTheNavigationDoes(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := staleSubmitHandlers(t, f)

	_, _, refused, refusalErr := h.executeActionResilient(ctx, staleClick(), "", h.Config, tabID, true)
	if !errors.Is(refusalErr, ErrStaleSubmitTarget) {
		t.Fatalf("refusal error = %v", refusalErr)
	}
	if refused != nil {
		t.Errorf("the refusal published a recovery record for a click that never happened: %+v", refused)
	}

	_, _, navigated, navErr := h.executeActionResilient(ctx, &bridge.ActionRequest{Kind: bridge.ActionClick, Ref: "e3"}, "", h.Config, tabID, true)
	if navErr != nil {
		t.Fatalf("the navigating click reported failure: %v", navErr)
	}
	if navigated == nil {
		t.Error("the navigation published no recovery record, so a dispatched click discloses nothing about its target")
	}
}

// cachedClick snapshots the live page into the ref cache and returns a click on the named
// element resolved the way the endpoint resolves it, with the intent a snapshot would have
// recorded. This is the other cache state an agent can be in: the ref still resolves, to a
// node the page is about to destroy.
func cachedClick(t *testing.T, h *Handlers, ctx context.Context, tabID, role, name string) *bridge.ActionRequest {
	t.Helper()
	h.refreshRefCache(ctx, tabID)
	for _, node := range h.Bridge.GetRefCache(tabID).Nodes {
		if node.Role == role && node.Name == name {
			h.Recovery.RecordIntent(tabID, node.Ref, recovery.IntentEntry{
				Descriptor: semantic.ElementDescriptor{Ref: node.Ref, Role: role, Name: name},
				CachedAt:   time.Now(),
			})
			return &bridge.ActionRequest{Kind: bridge.ActionClick, Ref: node.Ref, NodeID: node.NodeID}
		}
	}
	t.Fatalf("no %s named %q in the snapshot", role, name)
	return nil
}

func rerenderElement(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	js := `(() => { const el = document.getElementById("` + id + `"); el.outerHTML = el.outerHTML; })()`
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, nil)); err != nil {
		t.Fatal(err)
	}
}

func renderBody(t *testing.T, ctx context.Context, html string) {
	t.Helper()
	js := "document.body.innerHTML = " + strconv.Quote(html)
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, nil)); err != nil {
		t.Fatal(err)
	}
}

func clickedIDs(t *testing.T, ctx context.Context) []string {
	t.Helper()
	var ids []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.clicked`, &ids)); err != nil {
		t.Fatal(err)
	}
	return ids
}

// Replacing every interactive node starts a new ref epoch numbered from e0, so the ref
// the agent holds for "Apply coupon" now names whichever control renders first. The
// pointer-retry arm used to refresh the cache, look the same ref up and click there.
func TestAStaleRefInANewEpochIsNotDispatchedAtTheRenumberedControl(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := checkoutHandlers(t, f, "/renumber")
	click := cachedClick(t, h, ctx, tabID, "button", "Apply coupon")
	renderBody(t, ctx, renumberPage(true))

	result, _, rr, err := h.executeActionResilient(ctx, click, "", h.Config, tabID, false)

	if ids := clickedIDs(t, ctx); slices.Contains(ids, "b") {
		t.Fatalf("clicked %v: the stale ref was dispatched at the control that inherited its number", ids)
	}
	switch {
	case err == nil:
		if rr == nil || !rr.Recovered {
			t.Errorf("result %v with recovery %+v: a dispatch at a matched node must come through recovery", result, rr)
		}
	case errors.Is(err, ErrTargetNotFound):
		if !strings.Contains(err.Error(), "/snapshot") {
			t.Errorf("refusal %q does not tell the caller to re-snapshot", err)
		}
	default:
		t.Errorf("error = %v, want ref-not-found with the snapshot remedy or a recovered click", err)
	}
}

func TestAStaleRefRenumberedOntoASubmitControlPlacesNoOrder(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := checkoutHandlers(t, f, "/renumber")
	click := cachedClick(t, h, ctx, tabID, "button", "Apply coupon")
	renderBody(t, ctx, `<h1>Checkout</h1>`+
		`<form action="/order" method="post"><button id="p">Place order</button></form>`+
		`<a id="b" href="#">Browse catalogue</a>`)

	_, _, rr, err := h.executeActionResilient(ctx, click, "", h.Config, tabID, false)

	if placed := f.settleForOrder(); placed != 0 {
		t.Fatalf("%d order(s) placed by a click on a ref minted for a different control", placed)
	}
	if !errors.Is(err, ErrTargetNotFound) && !errors.Is(err, ErrStaleSubmitTarget) {
		t.Errorf("error = %v, want ref-not-found or the stale-submit refusal", err)
	}
	if rr != nil && rr.Recovered {
		t.Errorf("recovery reported a dispatch: %+v", rr)
	}
}

func TestACachedStaleRefOnASubmitControlIsRefusedBeforeAnyOrderIsPlaced(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := staleSubmitHandlers(t, f)
	click := cachedClick(t, h, ctx, tabID, "button", "Place order")
	rerenderElement(t, ctx, "b")

	_, _, rr, err := h.executeActionResilient(ctx, click, "", h.Config, tabID, false)

	if !errors.Is(err, ErrStaleSubmitTarget) {
		t.Fatalf("error = %v, want the stale-submit refusal", err)
	}
	if errors.Is(err, ErrTargetNotFound) {
		t.Errorf("the refusal was wrapped as ref-not-found: %v", err)
	}
	if rr != nil {
		t.Errorf("a recovery record was published for a click that never happened: %+v", rr)
	}
	if placed := f.settleForOrder(); placed != 0 {
		t.Errorf("%d order(s) placed by a call that refused", placed)
	}
}

func TestACachedStaleRefOnALinkStillRecoversAndClicks(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := staleSubmitHandlers(t, f)
	click := cachedClick(t, h, ctx, tabID, "link", "Read the terms")
	rerenderElement(t, ctx, "t")

	result, _, rr, err := h.executeActionResilient(ctx, click, "", h.Config, tabID, false)

	if err != nil {
		t.Fatalf("a cached stale ref on a link was not recovered: %v", err)
	}
	if rr == nil || !rr.Recovered {
		t.Errorf("recovery record = %+v, want recovered", rr)
	}
	if landed, _ := result[bridge.ResultLandedURL].(string); !strings.Contains(landed, "/terms") {
		t.Errorf("landed url = %v, want the link's own target", result[bridge.ResultLandedURL])
	}
	if placed := f.settleForOrder(); placed != 0 {
		t.Errorf("%d order(s) placed by a click on a link", placed)
	}
}

func TestTheEndpointRefusesACachedStaleSubmitRefWithNoOrderAndNoRecoveryRecord(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := staleSubmitHandlers(t, f)
	click := cachedClick(t, h, ctx, tabID, "button", "Place order")
	rerenderElement(t, ctx, "b")

	body := `{"kind":"click","ref":"` + click.Ref + `","tabId":"` + tabID + `"}`
	rec := httptest.NewRecorder()
	h.HandleAction(rec, httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(body)))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode refusal: %v (%s)", err, rec.Body.String())
	}
	if resp.Code != "submit_target_not_found" {
		t.Errorf("code = %q, want submit_target_not_found: %s", resp.Code, rec.Body.String())
	}
	if dispatched, ok := resp.Details["dispatched"].(bool); !ok || dispatched {
		t.Errorf("details.dispatched = %v, want false", resp.Details["dispatched"])
	}
	if strings.Contains(rec.Body.String(), "recovery") {
		t.Errorf("the refusal carries a recovery record for a click that never happened: %s", rec.Body.String())
	}
	if placed := f.settleForOrder(); placed != 0 {
		t.Errorf("%d order(s) placed by a refused request", placed)
	}
}

// The wire shape a caller actually sees. The helper-level tests above pin the behaviour at
// the shared owner; this pins that the refusal survives the endpoint as a 404 naming the
// snapshot, with dispatch state on it — and that following the printed remedy from the
// refused state posts nothing.
func TestTheEndpointRefusesAStaleSubmitRefWithASnapshotRemedyAndNoOrder(t *testing.T) {
	f := newOrderFixture(t)
	h, _, tabID := staleSubmitHandlers(t, f)

	body := `{"kind":"click","ref":"e2","tabId":"` + tabID + `"}`
	rec := httptest.NewRecorder()
	h.HandleAction(rec, httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(body)))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code    string         `json:"code"`
		Error   string         `json:"error"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode refusal: %v (%s)", err, rec.Body.String())
	}
	if resp.Code != "submit_target_not_found" {
		t.Errorf("code = %q, want submit_target_not_found", resp.Code)
	}
	if dispatched, ok := resp.Details["dispatched"].(bool); !ok || dispatched {
		t.Errorf("details.dispatched = %v, want false: this refusal exists to say nothing was clicked", resp.Details["dispatched"])
	}
	// Spelled out rather than compared against reSnapshot, which production formats from the
	// same constant: that comparison holds whatever the constant is changed to, so it pins
	// the two sides agreeing rather than the advice being the one that resolves a stale ref.
	if got, _ := resp.Details["remedy"].(string); got != "pinchtab snap" {
		t.Errorf("remedy = %q, want %q — the only advice that re-resolves a stale ref", got, "pinchtab snap")
	}
	if placed := f.settleForOrder(); placed != 0 {
		t.Fatalf("%d order(s) placed by a refused request", placed)
	}

	// Follow the remedy from the state it was printed in: a snapshot is a read, so the
	// advice cannot itself place the order the refusal just prevented.
	snapRec := httptest.NewRecorder()
	h.HandleSnapshot(snapRec, httptest.NewRequest(http.MethodGet, "/snapshot?tabId="+tabID, nil))
	if snapRec.Code != http.StatusOK {
		t.Fatalf("the printed remedy fails from the state it is printed in: %d %s", snapRec.Code, snapRec.Body.String())
	}
	if placed := f.settleForOrder(); placed != 0 {
		t.Errorf("%d order(s) placed by following the remedy", placed)
	}
}

// The helper returns the record; this pins that it survives the endpoint and reaches the
// caller. Without it the 200 reports the navigation and hides the substitution — and the
// helper-level pair cannot see that, because the field is attached one layer out.
func TestTheNavigationResponseDisclosesWhichElementWasActuallyClicked(t *testing.T) {
	f := newOrderFixture(t)
	h, _, tabID := staleSubmitHandlers(t, f)

	body := `{"kind":"click","ref":"e3","tabId":"` + tabID + `"}`
	rec := httptest.NewRecorder()
	h.HandleAction(rec, httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a click that ran and moved the page: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success  bool           `json:"success"`
		Result   map[string]any `json:"result"`
		Recovery map[string]any `json:"recovery"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode navigation response: %v (%s)", err, rec.Body.String())
	}
	if !resp.Success {
		t.Errorf("success = false for a click that landed: %s", rec.Body.String())
	}
	if resp.Result[bridge.ResultNavigated] != true {
		t.Errorf("result does not report the navigation: %v", resp.Result)
	}
	if resp.Recovery == nil {
		t.Fatalf("recovery absent, so the response says the page moved without saying the click went to a different element than the caller named: %s", rec.Body.String())
	}
	if newRef, _ := resp.Recovery["new_ref"].(string); newRef == "" {
		t.Errorf("recovery names no matched ref, so the substitution is still undisclosed: %v", resp.Recovery)
	}
}

// The macro/batch step path takes the same helper and the same refMissing argument, so the
// refusal has to arrive there as a failed step carrying the same advice — a step that
// reported success, or one whose text sent the caller back to --submit, would be the same
// defect one entry point over.
func TestTheStepPathTurnsTheRefusalIntoAFailedStepNamingTheSnapshot(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := staleSubmitHandlers(t, f)

	step := staleClick()
	result, _, _ := h.runResolvedActionStep(
		ctx, ctx,
		httptest.NewRequest(http.MethodPost, "/actions", strings.NewReader("{}")),
		httptest.NewRecorder(),
		step, "", h.Config, tabID, 0, true,
		func(err error) string { return err.Error() },
	)

	if result.Success {
		t.Fatalf("step reported success for a refused click: %+v", result)
	}
	if !strings.Contains(result.Error, "/snapshot") {
		t.Errorf("step error = %q, which does not tell the caller to re-snapshot", result.Error)
	}
	if strings.Contains(result.Error, "--submit") {
		t.Errorf("step error = %q recommends --submit, which cannot resolve a stale ref either", result.Error)
	}
	if placed := f.settleForOrder(); placed != 0 {
		t.Errorf("%d order(s) placed by a refused step", placed)
	}
}

// navigatingBridge dispatches the action and answers the guard sentinel — the bridge's own
// spelling of "the action ran, and then the page moved". Every dispatch is counted, because
// the defect is invisible in the returned error: both the fixed and the broken path answer
// the navigation, and only the count says whether a second, unrequested click went out.
type navigatingBridge struct {
	mockBridge
	refCache     *bridge.RefCache
	actionErr    error
	actionResult map[string]any
	failFirst    error
	dispatches   atomic.Int64
}

func (m *navigatingBridge) GetRefCache(string) *bridge.RefCache  { return m.refCache }
func (m *navigatingBridge) SetRefCache(string, *bridge.RefCache) {}

func (m *navigatingBridge) ExecuteAction(context.Context, string, bridge.ActionRequest) (map[string]any, error) {
	if m.dispatches.Add(1) == 1 && m.failFirst != nil {
		return nil, m.failFirst
	}
	return m.actionResult, m.actionErr
}

func navigatingHandlers(t *testing.T, actionErr error) (*navigatingBridge, *Handlers, string) {
	t.Helper()
	const tabID = "tab-nav-guard"
	mb := &navigatingBridge{
		mockBridge: mockBridge{availableActions: []string{bridge.ActionClick, bridge.ActionHover}},
		actionErr:  actionErr,
		refCache: &bridge.RefCache{
			Refs:  map[string]int64{"e6": 18},
			Nodes: []bridge.A11yNode{{Ref: "e6", Role: "link", Name: "Learn more", NodeID: 18}},
		},
	}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	h.Recovery.RecordIntent(tabID, "e6", recovery.IntentEntry{
		Descriptor: semantic.ElementDescriptor{Ref: "e6", Role: "link", Name: "Learn more"},
		CachedAt:   time.Now(),
	})
	return mb, h, tabID
}

// PIN-264 added a guard here that suppressed the post-execution heal when the error
// was the navigation sentinel. That sentinel no longer exists: a navigation is a
// SUCCESS now, so err is nil and the heal branch cannot run at all. The guard and its
// sentinel-shaped test were deleted together rather than left as a branch nothing can
// reach — but the property they protected is the reason this replacement exists: an
// action that ran and moved the page must dispatch exactly once.
func TestAnActionThatNavigatedIsNotDispatchedTwice(t *testing.T) {
	for _, kind := range []string{bridge.ActionClick, bridge.ActionHover} {
		t.Run(kind, func(t *testing.T) {
			mb, h, tabID := navigatingHandlers(t, nil)
			mb.actionResult = map[string]any{
				"ok":                     true,
				bridge.ResultNavigated:   true,
				bridge.ResultLandedURL:   "https://b.example/",
				bridge.ResultPreviousURL: "https://a.example/",
				bridge.ResultRefsStale:   true,
			}

			req := &bridge.ActionRequest{Kind: kind, Ref: "e6", NodeID: 18}
			result, _, rr, err := h.executeActionResilient(context.Background(), req, "", h.Config, tabID, false)

			if err != nil {
				t.Fatalf("an action that ran and moved the page reported failure: %v", err)
			}
			if result[bridge.ResultLandedURL] != "https://b.example/" {
				t.Errorf("the landed url did not reach the caller: %v", result)
			}
			if got := mb.dispatches.Load(); got != 1 {
				t.Errorf("%d dispatches for one request; re-matching a ref against the page the caller's own action navigated to is a second, unrequested action on the user's browser", got)
			}
			if rr != nil {
				t.Errorf("a recovery block was published for a request that clicked nothing else: %+v", rr)
			}
		})
	}
}

// The converse of the test above, and what stops that one from being read as "never
// heal anything a navigation touched": recovery's "navigation" failure class also
// carries a detached frame and a crashed page, where the dispatch never landed and
// re-dispatching is the whole point. Those still heal.
func TestANavigationClassifiedFailureThatNeverDispatchedStillHeals(t *testing.T) {
	for name, actionErr := range map[string]error{
		"a detached frame": errors.New("frame was detached"),
		"a crashed page":   errors.New("page crashed"),
	} {
		t.Run(name, func(t *testing.T) {
			mb, h, tabID := navigatingHandlers(t, actionErr)

			req := &bridge.ActionRequest{Kind: bridge.ActionClick, Ref: "e6", NodeID: 18}
			_, _, _, err := h.executeActionResilient(context.Background(), req, "", h.Config, tabID, false)

			if err == nil {
				t.Fatalf("%s reported success", name)
			}
			if got := mb.dispatches.Load(); got < 2 {
				t.Errorf("%d dispatch(es) for %s; recovery must still heal a failure whose action never took effect", got, name)
			}
		})
	}
}

// A ref carries nothing the retry arm can re-resolve: the node id stays the dead one,
// so a second dispatch there is a round trip spent to learn what the first already said.
// Recovery is the heal. The first dispatch fails stale; every later one succeeds, so the
// count says whether a doomed retry went out before recovery's single dispatch.
func TestAStaleRefIsNotRedispatchedAtTheDeadNodeBeforeRecovery(t *testing.T) {
	mb, h, tabID := navigatingHandlers(t, nil)
	mb.actionResult = map[string]any{"ok": true}
	mb.failFirst = errors.New("node is detached")

	req := &bridge.ActionRequest{Kind: bridge.ActionClick, Ref: "e6", NodeID: 18}
	_, _, rr, err := h.executeActionResilient(context.Background(), req, "", h.Config, tabID, false)

	if err != nil {
		t.Fatalf("recovery did not heal the stale ref: %v", err)
	}
	if rr == nil || !rr.Recovered {
		t.Fatalf("the click did not come through recovery: %+v", rr)
	}
	if got := mb.dispatches.Load(); got != 2 {
		t.Errorf("%d dispatches; want the failed one and recovery's, with no retry at the dead node between", got)
	}
}

// With a drag destination the retry IS the heal for that destination: recovery only
// re-matches the source, so the arm still re-resolves the destination and retries.
func TestAStaleDragWithADestinationStillRetriesBeforeRecovery(t *testing.T) {
	mb, h, tabID := navigatingHandlers(t, nil)
	mb.availableActions = append(mb.availableActions, bridge.ActionDrag)
	mb.actionResult = map[string]any{"ok": true}
	mb.failFirst = errors.New("node is detached")
	mb.refCache.Refs["e9"] = 77
	mb.refCache.Nodes = append(mb.refCache.Nodes, bridge.A11yNode{Ref: "e9", Role: "listitem", Name: "Done", NodeID: 77})
	h.Recovery.RecordIntent(tabID, "e9", recovery.IntentEntry{
		Descriptor: semantic.ElementDescriptor{Ref: "e9", Role: "listitem", Name: "Done"},
		CachedAt:   time.Now(),
	})

	req := &bridge.ActionRequest{Kind: bridge.ActionDrag, Ref: "e6", NodeID: 18, ToSelector: "e9", ToNodeID: 77}
	_, _, rr, err := h.executeActionResilient(context.Background(), req, "", h.Config, tabID, false)

	if err != nil {
		t.Fatalf("the retry did not heal the drag: %v", err)
	}
	if rr != nil {
		t.Errorf("recovery ran for a drag the retry arm healed: %+v", rr)
	}
	if got := mb.dispatches.Load(); got != 2 {
		t.Errorf("%d dispatches; want the failed one and the retry", got)
	}
}

// A CSS selector, unlike a ref, names its element by content rather than position, so the
// retry arm re-resolves it and dispatches at the node the re-render created. There is no
// ref for recovery to heal, so a retry at the dead node would be the final answer.
func TestAStaleCSSSelectorClickRedispatchesAtTheReResolvedNode(t *testing.T) {
	f := newOrderFixture(t)
	h, ctx, tabID := checkoutHandlers(t, f, "/")
	click := &bridge.ActionRequest{Kind: bridge.ActionClick, Selector: "#t"}
	if _, err := h.resolveActionRequestSelector(ctx, tabID, click); err != nil {
		t.Fatal(err)
	}
	deadNodeID := click.NodeID
	rerenderElement(t, ctx, "t")

	result, _, rr, err := h.executeActionResilient(ctx, click, "#t", h.Config, tabID, false)

	if err != nil {
		t.Fatalf("the stale CSS-selector click was not re-resolved: %v", err)
	}
	if click.NodeID == deadNodeID {
		t.Errorf("node id %d is the one the re-render destroyed", deadNodeID)
	}
	if landed, _ := result[bridge.ResultLandedURL].(string); !strings.Contains(landed, "/terms") {
		t.Errorf("landed url = %v, want the link's own target", result[bridge.ResultLandedURL])
	}
	if rr != nil {
		t.Errorf("recovery ran for a selector the retry arm re-resolved: %+v", rr)
	}
}
