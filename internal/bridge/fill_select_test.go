package bridge

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestFillPicksTheOptionOnASelectAndTypesIntoAnInput(t *testing.T) {
	ctx := newActionTab(t, 30*time.Second)
	html := `<body><select id="size"><option value="s">S</option><option value="m">M</option></select><input id="name" type="text"></body>`
	if err := chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html))),
		chromedp.WaitVisible("#name", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{}

	for _, tc := range []struct{ selector, text string }{
		{"#size", "m"},
		{"#name", "Ada"},
	} {
		if _, err := b.fillText(ctx, ActionRequest{Kind: "fill", Selector: tc.selector, Text: tc.text}); err != nil {
			t.Fatalf("fill %s: %v", tc.selector, err)
		}
		var got string
		if err := chromedp.Run(ctx, chromedp.Value(tc.selector, &got, chromedp.ByQuery)); err != nil {
			t.Fatal(err)
		}
		if got != tc.text {
			t.Fatalf("fill %s: value = %q, want %q", tc.selector, got, tc.text)
		}
	}
}
