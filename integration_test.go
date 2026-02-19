// TestCDPTimezoneOverride verifies that Emulation.
// TestStealthStatusEndpoint verifies the /stealth/status handler returns valid data with a live browser.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func testBridge(t *testing.T) (*Bridge, func()) {
	t.Helper()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Headless,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("disable-gpu", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	seed := rand.Intn(1000000000)
	seededScript := fmt.Sprintf("var __pinchtab_seed = %d;\nvar __pinchtab_stealth_level = %q;\n", seed, "full") + stealthScript
	if err := chromedp.Run(browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(seededScript).Do(ctx)
			return err
		}),
	); err != nil {
		allocCancel()
		t.Fatalf("failed to start Chrome: %v", err)
	}

	b := &Bridge{
		allocCtx:   allocCtx,
		browserCtx: browserCtx,
	}
	b.InitTabManager()

	initID := string(chromedp.FromContext(browserCtx).Target.TargetID)
	b.RegisterTab(initID, browserCtx)

	return b, func() {
		browserCancel()
		allocCancel()
	}
}

func navigateAndWait(t *testing.T, b *Bridge, dataURL string) {
	t.Helper()
	ctx, _, err := b.TabContext("")
	if err != nil {
		t.Fatalf("TabContext: %v", err)
	}
	tCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := chromedp.Run(tCtx, chromedp.Navigate(dataURL)); err != nil {
		t.Fatalf("navigate: %v", err)
	}
}

func TestStealthScriptInjected(t *testing.T) {
	b, cleanup := testBridge(t)
	defer cleanup()

	navigateAndWait(t, b, "data:text/html,<h1>stealth test</h1>")

	ctx, _, _ := b.TabContext("")
	tCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var result string
	if err := chromedp.Run(tCtx, chromedp.Evaluate(`String(navigator.webdriver)`, &result)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result == "true" {
		t.Error("navigator.webdriver should not be true (stealth script not working)")
	}

	if result != "undefined" && result != "false" {
		t.Errorf("navigator.webdriver = %q, want 'undefined' or 'false'", result)
	}
}

func TestCanvasNoiseApplied(t *testing.T) {
	b, cleanup := testBridge(t)
	defer cleanup()

	html := `data:text/html,<canvas id="c" width="200" height="50"></canvas>
<script>
var c = document.getElementById('c');
var ctx = c.getContext('2d');
ctx.fillStyle = 'red';
ctx.fillRect(0,0,200,50);
ctx.fillStyle = 'blue';
ctx.font = '18px Arial';
ctx.fillText('canvas fingerprint', 10, 30);
</script>`
	navigateAndWait(t, b, html)

	ctx, _, _ := b.TabContext("")
	tCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var result1, result2 string
	script := `
(function() {
  var c = document.getElementById('c');
  return c.toDataURL();
})()
`
	if err := chromedp.Run(tCtx,
		chromedp.Evaluate(script, &result1),
		chromedp.Evaluate(script, &result2),
	); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if result1 == result2 {
		t.Error("toDataURL returned identical results — canvas noise not applied")
	}
}

func TestFontMetricsNoise(t *testing.T) {
	b, cleanup := testBridge(t)
	defer cleanup()

	navigateAndWait(t, b, "data:text/html,<canvas id='c'></canvas>")

	ctx, _, _ := b.TabContext("")
	tCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	script := `
(function() {
  var c = document.getElementById('c').getContext('2d');
  c.font = '16px Arial';
  var widths = [];
  for (var i = 0; i < 10; i++) {
    widths.push(c.measureText('Hello World').width);
  }
  // Check if Proxy-wrapped (instanceof TextMetrics)
  var tm = c.measureText('test');
  return {
    widths: widths,
    isTextMetrics: tm instanceof TextMetrics
  };
})()
`
	var result struct {
		Widths        []float64 `json:"widths"`
		IsTextMetrics bool      `json:"isTextMetrics"`
	}
	if err := chromedp.Run(tCtx, chromedp.Evaluate(script, &result)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if !result.IsTextMetrics {
		t.Error("measureText result should be instanceof TextMetrics (Proxy not working)")
	}

	if len(result.Widths) == 0 {
		t.Fatal("no width measurements returned")
	}
	if result.Widths[0] <= 0 {
		t.Errorf("measureText width = %f, expected positive number", result.Widths[0])
	}
}

func TestWebGLVendorSpoofed(t *testing.T) {
	b, cleanup := testBridge(t)
	defer cleanup()

	navigateAndWait(t, b, "data:text/html,<canvas id='gl'></canvas>")

	ctx, _, _ := b.TabContext("")
	tCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	script := `
(function() {
  var canvas = document.getElementById('gl');
  var gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
  if (!gl) return {available: false, vendor: '', renderer: ''};
  var ext = gl.getExtension('WEBGL_debug_renderer_info');
  if (!ext) return {available: true, vendor: 'no-ext', renderer: 'no-ext'};
  return {
    available: true,
    vendor: gl.getParameter(ext.UNMASKED_VENDOR_WEBGL),
    renderer: gl.getParameter(ext.UNMASKED_RENDERER_WEBGL)
  };
})()
`
	var result struct {
		Available bool   `json:"available"`
		Vendor    string `json:"vendor"`
		Renderer  string `json:"renderer"`
	}
	if err := chromedp.Run(tCtx, chromedp.Evaluate(script, &result)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if !result.Available {
		t.Skip("WebGL not available in cfg.Headless mode — skipping vendor check")
	}

	if result.Vendor != "Intel Inc." {
		t.Errorf("WebGL vendor = %q, want %q", result.Vendor, "Intel Inc.")
	}
}

func TestPluginsPresent(t *testing.T) {
	b, cleanup := testBridge(t)
	defer cleanup()

	navigateAndWait(t, b, "data:text/html,<h1>plugins</h1>")

	ctx, _, _ := b.TabContext("")
	tCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var count int
	if err := chromedp.Run(tCtx, chromedp.Evaluate(`navigator.plugins.length`, &count)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if count < 3 {
		t.Errorf("navigator.plugins.length = %d, want >= 3", count)
	}
}

func TestFingerprintRotation(t *testing.T) {
	b, cleanup := testBridge(t)
	defer cleanup()

	navigateAndWait(t, b, "data:text/html,<h1>rotate</h1>")

	body := `{"os":"windows","browser":"edge","screen":"1920x1080"}`
	req := httptest.NewRequest("POST", "/fingerprint/rotate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	b.handleFingerprintRotate(w, req)

	resp := w.Result()
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("fingerprint/rotate returned %d: %s", resp.StatusCode, respBody)
	}

	var rotateResp map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&rotateResp)

	fp, ok := rotateResp["fingerprint"].(map[string]any)
	if !ok {
		t.Fatal("response missing fingerprint object")
	}
	newUA, _ := fp["userAgent"].(string)
	if !strings.Contains(newUA, "Edg/") {
		t.Errorf("expected Edge UA in fingerprint, got: %s", newUA)
	}

	ctx2, _, _ := b.TabContext("")
	tCtx2, cancel2 := context.WithTimeout(ctx2, 5*time.Second)
	defer cancel2()

	var uaAfter string
	if err := chromedp.Run(tCtx2, chromedp.Evaluate(`navigator.userAgent`, &uaAfter)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !strings.Contains(uaAfter, "Edg/") {
		t.Errorf("browser UA after rotation = %q, expected Edge UA", uaAfter)
	}
}

func TestCDPTimezoneOverride(t *testing.T) {
	b, cleanup := testBridge(t)
	defer cleanup()

	ctx, _, _ := b.TabContext("")
	tCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetTimezoneOverride("Pacific/Auckland").Do(ctx)
		}),
	); err != nil {
		t.Fatalf("setTimezoneOverride: %v", err)
	}

	navigateAndWait(t, b, "data:text/html,<h1>tz</h1>")

	ctx2, _, _ := b.TabContext("")
	tCtx2, cancel2 := context.WithTimeout(ctx2, 5*time.Second)
	defer cancel2()

	var tz string
	if err := chromedp.Run(tCtx2, chromedp.Evaluate(`Intl.DateTimeFormat().resolvedOptions().timeZone`, &tz)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if tz != "Pacific/Auckland" {
		t.Errorf("timezone = %q, want Pacific/Auckland", tz)
	}
}

func TestStealthStatusEndpoint(t *testing.T) {
	b, cleanup := testBridge(t)
	defer cleanup()

	navigateAndWait(t, b, "data:text/html,<h1>status</h1>")

	req := httptest.NewRequest("GET", "/stealth/status", nil)
	w := httptest.NewRecorder()
	b.handleStealthStatus(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stealth/status returned %d", resp.StatusCode)
	}

	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)

	score, _ := result["score"].(float64)
	if score < 50 {
		t.Errorf("stealth score = %.0f, expected >= 50", score)
	}

	level, _ := result["level"].(string)
	if level != "high" && level != "medium" {
		t.Errorf("stealth level = %q, expected high or medium", level)
	}
}
