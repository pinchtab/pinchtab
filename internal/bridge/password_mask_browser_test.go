package bridge

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
)

// The snapshot pipeline the /snapshot handler runs — fetch the a11y tree, flatten
// it, then enrich from the DOM — against a real page, because the defect lives in
// what the browser reports for a password field and no stub can stand in for it.
func TestSnapshotDistinguishesEmptyAndFilledPasswordInRealBrowser(t *testing.T) {
	ctx := newActionTab(t, 30*time.Second)

	html := `<form>
		<label for="untouched">Paste bearer token</label>
		<input id="untouched" type="password" autocomplete="off">
		<label for="filled">Password</label>
		<input id="filled" type="password" autocomplete="off">
		<button type="submit">Continue</button>
	</form>`
	if err := chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html))),
		chromedp.SendKeys("#filled", "supersecret123", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}

	rawNodes, err := FetchAXTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	flat, _ := BuildSnapshot(rawNodes, "", -1)
	if err := EnrichA11yNodesWithDOMMetadata(ctx, flat); err != nil {
		t.Fatal(err)
	}

	var untouched, filled *A11yNode
	for i := range flat {
		if flat[i].Role != "textbox" {
			continue
		}
		switch flat[i].Name {
		case "Paste bearer token":
			untouched = &flat[i]
		case "Password":
			filled = &flat[i]
		}
	}
	if untouched == nil || filled == nil {
		t.Fatalf("expected both password fields in the snapshot, got %+v", flat)
	}

	if untouched.Value != "" {
		t.Errorf("untouched password field reports value %q; an agent reads that as already filled", untouched.Value)
	}
	if filled.Value != bridgeobserve.MaskedValue {
		t.Errorf("filled password field value = %q, want %q", filled.Value, bridgeobserve.MaskedValue)
	}
	for i := range flat {
		if flat[i].Value == "supersecret123" || flat[i].Text == "supersecret123" {
			t.Fatalf("snapshot leaked the password: %+v", flat[i])
		}
	}
}

func TestAnUnenrichedSnapshotStillKeepsAPasswordOut(t *testing.T) {
	ctx := newActionTab(t, 30*time.Second)

	const secret = "supersecret123"
	html := `<form>
		<label for="offAutocomplete">Password</label>
		<input id="offAutocomplete" type="password" autocomplete="off">
		<label for="current">Current password</label>
		<input id="current" type="password" autocomplete="current-password">
		<button type="submit">Continue</button>
	</form>`
	if err := chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html))),
		chromedp.SendKeys("#offAutocomplete", secret, chromedp.ByID),
		chromedp.SendKeys("#current", secret, chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}

	rawNodes, err := FetchAXTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	flat, _ := BuildSnapshot(rawNodes, "", -1)

	textboxes := 0
	for i := range flat {
		if strings.Contains(flat[i].Value, secret) || strings.Contains(flat[i].Text, secret) {
			t.Errorf("a snapshot built without DOM enrichment carries the password: %+v", flat[i])
		}
		if flat[i].Role != "textbox" {
			continue
		}
		textboxes++
		if flat[i].Value != bridgeobserve.MaskedValue {
			t.Errorf("filled password field value = %q, want %q — the accessibility tree reports one bullet per character, so passing it through leaks the secret's length to whoever reads the snapshot",
				flat[i].Value, bridgeobserve.MaskedValue)
		}
	}
	if textboxes < 2 {
		t.Fatalf("expected both password fields in the snapshot, found %d textboxes in %+v", textboxes, flat)
	}
}
