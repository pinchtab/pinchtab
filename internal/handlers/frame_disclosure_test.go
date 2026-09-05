package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// frameIdentityBridge answers the in-frame identity read, which is how the disclosure learns
// what the content it is describing actually belongs to.
type frameIdentityBridge struct {
	*mockBridge
	askedFrameID string
	title, url   string
	fail         bool
}

func (b *frameIdentityBridge) EvaluateInFrame(_ context.Context, frameID string, _ string, result any, _ bridge.EvalOpts) error {
	b.askedFrameID = frameID
	if b.fail {
		return context.Canceled
	}
	payload, err := json.Marshal(map[string]string{"title": b.title, "url": b.url})
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, result)
}

func newScopedHandlers(t *testing.T, scope bridge.FrameScope, identity *frameIdentityBridge) *Handlers {
	t.Helper()
	mb := &mockBridge{frameScopes: map[string]bridge.FrameScope{"tab1": scope}}
	identity.mockBridge = mb
	return New(identity, &config.RuntimeConfig{}, nil, nil, nil)
}

// The common case must gain nothing: an unscoped read has no frame to disclose, and a payload
// that grew a key for every whole-document read would be a change to every caller.
func TestFrameDisclosureIsAbsentForAWholeDocumentRead(t *testing.T) {
	h := newScopedHandlers(t, bridge.FrameScope{}, &frameIdentityBridge{})

	if got := h.frameDisclosureFor(context.Background(), "tab1", ""); got != nil {
		t.Errorf("frameDisclosureFor = %+v, want nil for an unscoped read", got)
	}
	if got := (*frameDisclosure)(nil).attach(map[string]any{"url": "u"}); len(got) != 1 {
		t.Errorf("attach added %v to an unscoped payload", got)
	}
	if marker := (*frameDisclosure)(nil).marker(); marker != "" {
		t.Errorf("marker = %q, want empty for an unscoped read", marker)
	}
}

// The disclosure has to carry the frame's OWN url and title: the complaint this closes is
// attribution, and an id alone does not tell a reader which document the content came from.
func TestFrameDisclosureCarriesTheFramesOwnIdentity(t *testing.T) {
	identity := &frameIdentityBridge{title: "Inner", url: "http://127.0.0.1:1/inner.html"}
	h := newScopedHandlers(t, bridge.FrameScope{
		FrameID:   "frame-1",
		FrameURL:  "http://127.0.0.1:1/stale.html",
		FrameName: "payment",
		OwnerRef:  "e3",
	}, identity)

	got := h.frameDisclosureFor(context.Background(), "tab1", "frame-1")
	if got == nil {
		t.Fatal("a scoped read disclosed nothing")
	}
	if identity.askedFrameID != "frame-1" {
		t.Errorf("the identity was read in frame %q, want the frame the read was served from", identity.askedFrameID)
	}
	if got.FrameTitle != "Inner" {
		t.Errorf("frameTitle = %q, want the frame's own title", got.FrameTitle)
	}
	// The live url wins over the stored one: the scope records the url the frame had when it
	// was set, and a frame that navigated since would be attributed to a document it no
	// longer holds.
	if got.FrameURL != "http://127.0.0.1:1/inner.html" {
		t.Errorf("frameUrl = %q, want the frame's url at read time", got.FrameURL)
	}
	if got.OwnerRef != "e3" || got.FrameName != "payment" {
		t.Errorf("disclosure lost what the stored scope knows: %+v", got)
	}
}

// A cross-origin or torn-down frame cannot be read. The disclosure is still the honest half
// of the answer — the read WAS scoped — so it is published with what the scope knows.
func TestFrameDisclosureSurvivesAFrameItCannotRead(t *testing.T) {
	h := newScopedHandlers(t, bridge.FrameScope{FrameID: "frame-1", FrameURL: "http://stored/"},
		&frameIdentityBridge{fail: true})

	got := h.frameDisclosureFor(context.Background(), "tab1", "frame-1")
	if got == nil {
		t.Fatal("an unreadable frame suppressed the disclosure entirely, so the read looks like a whole-document one")
	}
	if got.FrameURL != "http://stored/" || got.FrameTitle != "" {
		t.Errorf("disclosure = %+v, want the stored url and no invented title", got)
	}
}

// The header is the surface the CLI prints, so this is where a scoped read becomes visible to
// a human reading a terminal. url and title keep meaning the TAB's document in both.
func TestSnapshotHeadersMarkAScopedReadAndLeaveAnUnscopedOneAlone(t *testing.T) {
	const title, pageURL = "Outer", "http://127.0.0.1:18798/"
	scope := &frameDisclosure{FrameScope: bridge.FrameScope{FrameID: "886601397BFA0B332880152438BD0153", OwnerRef: "e3"}}

	unscopedCompact := snapshotCompactHeader(title, pageURL, 7, nil)
	if unscopedCompact != "# Outer | http://127.0.0.1:18798/ | 7 nodes" {
		t.Errorf("an unscoped compact header changed shape: %q", unscopedCompact)
	}
	scopedCompact := snapshotCompactHeader(title, pageURL, 3, scope)
	if scopedCompact == unscopedCompact {
		t.Error("the two headers are identical, so a scoped read is indistinguishable from a whole-document one")
	}
	if !strings.Contains(scopedCompact, "frame e3") {
		t.Errorf("scoped compact header = %q, want it to name the frame", scopedCompact)
	}
	if !strings.HasPrefix(scopedCompact, "# Outer | http://127.0.0.1:18798/ |") {
		t.Errorf("scoped compact header re-pointed title or url at the frame: %q", scopedCompact)
	}

	unscopedText := snapshotTextHeader(title, pageURL, 7, nil)
	if unscopedText != "# Outer\n# http://127.0.0.1:18798/\n# 7 nodes" {
		t.Errorf("an unscoped text header changed shape: %q", unscopedText)
	}
	if !strings.Contains(snapshotTextHeader(title, pageURL, 3, scope), "# frame e3\n") {
		t.Errorf("scoped text header does not name the frame: %q", snapshotTextHeader(title, pageURL, 3, scope))
	}
}

// The owner ref is preferred because it is the handle `pinchtab frame <target>` accepts; a
// raw frame id is not a target that command resolves, so a header carrying only the id names
// something the reader cannot act on. Without a known ref the id is still better than silence.
func TestTheHeaderMarkerPrefersTheHandleTheFrameCommandAccepts(t *testing.T) {
	withRef := &frameDisclosure{FrameScope: bridge.FrameScope{FrameID: "886601397BFA0B33", OwnerRef: "e3"}}
	if got := withRef.marker(); got != "frame e3" {
		t.Errorf("marker = %q, want the owner ref", got)
	}
	withoutRef := &frameDisclosure{FrameScope: bridge.FrameScope{FrameID: "886601397BFA0B332880152438BD0153"}}
	got := withoutRef.marker()
	if !strings.HasPrefix(got, "frame 886601397BFA") {
		t.Errorf("marker = %q, want it to name the frame id", got)
	}
	if len(got) > len("frame ")+16 {
		t.Errorf("marker = %q, want the id shortened for a header line", got)
	}
}

// The text envelope is the other half of the reported defect. Two reads of the same tab and
// url differ in the disclosure, not only in what the text says.
func TestTextEnvelopeDisclosesTheFrameOnlyWhenScoped(t *testing.T) {
	extraction := textExtraction{Text: "inner frame paragraph", Mode: extractionRaw}

	unscoped := decodeTextEnvelope(t, scopedTextResponseRecorder(t, extraction, -1, "", nil))
	if _, ok := unscoped["frame"]; ok {
		t.Errorf("an unscoped text read published a frame key: %v", unscoped["frame"])
	}

	scoped := decodeTextEnvelope(t, scopedTextResponseRecorder(t, extraction, -1, "", &frameDisclosure{
		FrameScope: bridge.FrameScope{FrameID: "frame-1", FrameURL: "http://127.0.0.1:1/inner.html", OwnerRef: "e3"},
		FrameTitle: "Inner",
	}))
	frame, ok := scoped["frame"].(map[string]any)
	if !ok {
		t.Fatalf("a scoped text read published no frame object: %v", scoped)
	}
	for key, want := range map[string]string{
		"frameId":    "frame-1",
		"frameUrl":   "http://127.0.0.1:1/inner.html",
		"frameTitle": "Inner",
		"ownerRef":   "e3",
	} {
		if got, _ := frame[key].(string); got != want {
			t.Errorf("frame[%q] = %q, want %q", key, got, want)
		}
	}
	// The tab document keeps its own fields: that is what stops `url` meaning two things
	// depending on state a reader cannot see.
	if scoped["url"] != unscoped["url"] || scoped["title"] != unscoped["title"] {
		t.Errorf("a scoped read changed the tab's url/title: %v vs %v", scoped["url"], unscoped["url"])
	}
}

// outerFixtureHTML and innerFixtureHTML mirror the reported fixture: two documents that are
// distinguishable only by their content, so a read attributed to the wrong one is visible.
const outerFixtureHTML = `<!doctype html><html><head><title>Outer</title></head><body>
<h1>OuterHeading</h1><input id="oh" value="outerval"><button>OuterBtn</button>
<iframe id="f" src="/inner.html" width="300" height="200"></iframe>
<iframe id="g" src="/second.html" width="300" height="200"></iframe>
</body></html>`

// A SECOND child, so a read served from one frame can be told apart from a scope pointing at
// the other. With one frame, a disclosure that names the stored scope and one that names the
// frame the read used are the same string.
const secondFixtureHTML = `<!doctype html><html><head><title>Second</title></head><body>
<h1>SecondHeading</h1><p>Second frame paragraph text belonging to neither of the others.</p>
</body></html>`

const innerFixtureHTML = `<!doctype html><html><head><title>Inner</title></head><body>
<h1>InnerHeading</h1><input value="innerval"><button>InnerBtn</button>
<p>Inner frame paragraph text that is unique to the child document.</p>
</body></html>`

func newFrameDisclosureFixture(t *testing.T) (*Handlers, string, string) {
	t.Helper()
	chromePath := testbrowser.Path(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/second.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(secondFixtureHTML))
	})
	mux.HandleFunc("/inner.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(innerFixtureHTML))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(outerFixtureHTML))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

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

	if err := chromedp.Run(ctx, chromedp.Navigate(server.URL), chromedp.WaitVisible("#f", chromedp.ByID), chromedp.WaitVisible("#g", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{ActionTimeout: 10 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := bridge.New(context.Background(), ctx, cfg)
	b.RegisterTab("tab-frame", ctx)
	return New(b, cfg, nil, nil, nil), "tab-frame", server.URL
}

func snapshotBody(t *testing.T, h *Handlers, tabID, format string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/snapshot?tabId="+tabID+"&format="+format, nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot (%s): status %d body=%s", format, w.Code, w.Body.String())
	}
	return w.Body.String()
}

// The reported defect, end to end: the same tab at the same url read twice, once scoped and
// once not. Before this change the two answers differed only in the node count, which reads
// as a page change rather than a fragment.
func TestAScopedReadIsDistinguishableFromAWholeDocumentRead(t *testing.T) {
	h, tabID, pageURL := newFrameDisclosureFixture(t)

	unscopedHeader := snapshotBody(t, h, tabID, "compact")
	if !strings.Contains(unscopedHeader, "OuterHeading") || !strings.Contains(unscopedHeader, "InnerHeading") {
		t.Fatalf("the unscoped read did not cover both documents: %s", unscopedHeader)
	}
	if header := strings.SplitN(unscopedHeader, "\n", 2)[0]; strings.Contains(header, "frame ") {
		t.Errorf("an unscoped read claims a frame scope: %s", header)
	}

	frameReq := httptest.NewRequest("POST", "/frame", strings.NewReader(`{"tabId":"`+tabID+`","target":"inner.html"}`))
	frameRes := httptest.NewRecorder()
	h.HandleFrame(frameRes, frameReq)
	if frameRes.Code != http.StatusOK {
		t.Fatalf("frame scope: status %d body=%s", frameRes.Code, frameRes.Body.String())
	}

	scopedHeader := snapshotBody(t, h, tabID, "compact")
	if strings.Contains(scopedHeader, "OuterHeading") {
		t.Fatalf("the scope did not take effect, so this test proves nothing: %s", scopedHeader)
	}
	if !strings.Contains(strings.SplitN(scopedHeader, "\n", 2)[0], "frame ") {
		t.Errorf("the scoped header does not disclose the scope: %s", scopedHeader)
	}
	if !strings.Contains(scopedHeader, pageURL) {
		t.Errorf("the scoped header stopped reporting the tab's url: %s", scopedHeader)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(snapshotBody(t, h, tabID, "")), &envelope); err != nil {
		t.Fatalf("decode scoped snapshot: %v", err)
	}
	if envelope["title"] != "Outer" || envelope["url"] != pageURL+"/" && envelope["url"] != pageURL {
		t.Errorf("a scoped read re-pointed the tab's own url/title: url=%v title=%v", envelope["url"], envelope["title"])
	}
	frame, ok := envelope["frame"].(map[string]any)
	if !ok {
		t.Fatalf("the scoped snapshot published no frame object: %v", envelope)
	}
	if frame["frameTitle"] != "Inner" {
		t.Errorf("frame.frameTitle = %v, want the child document's title", frame["frameTitle"])
	}
	if url, _ := frame["frameUrl"].(string); !strings.HasSuffix(url, "/inner.html") {
		t.Errorf("frame.frameUrl = %v, want the child document's url", frame["frameUrl"])
	}
	if id, _ := frame["frameId"].(string); id == "" {
		t.Error("frame.frameId is empty, so the disclosure names no frame")
	}

	// /text is the other endpoint the card measured, and it reaches the disclosure through a
	// different path — its frame comes from resolveTargetFrameID, not from the snapshot's
	// scope lookup — so the wiring is asserted here rather than inferred from the snapshot.
	textReq := httptest.NewRequest("GET", "/text?tabId="+tabID, nil)
	textRes := httptest.NewRecorder()
	h.HandleText(textRes, textReq)
	if textRes.Code != http.StatusOK {
		t.Fatalf("text: status %d body=%s", textRes.Code, textRes.Body.String())
	}
	var textEnvelope map[string]any
	if err := json.Unmarshal(textRes.Body.Bytes(), &textEnvelope); err != nil {
		t.Fatalf("decode scoped text: %v", err)
	}
	if text, _ := textEnvelope["text"].(string); !strings.Contains(text, "Inner frame paragraph") {
		t.Fatalf("the scoped text read did not return the child document: %q", text)
	}
	textFrame, ok := textEnvelope["frame"].(map[string]any)
	if !ok {
		t.Fatalf("the scoped text read published no frame object: %v", textEnvelope)
	}
	if textFrame["frameTitle"] != "Inner" {
		t.Errorf("text frame.frameTitle = %v, want the child document's title", textFrame["frameTitle"])
	}
	if textEnvelope["title"] != "Outer" {
		t.Errorf("the scoped text read re-pointed the tab's title: %v", textEnvelope["title"])
	}
}

// A ?frameId= read carries no stored scope, and it is the path the design note claims covers
// itself: frameDisclosureFor takes the frame the read ALREADY resolved rather than looking the
// scope up again, so a one-shot read is disclosed like a scoped one. Nothing pinned that.
// Re-deriving the disclosure from the stored scope — the natural simplification, since passing
// the frame in looks redundant — leaves the rest of the package green and returns this card's
// exact defect here: the child's content under the parent's url and title, disclosing nothing.
//
// The scope is set only to LEARN the frame id and is cleared again, so the read under test
// runs against a tab whose stored scope is empty — asserted before the read, because that is
// the whole premise.
func TestAOneShotFrameReadDisclosesTheFrameOnAnUnscopedTab(t *testing.T) {
	h, tabID, pageURL := newFrameDisclosureFixture(t)

	frameID := scopeToInnerFrame(t, h, tabID)
	resetFrameScope(t, h, tabID)
	if scope, ok := h.currentFrameScope(tabID); ok {
		t.Fatalf("the tab still holds a scope (%+v), so this read would be an ordinary scoped one and would prove nothing", scope)
	}

	whole := textEnvelope(t, h, "/text?tabId="+tabID)
	if _, ok := whole["frame"]; ok {
		t.Fatalf("the unscoped read published a frame key before any frameId was asked for: %v", whole["frame"])
	}
	if text, _ := whole["text"].(string); !strings.Contains(text, "OuterHeading") {
		t.Fatalf("the unscoped read did not return the tab document: %q", text)
	}

	oneShot := textEnvelope(t, h, "/text?tabId="+tabID+"&frameId="+frameID)
	if text, _ := oneShot["text"].(string); !strings.Contains(text, "Inner frame paragraph") {
		t.Fatalf("?frameId= did not read the child document, so the disclosure below is not the thing under test: %q", text)
	}

	frame, ok := oneShot["frame"].(map[string]any)
	if !ok {
		t.Fatalf("a ?frameId= read returned the child's content with no disclosure — the parent's url and title with nothing to say the content is a fragment, which is this card's defect: %v", oneShot)
	}
	if got, _ := frame["frameId"].(string); got != frameID {
		t.Errorf("frame.frameId = %q, want %q — the disclosure must name the frame the read was served from, not whatever the tab was last scoped to", got, frameID)
	}
	if frame["frameTitle"] != "Inner" {
		t.Errorf("frame.frameTitle = %v, want the child document's title", frame["frameTitle"])
	}
	if url, _ := frame["frameUrl"].(string); !strings.HasSuffix(url, "/inner.html") {
		t.Errorf("frame.frameUrl = %v, want the child document's url", frame["frameUrl"])
	}
	// The tab document keeps its own identity here too: a one-shot read must not re-point what
	// url and title mean any more than a scoped one does.
	if oneShot["title"] != "Outer" {
		t.Errorf("a one-shot frame read re-pointed the tab's title: %v", oneShot["title"])
	}
	if url, _ := oneShot["url"].(string); !strings.HasPrefix(url, pageURL) {
		t.Errorf("a one-shot frame read re-pointed the tab's url: %v", url)
	}
}

// scopeToInnerFrame scopes the tab to the child frame and returns the frame id the server
// assigned, which is the only way to learn an id a caller could pass as ?frameId=.
func scopeToInnerFrame(t *testing.T, h *Handlers, tabID string) string {
	t.Helper()
	return scopeToFrame(t, h, tabID, "inner.html")
}

func scopeToFrame(t *testing.T, h *Handlers, tabID, target string) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/frame", strings.NewReader(`{"tabId":"`+tabID+`","target":"`+target+`"}`))
	res := httptest.NewRecorder()
	h.HandleFrame(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("frame scope: status %d body=%s", res.Code, res.Body.String())
	}
	scope, ok := h.currentFrameScope(tabID)
	if !ok || scope.FrameID == "" {
		t.Fatalf("scoping reported success but stored no frame id: %+v", scope)
	}
	return scope.FrameID
}

func resetFrameScope(t *testing.T, h *Handlers, tabID string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/frame", strings.NewReader(`{"tabId":"`+tabID+`","target":"main"}`))
	res := httptest.NewRecorder()
	h.HandleFrame(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("frame main: status %d body=%s", res.Code, res.Body.String())
	}
}

func textEnvelope(t *testing.T, h *Handlers, url string) map[string]any {
	t.Helper()
	res := httptest.NewRecorder()
	h.HandleText(res, httptest.NewRequest("GET", url, nil))
	if res.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d body=%s", url, res.Code, res.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return envelope
}

// The other half of the same claim, and the one a single-frame fixture cannot see: when the
// tab IS scoped and a read names a different frame explicitly, the disclosure must describe
// the frame the read was served from, not the stored scope. With one child frame those two
// answers are the same string, so this needs the second.
func TestAOneShotFrameReadDisclosesTheFrameItReadNotTheStoredScope(t *testing.T) {
	h, tabID, _ := newFrameDisclosureFixture(t)

	secondID := scopeToFrame(t, h, tabID, "second.html")
	innerID := scopeToFrame(t, h, tabID, "inner.html")
	if secondID == innerID {
		t.Fatalf("both fixtures resolved to frame %q, so this test cannot tell the two apart", innerID)
	}

	oneShot := textEnvelope(t, h, "/text?tabId="+tabID+"&frameId="+secondID)
	if text, _ := oneShot["text"].(string); !strings.Contains(text, "Second frame paragraph") {
		t.Fatalf("?frameId= read the stored scope's frame rather than the one it named: %q", text)
	}

	frame, ok := oneShot["frame"].(map[string]any)
	if !ok {
		t.Fatalf("the read published no disclosure: %v", oneShot)
	}
	if got, _ := frame["frameId"].(string); got != secondID {
		t.Errorf("frame.frameId = %q, want %q — the disclosure describes the stored scope rather than the frame the read used, which is the defect one level down", got, secondID)
	}
	if frame["frameTitle"] != "Second" {
		t.Errorf("frame.frameTitle = %v, want the document the content actually came from", frame["frameTitle"])
	}
}

// captureEnvelope drives the real /capture and returns the decoded response. A local Chrome
// that cannot screenshot skips rather than fails, the way the other capture tests do.
func captureEnvelope(t *testing.T, h *Handlers, tabID string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.HandleCapture(rec, httptest.NewRequest(http.MethodGet, "/capture?output=inline&format=png&tabId="+tabID, nil))
	if rec.Code != http.StatusOK {
		if strings.Contains(rec.Body.String(), "Unable to capture screenshot") {
			t.Skipf("local Chrome cannot capture screenshots: %s", rec.Body.String())
		}
		t.Fatalf("capture status %d: %s", rec.Code, rec.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode capture: %v (%s)", err, rec.Body.String())
	}
	return envelope
}

func captureSnapshotNodeCount(t *testing.T, envelope map[string]any) int {
	t.Helper()
	snapshot, ok := envelope["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("capture published no snapshot: %v", envelope)
	}
	count, ok := snapshot["nodeCount"].(float64)
	if !ok {
		t.Fatalf("capture snapshot has no nodeCount: %v", snapshot)
	}
	return int(count)
}

// /capture is the third scoped reader, and the one whose silence was worst: its snapshot
// half is filtered to the scoped frame while url and title name the parent, and the single
// frame-shaped key in the payload — epoch.frameId — is the frame tree's ROOT and does not
// move when the scope does. A reader who checked it while scoped was told the content came
// from the main document, which is worse than the silence this card started with.
//
// The epoch assertion is the one that would have caught that: it holds epoch.frameId
// IDENTICAL across the two reads, so it cannot be mistaken for a disclosure again.
func TestAScopedCaptureDisclosesTheFrameWhileItsEpochIdStaysTheMainFrame(t *testing.T) {
	h, tabID, pageURL := newFrameDisclosureFixture(t)

	unscoped := captureEnvelope(t, h, tabID)
	if _, ok := unscoped["frame"]; ok {
		t.Errorf("an unscoped capture published a frame object: %v", unscoped["frame"])
	}
	unscopedNodes := captureSnapshotNodeCount(t, unscoped)

	frameRes := httptest.NewRecorder()
	h.HandleFrame(frameRes, httptest.NewRequest("POST", "/frame", strings.NewReader(`{"tabId":"`+tabID+`","target":"inner.html"}`)))
	if frameRes.Code != http.StatusOK {
		t.Fatalf("frame scope: status %d body=%s", frameRes.Code, frameRes.Body.String())
	}

	scoped := captureEnvelope(t, h, tabID)
	if scopedNodes := captureSnapshotNodeCount(t, scoped); scopedNodes >= unscopedNodes {
		t.Fatalf("capture returned %d nodes scoped and %d unscoped; the scope did not take effect, so this test proves nothing", scopedNodes, unscopedNodes)
	}

	frame, ok := scoped["frame"].(map[string]any)
	if !ok {
		t.Fatalf("a scoped capture published no frame object, so the fragment it returned is attributed to the parent alone: %v", scoped)
	}
	if frame["frameTitle"] != "Inner" {
		t.Errorf("frame.frameTitle = %v, want the child document's title", frame["frameTitle"])
	}
	if url, _ := frame["frameUrl"].(string); !strings.HasSuffix(url, "/inner.html") {
		t.Errorf("frame.frameUrl = %v, want the child document's url", frame["frameUrl"])
	}
	if scoped["title"] != "Outer" || !strings.HasPrefix(scoped["url"].(string), pageURL) {
		t.Errorf("a scoped capture re-pointed the tab's own url/title: url=%v title=%v", scoped["url"], scoped["title"])
	}

	// epoch.frameId pairs the image with the DOM epoch it was shot against. It is the same
	// value scoped or not, which is exactly why it cannot serve as the scope disclosure —
	// and why the docs sentence that said it could was wrong.
	unscopedEpoch := epochFrameID(t, unscoped)
	scopedEpoch := epochFrameID(t, scoped)
	if unscopedEpoch != scopedEpoch {
		t.Errorf("epoch.frameId moved with the scope (%s -> %s); it is the epoch contract, not the scope", unscopedEpoch, scopedEpoch)
	}
	if scopedEpoch == frame["frameId"] {
		t.Errorf("epoch.frameId equals the scoped frame id (%s); the fixture is not distinguishing the two contracts", scopedEpoch)
	}
}

func epochFrameID(t *testing.T, envelope map[string]any) string {
	t.Helper()
	epoch, ok := envelope["epoch"].(map[string]any)
	if !ok {
		t.Fatalf("capture published no epoch block: %v", envelope)
	}
	id, _ := epoch["frameId"].(string)
	if id == "" {
		t.Fatalf("capture epoch carries no frameId: %v", epoch)
	}
	return id
}
