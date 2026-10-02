package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

const scrollPositionFixtureHTML = `<!DOCTYPE html>
<body style="margin:0">
<div id="list" style="height:200px;width:300px;overflow-y:auto">
	<div style="height:1000px">rows</div>
</div>
<div style="height:3000px">page</div>
</body>`

func newScrollPositionTab(t *testing.T) context.Context {
	t.Helper()
	ctx := newActionTab(t, 30*time.Second)
	url := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(scrollPositionFixtureHTML))
	if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func scrollAt(t *testing.T, ctx context.Context, x, y float64, deltaY int) map[string]any {
	t.Helper()
	result, err := (&Bridge{}).actionScroll(ctx, ActionRequest{HasXY: true, X: x, Y: y, HasScroll: true, ScrollY: deltaY})
	if err != nil {
		t.Fatalf("scroll by %d at (%v, %v): %v", deltaY, x, y, err)
	}
	return result
}

func evalFloat(t *testing.T, ctx context.Context, expr string) float64 {
	t.Helper()
	var value float64
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &value)); err != nil {
		t.Fatalf("evaluate %s: %v", expr, err)
	}
	return value
}

func TestScrollPastTheEndReportsTheClampedPositionInRealBrowser(t *testing.T) {
	ctx := newScrollPositionTab(t)
	const belowTheList = 400

	first := scrollAt(t, ctx, 100, belowTheList, 999999)

	maxScrollY := evalFloat(t, ctx, `document.scrollingElement.scrollHeight - document.scrollingElement.clientHeight`)
	if maxScrollY <= 0 {
		t.Fatalf("fixture page does not overflow the viewport: max scrollY %v", maxScrollY)
	}
	if first["scrolled"] != true || first["scrollY"] != maxScrollY || first["scrollX"] != 0.0 {
		t.Fatalf("scroll past the end = %#v, want scrolled:true at scrollY %v", first, maxScrollY)
	}
	if windowY := evalFloat(t, ctx, `window.scrollY`); first["scrollY"] != windowY {
		t.Fatalf("reported scrollY %v, window.scrollY is %v", first["scrollY"], windowY)
	}
	for _, legacy := range []string{"x", "y"} {
		if _, ok := first[legacy]; ok {
			t.Errorf("result still carries the legacy %q key: %#v", legacy, first)
		}
	}

	second := scrollAt(t, ctx, 100, belowTheList, 800)
	if second["scrolled"] != false || second["scrollY"] != maxScrollY {
		t.Fatalf("scroll at the bottom = %#v, want scrolled:false at scrollY %v", second, maxScrollY)
	}
}

func TestScrollOverAnInnerScrollerReportsItsPositionInRealBrowser(t *testing.T) {
	ctx := newScrollPositionTab(t)

	result := scrollAt(t, ctx, 100, 100, 150)

	if result["scrolled"] != true || result["scrollY"] != 150.0 {
		t.Fatalf("scroll over the list = %#v, want scrolled:true at scrollY 150", result)
	}
	if listY := evalFloat(t, ctx, `document.getElementById('list').scrollTop`); listY != 150 {
		t.Fatalf("list scrollTop = %v, want 150", listY)
	}
	if windowY := evalFloat(t, ctx, `window.scrollY`); windowY != 0 {
		t.Fatalf("window.scrollY = %v, want the window left still", windowY)
	}
}

func TestSmoothScrollingPageStillReportsTheMoveInRealBrowser(t *testing.T) {
	ctx := newScrollPositionTab(t)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.style.scrollBehavior = 'smooth'`, nil)); err != nil {
		t.Fatal(err)
	}

	result := scrollAt(t, ctx, 100, 400, 500)

	if result["scrolled"] != true || result["scrollY"] != 500.0 {
		t.Fatalf("scroll on a smooth-scrolling page = %#v, want scrolled:true at scrollY 500", result)
	}
}
