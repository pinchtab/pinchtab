package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func TestHandleGetVisible_SelectorParamEquivalentToRef(t *testing.T) {
	newH := func() *Handlers {
		mb := &mockBridge{
			refCache: &bridge.RefCache{
				Refs:    map[string]int64{"e1": 100},
				Targets: map[string]bridge.RefTarget{"e1": {BackendNodeID: 100}},
			},
		}
		return New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	}

	// Both the unified-selector `selector` param and the legacy `ref` alias must
	// feed the same resolver: an unknown ref via either param yields the same 404.
	for _, q := range []string{"selector=e99", "ref=e99"} {
		req := httptest.NewRequest("GET", "/visible?"+q, nil)
		w := httptest.NewRecorder()
		newH().HandleGetVisible(w, req)
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "e99") {
			t.Fatalf("%s: expected 404 mentioning e99, got %d: %s", q, w.Code, w.Body.String())
		}
	}

	// `selector` takes precedence over `ref` when both are present.
	req := httptest.NewRequest("GET", "/visible?selector=e99&ref=e1", nil)
	w := httptest.NewRecorder()
	newH().HandleGetVisible(w, req)
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "e99") {
		t.Fatalf("expected selector precedence (404 mentioning e99), got %d: %s", w.Code, w.Body.String())
	}
}

type visibleMockBridge struct {
	mockBridge
	rendered bool
}

func (m *visibleMockBridge) CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return json.Unmarshal([]byte(strconv.FormatBool(m.rendered)), result)
}

const visibleFixtureHTML = `<style>
#fixedByStylesheet { position: fixed; top:10px; left:10px; width:120px; height:40px }
#stickyByStylesheet { position: sticky; top:0; width:120px; height:40px }
#opaqueByStylesheet { position: fixed; top:200px; left:10px; width:120px; height:40px; opacity: 0 }
</style>
<div id="fixedByStylesheet">fixed by stylesheet</div>
<div id="fixedInline" style="position:fixed; top:80px; left:10px; width:120px; height:40px">fixed inline</div>
<div id="stickyByStylesheet">sticky by stylesheet</div>
<div id="stickyInline" style="position:sticky; top:0; width:120px; height:40px">sticky inline</div>
<div id="normalFlow" style="width:120px; height:40px">normal flow</div>
<div id="opaqueByStylesheet">fixed but transparent</div>
<div id="displayNone" style="display:none">display none</div>
<div id="visibilityHidden" style="visibility:hidden; width:120px; height:40px">visibility hidden</div>
<div id="opacityZero" style="opacity:0; width:120px; height:40px">opacity zero</div>
<div id="zeroSize" style="width:0; height:0">zero size</div>`

func runElementVisibleJS(t *testing.T, ctx context.Context, selector string) bool {
	t.Helper()

	var nodes []*cdp.Node
	if err := chromedp.Run(ctx, chromedp.Nodes(selector, &nodes, chromedp.ByQuery)); err != nil {
		t.Fatalf("%s: %v", selector, err)
	}
	if len(nodes) == 0 {
		t.Fatalf("%s matched nothing in the fixture", selector)
	}

	var visible bool
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		object, err := dom.ResolveNode().WithBackendNodeID(nodes[0].BackendNodeID).Do(ctx)
		if err != nil {
			return err
		}
		result, exception, err := runtime.CallFunctionOn(elementVisibleJS).WithObjectID(object.ObjectID).WithReturnByValue(true).Do(ctx)
		if err != nil {
			return err
		}
		if exception != nil {
			return exception
		}
		return json.Unmarshal(result.Value, &visible)
	})); err != nil {
		t.Fatalf("%s: %v", selector, err)
	}
	return visible
}

func TestTheVisibleEndpointReadsThePositionCSSComputed(t *testing.T) {

	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	t.Cleanup(cancelTimeout)

	url := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(visibleFixtureHTML))
	if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		selector string
		want     bool
		why      string
	}{
		{selector: "#fixedByStylesheet", want: true, why: "fixed by a stylesheet rule: the case that reported not visible while on screen"},
		{selector: "#fixedInline", want: true, why: "fixed by the style attribute: what worked before, and must not regress"},
		{selector: "#stickyByStylesheet", want: true, why: "sticky by a stylesheet rule"},
		{selector: "#stickyInline", want: true, why: "sticky by the style attribute"},
		{selector: "#normalFlow", want: true, why: "an ordinary in-flow element never needed the positioned branch"},
		{selector: "#displayNone", want: false, why: "display:none"},
		{selector: "#visibilityHidden", want: false, why: "visibility:hidden"},
		{selector: "#opacityZero", want: false, why: "opacity:0"},
		{selector: "#opaqueByStylesheet", want: false, why: "fixed and positioned, but transparent: reaching the positioned branch must not skip the hidden checks"},
		{selector: "#zeroSize", want: false, why: "zero-size box"},
	} {
		if got := runElementVisibleJS(t, ctx, tc.selector); got != tc.want {
			t.Errorf("%s reported visible=%v, want %v — %s", tc.selector, got, tc.want, tc.why)
		}
	}
}

// The browser-backed table above does not run where no browser is installed.
func TestTheVisiblePredicateNeverReadsTheInlineStyleAttribute(t *testing.T) {
	if strings.Contains(elementVisibleJS, "el.style.") {
		t.Errorf("the visible predicate reads the inline style attribute:\n%s", elementVisibleJS)
	}
	if !strings.Contains(elementVisibleJS, "style.position") {
		t.Errorf("the visible predicate no longer consults a position at all, so a fixed element falls to offsetParent alone:\n%s", elementVisibleJS)
	}
	if got := strings.Count(elementVisibleJS, "getComputedStyle"); got != 1 {
		t.Errorf("getComputedStyle appears %d times, want exactly 1; the position must come from the call the predicate already makes:\n%s", got, elementVisibleJS)
	}
	if strings.Index(elementVisibleJS, "getComputedStyle") > strings.Index(elementVisibleJS, "style.position") {
		t.Errorf("style.position is read before getComputedStyle assigns it:\n%s", elementVisibleJS)
	}
}

// The browser-backed divergence test lives in capture_test.go and skips where no
// browser is installed, so the property it protects is also stated here, where a
// browser is never needed: the endpoint predicate must not learn about scroll.
func TestTheVisiblePredicateNeverConsultsScrollOrViewport(t *testing.T) {
	for _, banned := range []string{"innerHeight", "innerWidth", "scrollY", "scrollX", "pageYOffset", "pageXOffset", "IntersectionObserver"} {
		if strings.Contains(elementVisibleJS, banned) {
			t.Errorf("the visible predicate reads %s, so it has become an on-screen check; that answer belongs to onScreen via bridge.IsOnScreen:\n%s", banned, elementVisibleJS)
		}
	}
}

// onScreen must stay the capture path's own predicate rather than a second
// implementation that can drift from it — this card exists because two spellings
// of one question disagreed.
func TestTheOnScreenAnswerDelegatesToTheCapturePredicate(t *testing.T) {
	pkg := srccensus.Load(t, ".", 50)
	onScreen, ok := pkg.Func("elementOnScreen")
	if !ok {
		t.Fatalf("elementOnScreen not found in %s; pin this property against whatever answers the visible endpoint's onScreen now", pkg.Dir())
	}
	for _, capture := range []string{"bridge.IsOnScreen", "bridge.ElementBorderBox", "bridge.FetchLayout"} {
		delegated := false
		for _, site := range pkg.Calls(t, capture) {
			delegated = delegated || pkg.Contains(onScreen, site)
		}
		if !delegated {
			t.Errorf("%s no longer calls %s; the endpoint would answer on-screen-ness with its own predicate and could disagree with the capture snapshot", onScreen.Name, capture)
		}
	}
}

func TestVisibleResponseOmitsOnScreenWhenUnmeasured(t *testing.T) {
	measured := false
	for _, tc := range []struct {
		name     string
		response visibleResponse
		wantKey  bool
		wantVal  any
	}{
		{name: "unmeasured", response: visibleResponse{Ref: "e1", Visible: true}, wantKey: false},
		{name: "measured false", response: visibleResponse{Ref: "e1", Visible: true, OnScreen: &measured}, wantKey: true, wantVal: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.response)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			value, ok := decoded["onScreen"]
			if ok != tc.wantKey {
				t.Fatalf("onScreen present = %v, want %v: absent means not measured, and a real false must stay visible (%s)", ok, tc.wantKey, encoded)
			}
			if tc.wantKey && value != tc.wantVal {
				t.Fatalf("onScreen = %v, want %v", value, tc.wantVal)
			}
		})
	}
}

// An element the endpoint can evaluate but cannot measure still gets its
// rendered-ness answer; the missing on-screen answer never turns into a false.
func TestHandleGetVisible_AnswersRenderedWhenNothingCanBeMeasured(t *testing.T) {
	mb := &visibleMockBridge{
		rendered: true,
		mockBridge: mockBridge{refCache: &bridge.RefCache{
			Refs:    map[string]int64{"e1": 100},
			Targets: map[string]bridge.RefTarget{"e1": {BackendNodeID: 100}},
		}},
	}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/visible?ref=e1", nil)
	w := httptest.NewRecorder()
	h.HandleGetVisible(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["visible"] != true {
		t.Fatalf("visible = %v, want true: %s", body["visible"], w.Body.String())
	}
	if _, ok := body["onScreen"]; ok {
		t.Fatalf("onScreen answered without a measurement: %s", w.Body.String())
	}
}
