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
	return openPage(t, timeInputPage)
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

func TestFillOnAContentEditableWritesItsText(t *testing.T) {
	ctx := openTimeInputPage(t)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector("#label").contentEditable = "true"`, nil)); err != nil {
		t.Fatalf("make editable: %v", err)
	}

	result, err := (&Bridge{}).actionFill(ctx, ActionRequest{NodeID: backendNodeID(t, ctx, "#label"), Text: "Front door", HasText: true})
	if err != nil {
		t.Fatalf("fill on a contenteditable: %v", err)
	}
	if warning, ok := result["warning"]; ok {
		t.Errorf("fill warned %q, want the read-back to see the written text", warning)
	}
	var text string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector("#label").textContent`, &text)); err != nil {
		t.Fatalf("read text: %v", err)
	}
	if text != "Front door" {
		t.Errorf("textContent = %q, want %q", text, "Front door")
	}
}

func TestFillOnATimeInputRefusesANonISOValue(t *testing.T) {
	targets := map[string]func(t *testing.T, ctx context.Context) ActionRequest{
		"by ref": func(t *testing.T, ctx context.Context) ActionRequest {
			return ActionRequest{NodeID: backendNodeID(t, ctx, "#delivery")}
		},
		"by selector": func(*testing.T, context.Context) ActionRequest {
			return ActionRequest{Selector: "#delivery"}
		},
	}
	for name, target := range targets {
		t.Run(name, func(t *testing.T) {
			ctx := openTimeInputPage(t)
			req := target(t, ctx)
			req.Text, req.HasText = "half past", true

			result, err := (&Bridge{}).actionFill(ctx, req)
			if err == nil {
				t.Fatalf("fill returned %v with no error", result)
			}
			if !errors.Is(err, ErrInvalidActionRequest) {
				t.Errorf("error = %v, want it classified as an invalid request", err)
			}
			for _, want := range []string{`type="time"`, "ISO", "HH:MM"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to mention %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "React") {
				t.Errorf("error = %q, want no React hint", err)
			}
		})

		t.Run(name+" ISO value", func(t *testing.T) {
			ctx := openTimeInputPage(t)
			req := target(t, ctx)
			req.Text, req.HasText = "12:30", true

			result, err := (&Bridge{}).actionFill(ctx, req)
			if err != nil {
				t.Fatalf("fill with an ISO value: %v", err)
			}
			if result["filled"] != true {
				t.Errorf("result = %v, want filled:true", result)
			}
			if warning, ok := result["warning"]; ok {
				t.Errorf("fill warned %q, want none for an ISO value", warning)
			}
			if got := pageValue(t, ctx, "#delivery"); got != "12:30" {
				t.Errorf("time input value = %q, want %q", got, "12:30")
			}
		})
	}
}

func TestFillOnATextInputKeepsTheTypeHintWhenThePageDropsTheValue(t *testing.T) {
	ctx := openTimeInputPage(t)
	const makeResettingTextInput = `(() => {
		const input = document.querySelector("#delivery");
		input.type = "text";
		input.addEventListener("input", () => { input.value = ""; });
	})()`
	if err := chromedp.Run(ctx, chromedp.Evaluate(makeResettingTextInput, nil)); err != nil {
		t.Fatalf("make resetting text input: %v", err)
	}

	result, err := (&Bridge{}).actionFill(ctx, ActionRequest{NodeID: backendNodeID(t, ctx, "#delivery"), Text: "half past", HasText: true})
	if err != nil {
		t.Fatalf("fill on a text input: %v", err)
	}
	warning, _ := result["warning"].(string)
	if !strings.Contains(warning, "try 'type' instead") {
		t.Errorf("warning = %q, want the unchanged type hint", warning)
	}
}

func TestFillNamesTheISOFormatForEveryDateTimeInputType(t *testing.T) {
	ctx := openTimeInputPage(t)
	formats := map[string]string{
		"date":           "YYYY-MM-DD",
		"time":           "HH:MM",
		"datetime-local": "YYYY-MM-DDTHH:MM",
		"month":          "YYYY-MM",
		"week":           "YYYY-Www",
	}
	for inputType, format := range formats {
		t.Run(inputType, func(t *testing.T) {
			if err := chromedp.Run(ctx, chromedp.SetAttributeValue("#delivery", "type", inputType, chromedp.ByQuery)); err != nil {
				t.Fatalf("set type=%s: %v", inputType, err)
			}

			_, err := (&Bridge{}).actionFill(ctx, ActionRequest{Selector: "#delivery", Text: "half past", HasText: true})
			if !errors.Is(err, ErrInvalidActionRequest) {
				t.Fatalf("error = %v, want an invalid request", err)
			}
			for _, want := range []string{`type="` + inputType + `"`, "ISO format " + format} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to mention %q", err, want)
				}
			}
		})
	}
}

func TestFillOnADateTimeInputThatThePageClearsKeepsTheTypeHint(t *testing.T) {
	targets := map[string]func(t *testing.T, ctx context.Context) ActionRequest{
		"by ref": func(t *testing.T, ctx context.Context) ActionRequest {
			return ActionRequest{NodeID: backendNodeID(t, ctx, "#delivery")}
		},
		"by selector": func(*testing.T, context.Context) ActionRequest {
			return ActionRequest{Selector: "#delivery"}
		},
	}
	values := map[string]string{"time": "12:30", "date": "2026-10-02"}
	for name, target := range targets {
		for inputType, value := range values {
			t.Run(name+" "+inputType, func(t *testing.T) {
				ctx := openTimeInputPage(t)
				makeClearing := `(() => {
					const input = document.querySelector("#delivery");
					input.type = "` + inputType + `";
					input.addEventListener("input", () => { input.value = ""; });
				})()`
				if err := chromedp.Run(ctx, chromedp.Evaluate(makeClearing, nil)); err != nil {
					t.Fatalf("make clearing %s input: %v", inputType, err)
				}
				req := target(t, ctx)
				req.Text, req.HasText = value, true

				result, err := (&Bridge{}).actionFill(ctx, req)
				if err != nil {
					t.Fatalf("fill %q on a cleared %s input: %v", value, inputType, err)
				}
				warning, _ := result["warning"].(string)
				if !strings.Contains(warning, "try 'type' instead") {
					t.Errorf("warning = %q, want the React/type hint", warning)
				}
			})
		}
	}
}

func TestFillOnADateInputRefusesAnISOShapedImpossibleDate(t *testing.T) {
	ctx := openTimeInputPage(t)
	if err := chromedp.Run(ctx, chromedp.SetAttributeValue("#delivery", "type", "date", chromedp.ByQuery)); err != nil {
		t.Fatalf("set type=date: %v", err)
	}

	_, err := (&Bridge{}).actionFill(ctx, ActionRequest{Selector: "#delivery", Text: "2026-02-30", HasText: true})
	if !errors.Is(err, ErrInvalidActionRequest) {
		t.Fatalf("error = %v, want an invalid request", err)
	}
	if !strings.Contains(err.Error(), "ISO format YYYY-MM-DD") {
		t.Errorf("error = %q, want the ISO format hint", err)
	}
}

func openPage(t *testing.T, html string) context.Context {
	t.Helper()
	ctx := testbrowser.Launch(t)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(cancel)
	page := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx, chromedp.Navigate(page)); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	return ctx
}

func TestFillBySelectorReachesAnInputInAnOpenShadowRoot(t *testing.T) {
	ctx := openPage(t, `<div id="host"></div>
	<script>
		document.getElementById("host").attachShadow({mode: "open"}).innerHTML = '<input id="inner">';
	</script>`)

	result, err := (&Bridge{}).actionFill(ctx, ActionRequest{Selector: "#inner", Text: "hello", HasText: true})
	if err != nil {
		t.Fatalf("fill on a shadow-root input by selector: %v", err)
	}
	if warning, ok := result["warning"]; ok {
		t.Errorf("fill warned %q, want the read-back to see the written value", warning)
	}
	var value string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("host").shadowRoot.getElementById("inner").value`, &value)); err != nil {
		t.Fatalf("read shadow input value: %v", err)
	}
	if value != "hello" {
		t.Errorf("shadow input value = %q, want %q", value, "hello")
	}
}

func TestFillOnAMissingSelectorIsErrSelectorNoMatch(t *testing.T) {
	ctx := openTimeInputPage(t)

	_, err := (&Bridge{}).actionFill(ctx, ActionRequest{Selector: "#missing", Text: "x", HasText: true})
	if !errors.Is(err, ErrSelectorNoMatch) {
		t.Fatalf("fill on a missing selector: err = %v, want ErrSelectorNoMatch", err)
	}
}
