package handlers

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func TestADragDestinationIsResolvedThroughTheSourceResolver(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := bridge.ActionRequest{Kind: bridge.ActionDrag, Selector: "#src", ToSelector: "e9"}

	_, err := h.resolveActionRequestDestination(context.Background(), "tab1", &req)

	if err == nil {
		t.Fatal("a destination selector resolved without any CDP context, so it was not routed through the resolver at all")
	}
	if !strings.Contains(err.Error(), "drag destination") {
		t.Errorf("error = %v, want it to name the end of the drag that failed", err)
	}
	if req.ToNodeID != 0 {
		t.Errorf("ToNodeID = %d, want 0 when resolution failed", req.ToNodeID)
	}
}

func TestAnAlreadyResolvedDragDestinationIsLeftAlone(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	for _, req := range []bridge.ActionRequest{
		{Kind: bridge.ActionDrag, Selector: "#src", ToSelector: "e9", ToNodeID: 77},
		{Kind: bridge.ActionDrag, Selector: "#src"},
		{Kind: bridge.ActionDrag, Selector: "#src", ToX: 400, ToY: 320, HasToXY: true},
	} {
		before := req
		if _, err := h.resolveActionRequestDestination(context.Background(), "tab1", &req); err != nil {
			t.Errorf("%+v: %v", before, err)
		}
		if req.ToNodeID != before.ToNodeID {
			t.Errorf("%+v: ToNodeID became %d", before, req.ToNodeID)
		}
	}
}

func TestTheActionHandlerResolvesADragDestination(t *testing.T) {
	h := New(&mockBridge{availableActions: []string{bridge.ActionDrag}}, &config.RuntimeConfig{}, nil, nil, nil)
	body := `{"kind":"drag","nodeId":42,"toSelector":"e9","tabId":"tab1"}`
	w := httptest.NewRecorder()

	h.HandleAction(w, httptest.NewRequest("POST", "/action", strings.NewReader(body)))

	if w.Code == 200 {
		t.Fatalf("the handler answered 200 for a destination it never resolved: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "drag destination") {
		t.Errorf("response = %s, want it to name the drag destination; without that the destination was skipped and the drag went to (0,0)", w.Body.String())
	}
}

func TestTheQueryFormOfActionCarriesTheDragDestination(t *testing.T) {
	req, ok := decodeActionRequest(httptest.NewRecorder(), httptest.NewRequest("GET", "/action?kind=drag&selector=%23src&toSelector=e9", nil))
	if !ok {
		t.Fatal("decode refused a drag query")
	}
	if req.ToSelector != "e9" {
		t.Errorf("ToSelector = %q, want e9", req.ToSelector)
	}

	point, ok := decodeActionRequest(httptest.NewRecorder(), httptest.NewRequest("GET", "/action?kind=drag&selector=%23src&toX=400&toY=320", nil))
	if !ok {
		t.Fatal("decode refused a drag query with a coordinate destination")
	}
	if point.ToX != 400 || point.ToY != 320 || !point.HasToXY {
		t.Errorf("destination = (%v,%v) hasToXY=%v, want (400,320) hasToXY=true", point.ToX, point.ToY, point.HasToXY)
	}
}

const dragRecoveryFixtureHTML = `<body style="margin:0">
<button id="a" style="position:absolute;left:10px;top:10px;width:120px;height:30px">Alpha</button>
<button id="b" style="position:absolute;left:10px;top:60px;width:120px;height:30px">Bravo</button>
<button id="c" style="position:absolute;left:10px;top:110px;width:120px;height:30px">Charlie</button>
<script>
window.__drops = [];
document.addEventListener('mouseup', e => {
	const el = document.elementFromPoint(e.clientX, e.clientY);
	window.__drops.push(el ? el.id : '');
});
</script>
</body>`

func newDragRecoveryFixture(t *testing.T) (context.Context, *bridge.Bridge, *Handlers) {
	t.Helper()
	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	t.Cleanup(cancelTimeout)
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(dragRecoveryFixtureHTML)))); err != nil {
		t.Fatal(err)
	}
	cfg := &config.RuntimeConfig{ActionTimeout: 45 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := browserBridge(ctx, cfg)
	b.RegisterTab("tab-drag", ctx)
	h := New(b, cfg, nil, nil, nil)
	return ctx, b, h
}

func seedDragSnapshot(t *testing.T, ctx context.Context, b *bridge.Bridge) {
	t.Helper()
	nids := map[string]int64{}
	for id := range map[string]bool{"a": true, "b": true} {
		nid, err := bridge.ResolveCSSToNodeID(ctx, "#"+id)
		if err != nil {
			t.Fatalf("resolve #%s: %v", id, err)
		}
		nids[id] = nid
	}
	b.SetRefCache("tab-drag", &bridge.RefCache{
		Targets: map[string]bridge.RefTarget{
			"e5": {BackendNodeID: nids["a"]},
			"e9": {BackendNodeID: nids["b"]},
		},
		Nodes: []bridge.A11yNode{
			{Ref: "e5", Role: "button", Name: "Alpha", NodeID: nids["a"]},
			{Ref: "e9", Role: "button", Name: "Bravo", NodeID: nids["b"]},
		},
	})
}

func recreateDragElements(t *testing.T, ctx context.Context, ids ...string) {
	t.Helper()
	for _, id := range ids {
		script := `(() => { const el = document.getElementById("` + id + `");
			const clone = el.cloneNode(true); el.remove(); document.body.appendChild(clone); })()`
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, nil)); err != nil {
			t.Fatal(err)
		}
	}
}

func recordedDrops(t *testing.T, ctx context.Context) []string {
	t.Helper()
	var drops []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__drops`, &drops)); err != nil {
		t.Fatal(err)
	}
	return drops
}

func postDrag(t *testing.T, h *Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.HandleAction(w, httptest.NewRequest("POST", "/action", strings.NewReader(body)))
	return w
}

func TestADragWithAStaleDestinationRecoversAndLandsOnIt(t *testing.T) {
	ctx, b, h := newDragRecoveryFixture(t)
	seedDragSnapshot(t, ctx, b)
	recreateDragElements(t, ctx, "b")

	w := postDrag(t, h, `{"kind":"drag","selector":"#a","toSelector":"e9","tabId":"tab-drag"}`)

	if w.Code != 200 {
		t.Fatalf("stale destination did not recover: status=%d body=%s", w.Code, w.Body.String())
	}
	drops := recordedDrops(t, ctx)
	if len(drops) == 0 || drops[len(drops)-1] != "b" {
		t.Fatalf("drop landed on %v, want the re-created #b", drops)
	}
}

func TestADragWithBothEndsStaleRecoversBothFromOneSnapshot(t *testing.T) {
	ctx, b, h := newDragRecoveryFixture(t)
	seedDragSnapshot(t, ctx, b)
	recreateDragElements(t, ctx, "a", "b")

	w := postDrag(t, h, `{"kind":"drag","selector":"ref:e5","toSelector":"e9","tabId":"tab-drag"}`)

	if w.Code != 200 {
		t.Fatalf("both-ends-stale drag did not recover: status=%d body=%s", w.Code, w.Body.String())
	}
	drops := recordedDrops(t, ctx)
	if len(drops) == 0 || drops[len(drops)-1] != "b" {
		t.Fatalf("drop landed on %v, want the re-created #b", drops)
	}
}

func TestADragDestinationGoneAfterRefreshStillRefuses(t *testing.T) {
	ctx, b, h := newDragRecoveryFixture(t)
	seedDragSnapshot(t, ctx, b)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("b").remove()`, nil)); err != nil {
		t.Fatal(err)
	}

	w := postDrag(t, h, `{"kind":"drag","selector":"#a","toSelector":"e9","tabId":"tab-drag"}`)

	if w.Code == 200 {
		t.Fatalf("a drag to a destination that no longer exists reported success: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "drag destination e9") {
		t.Errorf("response = %s, want the refusal to name the destination, not the source", w.Body.String())
	}
	if drops := recordedDrops(t, ctx); len(drops) != 0 {
		t.Errorf("a refused drag still dropped on %v", drops)
	}
}

func TestEveryReResolutionSiteRefreshesSecondaryTargets(t *testing.T) {
	pkg := srccensus.Load(t, ".", 80)
	pkg.MustFunc(t, "refreshActionSecondaryTargets")
	resilient := pkg.Source(pkg.MustFunc(t, "executeActionResilient"))
	if got := strings.Count(resilient, "h.refreshActionSecondaryTargets("); got != 3 {
		t.Errorf("executeActionResilient calls refreshActionSecondaryTargets %d times, want 3 — the recovery-first callback, the pointer retry, and the post-failure heal each re-resolve after the snapshot may have changed", got)
	}

	if !strings.Contains(pkg.Source(pkg.MustFunc(t, "resolveActionDestination")), "h.refreshActionSecondaryTargets(") {
		t.Error("HandleAction no longer routes a missing drag destination through the shared re-resolution; the unconditional 404 this card removed is the likely regression")
	}
}
