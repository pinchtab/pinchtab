package bridge

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestActionSelectRefusesANonSelectElementNamingIt(t *testing.T) {
	ctx := newActionTab(t, 30*time.Second)
	html := `<body><select id="size"><option value="s">S</option><option value="m">M</option></select><button type="submit" id="send">Send</button></body>`
	if err := chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html))),
		chromedp.WaitVisible("#send", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{}

	if _, err := b.actionSelect(ctx, ActionRequest{Kind: "select", Selector: "#size", Value: "m"}); err != nil {
		t.Fatalf("select on a real <select>: %v", err)
	}

	_, err := b.actionSelect(ctx, ActionRequest{Kind: "select", Selector: "#send", Value: "m"})

	if !errors.Is(err, ErrInvalidActionRequest) {
		t.Fatalf("got %v, want ErrInvalidActionRequest", err)
	}
	if msg := err.Error(); !strings.Contains(msg, `<button type="submit">`) || !strings.Contains(msg, "is not a <select>") {
		t.Fatalf("error %q should name the button and say it is not a <select>", msg)
	}
}
