package bridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

func decodeSelectRequest(t *testing.T, body string) ActionRequest {
	t.Helper()

	var req ActionRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return req
}

// `<option value="">` is the standard placeholder, and selecting it is how a dropdown is
// reset. An empty value and an absent one both left Value and Text empty, so select refused
// both — and told a caller that had supplied the argument it was missing.
//
// The pair is the assertion: either half alone is satisfiable by deleting the check.
func TestSelectSeparatesASuppliedEmptyValueFromAnAbsentOne(t *testing.T) {
	for _, tc := range []struct {
		body     string
		want     string
		supplied bool
	}{
		{body: `{"kind":"select","selector":"#pick"}`},
		{body: `{"kind":"select","selector":"#pick","value":""}`, supplied: true},
		{body: `{"kind":"select","selector":"#pick","text":""}`, supplied: true},
		{body: `{"kind":"select","selector":"#pick","value":"uk"}`, want: "uk", supplied: true},
		{body: `{"kind":"select","selector":"#pick","text":"United Kingdom"}`, want: "United Kingdom", supplied: true},
	} {
		got, supplied := SelectValue(decodeSelectRequest(t, tc.body))
		if supplied != tc.supplied || got != tc.want {
			t.Errorf("%s: SelectValue = (%q, %v), want (%q, %v)", tc.body, got, supplied, tc.want, tc.supplied)
		}
	}
}

// The browserless half of the pair, so the rule is still guarded where no browser runs.
// Resolving the element needs a browser this test does not have, so the assertion on the
// supplied-empty case is that it got PAST the refusal and failed later.
func TestActionSelectRefusesOnlyWhenNoValueWasSupplied(t *testing.T) {
	b := &Bridge{}

	_, err := b.actionSelect(context.Background(), decodeSelectRequest(t, `{"kind":"select","selector":"#pick"}`))
	if err == nil {
		t.Fatal("a select carrying no value at all was accepted")
	}
	if !strings.Contains(err.Error(), `send "value": ""`) {
		t.Errorf("refusal = %v, want it to name the empty-valued option idiom", err)
	}

	_, err = b.actionSelect(context.Background(), decodeSelectRequest(t, `{"kind":"select","selector":"#pick","value":""}`))
	if err != nil && strings.Contains(err.Error(), `send "value": ""`) {
		t.Errorf("a supplied empty value was refused as absent: %v", err)
	}
}

// The outcome asserted against the PAGE, not against the action result: the result echoes
// back what the caller asked for, so it reports success for a selection that never landed.
func TestSelectAppliesASuppliedEmptyValueToThePage(t *testing.T) {
	ctx := newActionTab(t, 20*time.Second)

	html := `<select id="pick">
		<option value="uk">United Kingdom</option>
		<option value="">-- choose --</option>
		<option value="fr">France</option>
	</select>`
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx, chromedp.Navigate(dataURL)); err != nil {
		t.Fatal(err)
	}
	selected := func() string {
		t.Helper()
		var value string
		if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#pick').value`, &value)); err != nil {
			t.Fatalf("read the selected value: %v", err)
		}
		return value
	}
	b := New(context.Background(), nil, &config.RuntimeConfig{})

	if _, err := b.Actions[ActionSelect](ctx, decodeSelectRequest(t, `{"kind":"select","selector":"#pick","value":""}`)); err != nil {
		t.Fatalf("a supplied empty value was refused: %v", err)
	}
	if got := selected(); got != "" {
		t.Errorf("the page has %q selected, want the empty-valued placeholder", got)
	}

	// The value-then-text fallback is what makes most placeholders reachable by label, so
	// selection by visible text must keep working alongside the empty-value case.
	if _, err := b.Actions[ActionSelect](ctx, decodeSelectRequest(t, `{"kind":"select","selector":"#pick","value":"France"}`)); err != nil {
		t.Fatalf("select by visible text: %v", err)
	}
	if got := selected(); got != "fr" {
		t.Errorf("selection by visible text left %q selected, want fr", got)
	}

	if _, err := b.Actions[ActionSelect](ctx, decodeSelectRequest(t, `{"kind":"select","selector":"#pick"}`)); err == nil {
		t.Error("a select carrying no value at all was accepted")
	}
	if got := selected(); got != "fr" {
		t.Errorf("the refused select changed the page to %q; a refusal must not act", got)
	}
}

func TestActionCheckRefusesANonCheckableElementNamingIt(t *testing.T) {
	ctx := newActionTab(t, 30*time.Second)
	html := `<body><button type="submit" id="send">Send</button></body>`
	if err := chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html))),
		chromedp.WaitVisible("#send", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{}

	_, err := b.actionCheck(ctx, ActionRequest{Kind: "check", Selector: "#send"})

	if !errors.Is(err, ErrInvalidActionRequest) {
		t.Fatalf("got %v, want ErrInvalidActionRequest", err)
	}
	want := `invalid action request: target <button type="submit"> is not a checkbox, radio, or role=checkbox control`
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}
}
