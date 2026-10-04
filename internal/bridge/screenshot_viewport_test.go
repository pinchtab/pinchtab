package bridge

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestClipsToViewport(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  *config.RuntimeConfig
		want bool
	}{
		{"nil", nil, false},
		{"chrome", &config.RuntimeConfig{DefaultBrowser: "chrome"}, true},
		{"cloak", &config.RuntimeConfig{DefaultBrowser: "cloak"}, false},
		{"unknown resolves to chrome", &config.RuntimeConfig{DefaultBrowser: "unknown"}, true},
		{"ghost-chrome", &config.RuntimeConfig{DefaultBrowser: "ghost-chrome"}, true},
		{"normalized", &config.RuntimeConfig{DefaultBrowser: " CHROME "}, true},
		{"default", &config.RuntimeConfig{}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClipsToViewport(tt.cfg); got != tt.want {
				t.Errorf("ClipsToViewport(%+v) = %v, want %v", tt.cfg, got, tt.want)
			}
		})
	}
}

func TestCaptureScreenshotClipsToEmulatedViewport(t *testing.T) {
	ctx := newClipFixture(t)
	// The band starts at the scroll target so a clip at document origin is white.
	const html = `<body style="margin:0;height:1500px;background:#fff">
<div style="position:absolute;left:0;top:500px;width:100%;height:300px;background:#12ab34"></div>
</body>`
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		emulation.SetDeviceMetricsOverride(400, 300, 1, false),
	); err != nil {
		t.Fatal(err)
	}

	opts := ScreenshotOpts{Format: ScreenshotFormatPng, ClipToViewport: true}
	buf, err := CaptureScreenshot(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := imageSize(t, buf); w != 400 || h != 300 {
		t.Fatalf("viewport capture is %dx%d, want 400x300", w, h)
	}

	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.scrollTo(0, 500)`, nil)); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		scale  float64
		width  int
		height int
	}{
		{"native", 1, 400, 300},
		{"half", 0.5, 200, 150},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts.Scale = tt.scale
			buf, err := CaptureScreenshot(ctx, opts)
			if err != nil {
				t.Fatal(err)
			}
			img, _, err := image.Decode(bytes.NewReader(buf))
			if err != nil {
				t.Fatalf("decode screenshot: %v", err)
			}
			if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != tt.width || h != tt.height {
				t.Errorf("scrolled capture is %dx%d, want %dx%d", w, h, tt.width, tt.height)
			}
			want := color.RGBA{R: 0x12, G: 0xab, B: 0x34, A: 0xff}
			if got := color.RGBAModel.Convert(img.At(0, 0)); got != want {
				t.Errorf("top-left pixel = %v, want band colour %v", got, want)
			}
		})
	}
}
