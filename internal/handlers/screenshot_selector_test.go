package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// One element of a known size, far smaller than any viewport, so the expected
// clipped dimensions are exact rather than a ratio.
const screenshotFixtureHTML = `<body style="margin:0;background:#fff">
<div id="small" style="position:absolute;left:40px;top:60px;width:120px;height:60px;background:#c00">small</div>
<div id="empty" style="position:absolute;left:0;top:0;width:0;height:0"></div>
</body>`

const (
	screenshotSmallWidth  = 120
	screenshotSmallHeight = 60
	screenshotTabID       = "tab-shot"
)

type screenshotFixture struct {
	handlers *Handlers
	ctx      context.Context
}

func newScreenshotFixture(t *testing.T) screenshotFixture {
	t.Helper()

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(screenshotFixtureHTML))
	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	t.Cleanup(cancelTimeout)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#small", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{ActionTimeout: 10 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := browserBridge(ctx, cfg)
	b.RegisterTab(screenshotTabID, ctx)

	return screenshotFixture{handlers: New(b, cfg, nil, nil, nil), ctx: ctx}
}

// shoot drives the real endpoint and returns the decoded PNG plus its status.
func (f screenshotFixture) shoot(t *testing.T, query string) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	f.handlers.HandleScreenshot(rec, httptest.NewRequest(http.MethodGet, "/screenshot?format=png&tabId="+screenshotTabID+query, nil))
	if rec.Code != http.StatusOK {
		return rec.Code, rec.Body.Bytes()
	}
	var resp struct {
		Base64 string `json:"base64"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, rec.Body.String())
	}
	buf, err := base64.StdEncoding.DecodeString(resp.Base64)
	if err != nil {
		t.Fatalf("decode base64 image: %v", err)
	}
	return rec.Code, buf
}

func pngSize(t *testing.T, buf []byte) (int, int) {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return cfg.Width, cfg.Height
}

// The reported defect, at the endpoint: a selector was accepted, resolved, and
// silently ignored — every capture came back the full viewport and byte-identical
// to one that asked for no selector at all.
func TestScreenshotSelectorClipsToElement(t *testing.T) {
	f := newScreenshotFixture(t)

	statusFull, full := f.shoot(t, "")
	if statusFull != http.StatusOK {
		t.Fatalf("no-selector capture: status %d, body %s", statusFull, full)
	}
	fullW, fullH := pngSize(t, full)
	if fullW == screenshotSmallWidth && fullH == screenshotSmallHeight {
		t.Fatalf("viewport is %dx%d, the same as the target element — the fixture cannot tell them apart", fullW, fullH)
	}

	// A CSS selector and a ref for the same element must give the same answer:
	// the ref path resolves through the snapshot cache and the CSS path does not.
	nodeID, err := bridge.ResolveCSSToNodeID(f.ctx, "#small")
	if err != nil {
		t.Fatal(err)
	}
	f.handlers.Bridge.SetRefCache(screenshotTabID, &bridge.RefCache{Targets: map[string]bridge.RefTarget{
		"e0": {BackendNodeID: nodeID},
	}})

	for _, sel := range []string{"%23small", "css:%23small", "e0", "ref:e0"} {
		status, buf := f.shoot(t, "&selector="+sel)
		if status != http.StatusOK {
			t.Errorf("selector %q: status %d, body %s", sel, status, buf)
			continue
		}
		w, h := pngSize(t, buf)
		if w != screenshotSmallWidth || h != screenshotSmallHeight {
			t.Errorf("selector %q: image is %dx%d, want %dx%d", sel, w, h, screenshotSmallWidth, screenshotSmallHeight)
		}
		if bytes.Equal(buf, full) {
			t.Errorf("selector %q: image is byte-identical to the no-selector capture — the selector was ignored", sel)
		}
	}
}

// An element with no box cannot be clipped. Returning the whole viewport would
// be the original defect wearing a different hat, so it has to be an error.
func TestScreenshotSelectorThatCannotBeClippedErrors(t *testing.T) {
	f := newScreenshotFixture(t)

	status, body := f.shoot(t, "&selector=%23empty")
	if status == http.StatusOK {
		w, h := pngSize(t, body)
		t.Fatalf("zero-size element returned a %dx%d image, want an error", w, h)
	}
	if status < 400 || status > 599 {
		t.Errorf("status = %d, want a 4xx/5xx error", status)
	}
}

// Documented precedence: a selector already clips to an element, so it wins and
// beyondViewport is dropped. The combination must still yield the element's box,
// not a document-sized capture.
func TestScreenshotSelectorWinsOverBeyondViewport(t *testing.T) {
	f := newScreenshotFixture(t)

	if req := parseScreenshotRequest(httptest.NewRequest(http.MethodGet, "/screenshot?beyondViewport=true&selector=%23small", nil)); req.beyondViewport {
		t.Error("beyondViewport must be dropped when a selector is set")
	}

	status, buf := f.shoot(t, "&selector=%23small&beyondViewport=true")
	if status != http.StatusOK {
		t.Fatalf("status %d, body %s", status, buf)
	}
	w, h := pngSize(t, buf)
	if w != screenshotSmallWidth || h != screenshotSmallHeight {
		t.Errorf("selector+beyondViewport image is %dx%d, want the element's %dx%d", w, h, screenshotSmallWidth, screenshotSmallHeight)
	}
}
