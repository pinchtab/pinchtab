package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const timeInputPage = `<input id="delivery" type="time"><div id="label" tabindex="-1">Delivery</div>`

func openTimeInputPage(t *testing.T) context.Context {
	t.Helper()
	ctx := testbrowser.Launch(t)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(cancel)
	page := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(timeInputPage))
	if err := chromedp.Run(ctx, chromedp.Navigate(page)); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	return ctx
}

func axNodeByRole(t *testing.T, ctx context.Context, role string) cdp.BackendNodeID {
	t.Helper()
	var nodes []*accessibility.Node
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		nodes, err = accessibility.GetFullAXTree().Do(ctx)
		return err
	})); err != nil {
		t.Fatalf("read accessibility tree: %v", err)
	}
	for _, n := range nodes {
		if n.Role != nil && strings.Trim(string(n.Role.Value), `"`) == role && n.BackendDOMNodeID != 0 {
			return n.BackendDOMNodeID
		}
	}
	t.Fatalf("no %s node in the accessibility tree", role)
	return 0
}

func pageValue(t *testing.T, ctx context.Context, selector string) string {
	t.Helper()
	var value string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector("`+selector+`").value`, &value)); err != nil {
		t.Fatalf("read %s value: %v", selector, err)
	}
	return value
}

func TestFillOnATimeInputSegmentRefusesInsteadOfReportingSuccess(t *testing.T) {
	ctx := openTimeInputPage(t)
	segment := axNodeByRole(t, ctx, "spinbutton")

	result, err := (&Bridge{}).actionFill(ctx, ActionRequest{NodeID: int64(segment), Text: "12", HasText: true})
	if err == nil {
		t.Fatalf("fill on a time segment returned %v with no error", result)
	}
	if !errors.Is(err, ErrInvalidActionRequest) {
		t.Errorf("error = %v, want it classified as an invalid request", err)
	}
	for _, want := range []string{"spinbutton", `type="time"`, "ISO"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
	if got := pageValue(t, ctx, "#delivery"); got != "" {
		t.Errorf("time input value = %q, want it untouched", got)
	}
}

func backendNodeID(t *testing.T, ctx context.Context, selector string) int64 {
	t.Helper()
	var nodes []*cdp.Node
	if err := chromedp.Run(ctx, chromedp.Nodes(selector, &nodes, chromedp.ByQuery)); err != nil {
		t.Fatalf("find %s: %v", selector, err)
	}
	return int64(nodes[0].BackendNodeID)
}

func TestReadInputValueIgnoresAValueExpando(t *testing.T) {
	ctx := openTimeInputPage(t)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector("#label").value = "12"`, nil)); err != nil {
		t.Fatalf("plant expando: %v", err)
	}

	got, err := bridgecdpops.ReadInputValue(ctx, backendNodeID(t, ctx, "#label"))
	if err != nil {
		t.Fatalf("ReadInputValue: %v", err)
	}
	if got != "" {
		t.Errorf("ReadInputValue = %q, want the expando ignored", got)
	}
}

func TestFillOnAPlainElementRefuses(t *testing.T) {
	ctx := openTimeInputPage(t)

	_, err := (&Bridge{}).actionFill(ctx, ActionRequest{NodeID: backendNodeID(t, ctx, "#label"), Text: "12", HasText: true})
	if !errors.Is(err, ErrInvalidActionRequest) || !strings.Contains(err.Error(), "<div>") {
		t.Fatalf("error = %v, want an invalid-request refusal naming the <div>", err)
	}
}
