package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
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

func TestHandleCapture_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/capture", nil)
	w := httptest.NewRecorder()
	h.HandleCapture(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleCapture_UnknownOutput(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/capture?output=carrier-pigeon", nil)
	w := httptest.NewRecorder()
	h.HandleCapture(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleTabCapture_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//capture", nil)
	w := httptest.NewRecorder()
	h.HandleTabCapture(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabCapture_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/capture", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabCapture(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleCapture_RouteMounted(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	for _, path := range []string{"/capture", "/tabs/abc/capture"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404 from handler, got %d body=%s", path, w.Code, w.Body.String())
		}
		// Empty body would mean the mux didn't match the path.
		if !strings.Contains(w.Body.String(), "tab") {
			t.Errorf("%s: expected handler-shaped 404 body, got %q", path, w.Body.String())
		}
	}
}

func TestHandleCapture_WaitParamParses(t *testing.T) {
	for _, v := range []string{"stable", "load", "none", ""} {
		h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
		req := httptest.NewRequest("GET", "/capture?wait="+v, nil)
		w := httptest.NewRecorder()
		h.HandleCapture(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("wait=%q: expected 404 (tab not found), got %d", v, w.Code)
		}
	}
}

func TestHandleCapture_BoundsAndBeyondViewportParse(t *testing.T) {
	cases := []string{
		"/capture?withBounds=true&beyondViewport=true",
		"/capture?withBounds=false",
		"/capture?beyondViewport=1",
		"/capture?withBounds=0&beyondViewport=0",
		"/capture?scale=0.5",
		"/capture?scale=2.0",
		"/capture?scale=not-a-number",
	}
	for _, url := range cases {
		h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
		req := httptest.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		h.HandleCapture(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404 (tab not found), got %d", url, w.Code)
		}
	}
}

func TestHandleCapture_OpenAPIExposes(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	h.HandleOpenAPI(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/capture") {
		t.Fatalf("expected /openapi.json to list /capture, got %s", w.Body.String())
	}
}

// capturePairFixtureHTML puts one button above the fold and one far below it, so
// a single scroll leaves one measured-and-on-screen and one
// measured-and-off-screen — the two states the wire could not tell apart while
// the field was an omitempty bool. Scrolled past ABOVE is the direction the
// original report checked last, and it must serialise false like any other
// off-screen node.
const capturePairFixtureHTML = `<body style="margin:0;height:4000px">
<button id="top" style="position:absolute;top:0;left:0">TopBtn</button>
<button id="bottom" style="position:absolute;top:2500px;left:0">BotBtn</button>
<input type="checkbox" checked aria-label="OnBox" style="position:absolute;top:2600px;left:0">
<input type="checkbox" aria-label="OffBox" style="position:absolute;top:2640px;left:0">
<div role="checkbox" aria-checked="mixed" tabindex="0" aria-label="MixedBox" style="position:absolute;top:2680px;left:0"></div>
</body>`

const capturePairTabID = "tab-capture-visible"

// captureBoundsParam is the query key that turns the bounds pass off. The two wire tests
// below build their queries from it, and TestTheBoundsQueryKeyIsOneTheHandlerReads checks
// production reads this exact spelling — a browserless check on purpose: these tests skip
// where local Chrome cannot screenshot, and a test that drives a key the handler ignores
// then reads as a pass instead of asserting anything. That is how "&bounds=false" survived
// here while the handler was looking for "withBounds".
const captureBoundsParam = "withBounds"

type captureFixture struct {
	handlers *Handlers
}

func newCaptureFixture(t *testing.T) captureFixture {
	t.Helper()
	chromePath := testbrowser.Path(t)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(capturePairFixtureHTML))
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
	})

	var scrolled bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#bottom", chromedp.ByID),
		chromedp.Evaluate(`(() => { window.scrollTo(0, 2423); return window.scrollY > 2000; })()`, &scrolled),
	); err != nil {
		t.Fatal(err)
	}
	if !scrolled {
		t.Fatal("fixture did not scroll; both buttons would report the same visibility")
	}

	cfg := &config.RuntimeConfig{ActionTimeout: 10 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := bridge.New(context.Background(), ctx, cfg)
	b.RegisterTab(capturePairTabID, ctx)

	return captureFixture{handlers: New(b, cfg, nil, nil, nil)}
}

// capture drives the real endpoint and returns the snapshot nodes as they were
// SERIALISED, not as Go structs: the defect lived entirely in the encoding, so
// decoding into a typed struct would restore what the wire had lost.
func (f captureFixture) capture(t *testing.T, query string) []map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	f.handlers.HandleCapture(rec, httptest.NewRequest(http.MethodGet, "/capture?output=inline&format=png&tabId="+capturePairTabID+query, nil))
	if rec.Code != http.StatusOK {
		if strings.Contains(rec.Body.String(), "Unable to capture screenshot") {
			t.Skipf("local Chrome cannot capture screenshots: %s", rec.Body.String())
		}
		t.Fatalf("capture status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Snapshot struct {
			Nodes []map[string]any `json:"nodes"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode capture response: %v (body %s)", err, rec.Body.String())
	}
	if len(resp.Snapshot.Nodes) == 0 {
		t.Fatal("capture returned no snapshot nodes")
	}
	return resp.Snapshot.Nodes
}

func nodeByName(t *testing.T, nodes []map[string]any, name string) map[string]any {
	t.Helper()
	for _, node := range nodes {
		if node["name"] == name {
			return node
		}
	}
	t.Fatalf("no node named %q in the capture snapshot (%d nodes)", name, len(nodes))
	return nil
}

// The reported defect on the wire: the off-screen button carried no visible key
// at all, so a client could not tell "measured, off-screen" from "never
// measured". Both buttons are measured here, so both must state their answer.
func TestCaptureSerialisesMeasuredVisibleFalse(t *testing.T) {
	nodes := newCaptureFixture(t).capture(t, "&"+captureBoundsParam+"=true")

	offScreen := nodeByName(t, nodes, "TopBtn")
	value, ok := offScreen["visible"]
	if !ok {
		t.Fatalf("the scrolled-past button published no visible key: %v", offScreen)
	}
	if value != false {
		t.Errorf("scrolled-past button visible = %v, want false", value)
	}

	onScreen := nodeByName(t, nodes, "BotBtn")
	if onScreen["visible"] != true {
		t.Errorf("on-screen button visible = %v, want true", onScreen["visible"])
	}

	for i, node := range nodes {
		_, hasVisible := node["visible"]
		_, hasBounds := node["boundingBox"]
		if hasVisible != hasBounds {
			t.Errorf("node %d (%v): visible present=%v but boundingBox present=%v", i, node["ref"], hasVisible, hasBounds)
		}
	}
}

// bounds=false measures nothing, so no node may claim a visibility it was never
// asked to compute.
func TestCaptureWithoutBoundsPublishesNoVisibleKey(t *testing.T) {
	nodes := newCaptureFixture(t).capture(t, "&"+captureBoundsParam+"=false")

	for i, node := range nodes {
		if _, ok := node["visible"]; ok {
			t.Errorf("node %d (%v) published visible without a bounds pass: %v", i, node["ref"], node)
		}
		if _, ok := node["boundingBox"]; ok {
			t.Errorf("node %d (%v) published boundingBox with bounds=false: %v", i, node["ref"], node)
		}
	}
}

func TestCaptureSerialisesCheckedStateForCheckableControls(t *testing.T) {
	nodes := newCaptureFixture(t).capture(t, "&"+captureBoundsParam+"=true")

	for name, want := range map[string]string{"OnBox": "true", "OffBox": "false", "MixedBox": "mixed"} {
		if got := nodeByName(t, nodes, name)["checked"]; got != want {
			t.Errorf("%s checked = %v, want %q", name, got, want)
		}
	}

	for _, name := range []string{"TopBtn", "BotBtn"} {
		if value, ok := nodeByName(t, nodes, name)["checked"]; ok {
			t.Errorf("%s has no checkedness but published checked = %v", name, value)
		}
	}
}

// A skipped test asserts nothing, so the query key the two tests above depend on is pinned
// where no browser is involved: the handler must read this exact parameter, or bounds are
// computed regardless of what the query says and the unmeasured case is never exercised.
func TestTheBoundsQueryKeyIsOneTheHandlerReads(t *testing.T) {
	src := captureSource(t)
	needle := `q.Get("` + captureBoundsParam + `")`
	if !strings.Contains(src, needle) {
		t.Fatalf("the capture handler's file does not read %s; the wire tests would drive a parameter the handler ignores, measure bounds anyway, and their unmeasured case would never run", needle)
	}
}

// The two tests above are the real proof, but they need a Chrome that can
// screenshot, and they skip where it cannot. Their claim only transfers to the
// wire while the envelope embeds the snapshot nodes verbatim: a hand-built node
// map could drop visible again with every observe-level test still green.
func TestCaptureEnvelopeEmbedsSnapshotNodesVerbatim(t *testing.T) {
	if !strings.Contains(captureSource(t), `"nodes":     result.Nodes`) {
		t.Fatal("the capture envelope no longer embeds result.Nodes directly; re-mapping nodes into a literal drops whatever field the map forgets")
	}
}

func captureSource(t *testing.T) string {
	t.Helper()
	pkg := srccensus.Load(t, ".", 80)
	return pkg.FileSource(pkg.MustFunc(t, "HandleCapture"))
}

// visible drives GET /visible for a ref the capture just published, on the same
// tab and at the same scroll position.
func (f captureFixture) visible(t *testing.T, ref string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	f.handlers.HandleGetVisible(rec, httptest.NewRequest(http.MethodGet, "/visible?ref="+ref+"&tabId="+capturePairTabID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/visible?ref=%s status %d: %s", ref, rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /visible response: %v (body %s)", err, rec.Body.String())
	}
	return body
}

// Two surfaces publish a boolean called visible for the same node and they
// answer different questions: GET /visible is CSS rendered-ness with scroll
// position as no input, the capture snapshot's field is viewport intersection.
// Both answers are wanted, so this fixture holds them apart — a later change
// that quietly aligns the two predicates reds here. The endpoint's own onScreen
// field is the bridge between them and must agree with capture node for node.
func TestTheVisibleEndpointAndCaptureAnswerDifferentQuestions(t *testing.T) {
	f := newCaptureFixture(t)
	nodes := f.capture(t, "&"+captureBoundsParam+"=true")

	for _, tc := range []struct {
		name            string
		wantCapture     bool
		wantRendered    bool
		wantOnScreen    bool
		whyTheyDisagree string
	}{
		{
			name: "TopBtn", wantCapture: false, wantRendered: true, wantOnScreen: false,
			whyTheyDisagree: "scrolled past above: rendered by CSS, not intersecting the viewport",
		},
		{
			name: "BotBtn", wantCapture: true, wantRendered: true, wantOnScreen: true,
			whyTheyDisagree: "on screen: both questions answer true, so the divergence above is not a blanket disagreement",
		},
	} {
		node := nodeByName(t, nodes, tc.name)
		if node["visible"] != tc.wantCapture {
			t.Fatalf("%s: capture visible = %v, want %v (%s)", tc.name, node["visible"], tc.wantCapture, tc.whyTheyDisagree)
		}

		ref, _ := node["ref"].(string)
		if ref == "" {
			t.Fatalf("%s: capture node published no ref: %v", tc.name, node)
		}
		body := f.visible(t, ref)

		if body["visible"] != tc.wantRendered {
			t.Errorf("%s: /visible visible = %v, want %v — the endpoint answers CSS rendered-ness and scroll position is not an input (%s)",
				tc.name, body["visible"], tc.wantRendered, tc.whyTheyDisagree)
		}
		onScreen, ok := body["onScreen"]
		if !ok {
			t.Fatalf("%s: /visible published no onScreen for a node capture measured: %v", tc.name, body)
		}
		if onScreen != tc.wantOnScreen {
			t.Errorf("%s: /visible onScreen = %v, want %v", tc.name, onScreen, tc.wantOnScreen)
		}
		if onScreen != node["visible"] {
			t.Errorf("%s: /visible onScreen = %v but capture visible = %v; they must be the same predicate", tc.name, onScreen, node["visible"])
		}
	}
}
