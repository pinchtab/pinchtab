package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/scroll"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/spf13/cobra"
)

func TestHandleClick(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"ref": "e5",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "click") {
		t.Errorf("expected click in response, got %s", text)
	}
}

func TestHandleClickWaitNav(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"ref":     "e5",
		"waitNav": true,
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, `"waitNav":true`) {
		t.Errorf("expected waitNav in action payload, got %s", text)
	}
}

func TestHandleClickMode(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"ref":  "e5",
		"mode": "dispatch",
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["mode"].(string); got != "dispatch" {
		t.Fatalf("mode = %q, want dispatch", got)
	}
}

func TestHandleClickModeRejectsInvalidValue(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"ref":  "e5",
		"mode": "raw",
	}, srv)
	if !r.IsError {
		t.Fatal("expected error for invalid mode")
	}
}

func TestHandleClickRejectsModeAndHumanizeTogether(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"ref":      "e5",
		"mode":     "dom",
		"humanize": true,
	}, srv)
	if !r.IsError {
		t.Fatal("expected error when mode and humanize are both set")
	}
}

func TestHandleClickMissingRef(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{}, srv)
	if !r.IsError {
		t.Error("expected error for missing ref")
	}
}

func TestHandleClickCoordinates(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"x": float64(120),
		"y": float64(340),
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, ok := body["hasXY"].(bool); !ok || !got {
		t.Fatalf("expected hasXY=true, got %#v", body["hasXY"])
	}
	if got, _ := body["x"].(float64); got != 120 {
		t.Fatalf("x = %v, want 120", got)
	}
	if got, _ := body["y"].(float64); got != 340 {
		t.Fatalf("y = %v, want 340", got)
	}
}

func TestHandleClickQueryAliasUsesSemanticSelector(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"query": "login button",
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["selector"].(string); got != "find:login button" {
		t.Fatalf("selector = %q, want %q", got, "find:login button")
	}
}

func TestHandleClickQueryAliasNumericTextUsesSemanticSelector(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"query": "50.50",
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["selector"].(string); got != "find:50.50" {
		t.Fatalf("selector = %q, want %q", got, "find:50.50")
	}
}

func TestHandleClickQueryAliasPreservesStructuredLocator(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"query": "label:Email",
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["selector"].(string); got != "label:Email" {
		t.Fatalf("selector = %q, want label:Email", got)
	}
}

func TestHandleClickDialogActionPassThrough(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"ref":          "e5",
		"dialogAction": "accept",
		"dialogText":   "pinchtab",
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["dialogAction"].(string); got != "accept" {
		t.Fatalf("dialogAction = %q, want accept", got)
	}
	if got, _ := body["dialogText"].(string); got != "pinchtab" {
		t.Fatalf("dialogText = %q, want pinchtab", got)
	}
}

func TestHandleClickDialogActionRejectsInvalidValue(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"ref":          "e5",
		"dialogAction": "maybe",
	}, srv)

	if !r.IsError {
		t.Fatal("expected error for invalid dialogAction")
	}
}

func TestHandleType(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_type", map[string]any{
		"ref":  "e12",
		"text": "hello world",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "type") {
		t.Errorf("expected type in response, got %s", text)
	}
	if !strings.Contains(text, "hello world") {
		t.Errorf("expected text in response, got %s", text)
	}
}

func TestHandlePress(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_key", map[string]any{
		"action": "press",
		"key":    "Enter",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "Enter") {
		t.Errorf("expected Enter in response, got %s", text)
	}
}

// The five keyboard actions are one tool over one endpoint, so each has to reach
// /action under the bridge kind that implements it, carrying its own argument —
// a wrong kind is dispatched as a different keystroke, silently.
func TestEveryKeyboardActionReachesItsBridgeKind(t *testing.T) {
	for _, tc := range []struct{ action, arg, value, kind string }{
		{"press", "key", "Enter", "press"},
		{"down", "key", "Control", "keydown"},
		{"up", "key", "Control", "keyup"},
		{"type", "text", "hello", "keyboard-type"},
		{"insert", "text", "hello", "keyboard-inserttext"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			srv, _ := upstreamRecorder(t)

			r := callTool(t, "pinchtab_key", map[string]any{"action": tc.action, tc.arg: tc.value}, srv)
			if r.IsError {
				t.Fatalf("action=%q was rejected: %s", tc.action, resultText(t, r))
			}
			body, _ := resultJSON(t, r)["body"].(map[string]any)
			if got, _ := body["kind"].(string); got != tc.kind {
				t.Errorf("action=%q sent kind=%q, want %q", tc.action, got, tc.kind)
			}
			if got, _ := body[tc.arg].(string); got != tc.value {
				t.Errorf("action=%q sent %s=%q, want %q", tc.action, tc.arg, got, tc.value)
			}
		})
	}
}

func TestKeyboardRefusesAnUnknownAction(t *testing.T) {
	srv, paths := upstreamRecorder(t)

	r := callTool(t, "pinchtab_key", map[string]any{"action": "tap", "key": "Enter"}, srv)
	if !r.IsError {
		t.Fatalf("action=\"tap\" was accepted; upstream saw %v", *paths)
	}
	if text := resultText(t, r); !strings.Contains(text, "press") || !strings.Contains(text, "insert") {
		t.Errorf("refusal %q does not list the accepted actions, so the caller cannot correct it", text)
	}
}

func TestHandleSelect(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_select", map[string]any{
		"ref":   "e3",
		"value": "option2",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "select") {
		t.Errorf("expected select, got %s", text)
	}
}

func TestHandleScroll(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scroll", map[string]any{
		"pixels": float64(500),
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "scroll") {
		t.Errorf("expected scroll, got %s", text)
	}
}

// A direction keyword routes to the `scroll` action, not to a wheel event. It used to
// post kind=mouse-wheel with a notch magnitude, which is the category error that made a
// sixth of a viewport look like a reasonable answer to "down": a wheel notch is a device
// unit, and a keyword is an intent. steps still multiplies.
func TestHandleScrollDirectionRoutesToTheScrollAction(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scroll", map[string]any{
		"direction": "down",
		"steps":     float64(2),
	}, srv)

	body, _ := resultJSON(t, r)["body"].(map[string]any)
	if got, _ := body["kind"].(string); got != "scroll" {
		t.Fatalf("kind = %q, want scroll — the same action the CLI's keyword posts", got)
	}
	if got, _ := body["scrollY"].(float64); got != float64(2*scroll.StepPixels) {
		t.Fatalf("scrollY = %v, want %d", got, 2*scroll.StepPixels)
	}
	if _, wheel := body["deltaY"]; wheel {
		t.Errorf("payload still carries a wheel delta: %v", body)
	}
}

// An explicit pixels value still overrides the keyword magnitude, keeping its sign, so a
// caller who wants a notch can still ask for one.
func TestHandleScrollDirectionHonoursAnExplicitPixelsOverride(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scroll", map[string]any{
		"direction": "up",
		"pixels":    float64(120),
		"steps":     float64(3),
	}, srv)

	body, _ := resultJSON(t, r)["body"].(map[string]any)
	if got, _ := body["scrollY"].(float64); got != -360 {
		t.Fatalf("scrollY = %v, want -360 (120 x 3, upward)", got)
	}
}

// A supplied pixels:0 is a caller asking for NO movement — the agent computing a remaining
// distance that reaches zero exactly when it wants to stop. It used to fall through to the
// keyword step and scroll the full 800px in the direction named, answering success, so the
// agent re-measured and asked again and walked the page a step per turn.
//
// The fix is presence, not truthiness: the zero travels on the direction's axis and reaches
// the shared zero-delta resolver that refuses it for every other spelling. That refusal is
// the bridge's and is pinned in internal/bridge; what belongs here is that the zero reaches
// the wire at all, since the old code substituted a magnitude before the request was ever
// built. The mock server cannot refuse, which is exactly why the assertion is on the body.
// The direction rows are asserted TOGETHER with the four spellings that already forwarded
// their zero, because the defect was not that the zero was mishandled — it was that one
// route out of five never reached the rule. A table covering only the fixed route would
// pass again the next time a branch learns to substitute a magnitude of its own.
func TestEverySpellingOfAZeroReachesTheResolverRatherThanASubstitutedMagnitude(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
		axis string
	}{
		{name: "direction down", args: map[string]any{"direction": "down", "pixels": float64(0)}, axis: "scrollY"},
		{name: "direction up", args: map[string]any{"direction": "up", "pixels": float64(0)}, axis: "scrollY"},
		{name: "direction left", args: map[string]any{"direction": "left", "pixels": float64(0)}, axis: "scrollX"},
		{name: "pixels alone", args: map[string]any{"pixels": float64(0)}, axis: "scrollY"},
		{name: "pixels at a coordinate", args: map[string]any{"pixels": float64(0), "x": float64(200), "y": float64(200)}, axis: "deltaY"},
		{name: "pixels at an element", args: map[string]any{"pixels": float64(0), "selector": "#t"}, axis: "deltaY"},
		{name: "the wheel spelling", args: map[string]any{"deltaY": float64(0)}, axis: "deltaY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := mockPinchTab()
			defer srv.Close()

			r := callTool(t, "pinchtab_scroll", tc.args, srv)

			body, _ := resultJSON(t, r)["body"].(map[string]any)
			got, supplied := suppliedScrollAxis(decodeActionBody(t, body), tc.axis)
			if !supplied {
				t.Fatalf("the bridge decodes no supplied %s, so it cannot refuse a zero it never receives: %v", tc.axis, body)
			}
			if got != 0 {
				t.Errorf("%s = %v, want 0 — the caller supplied the magnitude and asked for none", tc.axis, got)
			}
		})
	}
}

// The other half, asserted separately so neither can carry the other: absence still means
// the keyword step, and a supplied non-zero still overrides it with the direction's sign.
// Making a supplied zero honoured must not make an ABSENT pixels honoured as zero.
func TestScrollDirectionKeepsItsStepWhenPixelsIsAbsentOrNonZero(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
		want float64
	}{
		{name: "absent pixels keeps the keyword step", args: map[string]any{"direction": "down"}, want: float64(scroll.StepPixels)},
		{name: "absent pixels still multiplies by steps", args: map[string]any{"direction": "down", "steps": float64(2)}, want: float64(2 * scroll.StepPixels)},
		{name: "a supplied non-zero still overrides", args: map[string]any{"direction": "down", "pixels": float64(50)}, want: 50},
		{name: "the override takes the direction's sign", args: map[string]any{"direction": "up", "pixels": float64(50)}, want: -50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := mockPinchTab()
			defer srv.Close()

			r := callTool(t, "pinchtab_scroll", tc.args, srv)

			body, _ := resultJSON(t, r)["body"].(map[string]any)
			if got, _ := body["scrollY"].(float64); got != tc.want {
				t.Errorf("scrollY = %v, want %v", got, tc.want)
			}
		})
	}
}

// The crossed case neither half covers alone: a direction with an element target AND a
// supplied zero. TestScrollDirectionAtAnElementStaysOnTheWheel pins the wheel routing only
// with pixels absent, so a regression that reached the zero by dropping back to a page
// scroll for the element target would keep every row above green while defeating the
// documented reason the element target rides the wheel — actionScroll short-circuits on a
// target into scroll-into-view, discarding direction, distance and sign, and still answers
// scrolled:true. Routing and magnitude are therefore asserted together, per target spelling.
func TestScrollDirectionAtATargetKeepsBothItsRoutingAndASuppliedZero(t *testing.T) {
	for _, tc := range []struct {
		name     string
		target   map[string]any
		wantKind string
		axis     string
	}{
		{name: "selector", target: map[string]any{"selector": "#feed"}, wantKind: "mouse-wheel", axis: "scrollY"},
		{name: "ref", target: map[string]any{"ref": "e5"}, wantKind: "mouse-wheel", axis: "scrollY"},
		{name: "nodeId", target: map[string]any{"nodeId": float64(42)}, wantKind: "mouse-wheel", axis: "scrollY"},
		{name: "coordinate", target: map[string]any{"x": float64(200), "y": float64(200)}, wantKind: "scroll", axis: "scrollY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := mockPinchTab()
			defer srv.Close()

			args := map[string]any{"direction": "down", "pixels": float64(0)}
			maps.Copy(args, tc.target)
			body, _ := resultJSON(t, callTool(t, "pinchtab_scroll", args, srv))["body"].(map[string]any)

			if got, _ := body["kind"].(string); got != tc.wantKind {
				t.Errorf("kind = %q, want %q — honouring the zero must not change which action the target routes to", got, tc.wantKind)
			}
			got, supplied := suppliedScrollAxis(decodeActionBody(t, body), tc.axis)
			if !supplied {
				t.Fatalf("the bridge decodes no supplied %s, so it cannot refuse a zero it never receives: %v", tc.axis, body)
			}
			if got != 0 {
				t.Errorf("%s = %v, want 0 — the caller supplied the magnitude and asked for none", tc.axis, got)
			}
		})
	}
}

func TestHandleScrollSelectorPixelsUsesMouseWheel(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scroll", map[string]any{
		"selector": "#list",
		"pixels":   float64(300),
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["kind"].(string); got != "mouse-wheel" {
		t.Fatalf("kind = %q, want mouse-wheel", got)
	}
	if got, _ := body["deltaY"].(float64); got != 300 {
		t.Fatalf("deltaY = %v, want 300", got)
	}
}

func TestHandleScrollIntoView(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scroll_into_view", map[string]any{
		"ref": "e9",
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["kind"].(string); got != "scrollintoview" {
		t.Fatalf("kind = %q, want scrollintoview", got)
	}
	if got, _ := body["selector"].(string); got != "e9" {
		t.Fatalf("selector = %q, want e9", got)
	}
}

func TestHandleFill(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_fill", map[string]any{
		"ref":   "e7",
		"value": "test@example.com",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "fill") {
		t.Errorf("expected fill, got %s", text)
	}
}

func TestHandleHover(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_hover", map[string]any{"ref": "e3"}, srv)
	text := resultText(t, r)
	if !strings.Contains(text, "hover") {
		t.Errorf("expected hover, got %s", text)
	}
}

func TestHandleFocus(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_focus", map[string]any{"ref": "e1"}, srv)
	text := resultText(t, r)
	if !strings.Contains(text, "focus") {
		t.Errorf("expected focus, got %s", text)
	}
}

// actionToolTargets is the per-tool matrix this card decided: every action tool
// declares nodeId, because the bridge honours req.NodeID for every one of the nine
// kinds. requiredArgs are the tool's own non-target requirements, and
// selectorOptionalWithNodeID marks the tools whose MCP layer used to demand a
// selector even though the bridge does not.
var actionToolTargets = []struct {
	tool                       string
	requiredArgs               map[string]any
	selectorOptionalWithNodeID bool
}{
	{tool: "pinchtab_click", selectorOptionalWithNodeID: true},
	{tool: "pinchtab_hover", selectorOptionalWithNodeID: true},
	{tool: "pinchtab_focus", selectorOptionalWithNodeID: true},
	{tool: "pinchtab_type", requiredArgs: map[string]any{"text": "hi"}, selectorOptionalWithNodeID: true},
	{tool: "pinchtab_fill", requiredArgs: map[string]any{"value": "v"}, selectorOptionalWithNodeID: true},
	{tool: "pinchtab_select", requiredArgs: map[string]any{"value": "v"}, selectorOptionalWithNodeID: true},
	{tool: "pinchtab_scroll_into_view", selectorOptionalWithNodeID: true},
	{tool: "pinchtab_scroll"},
	{tool: "pinchtab_key", requiredArgs: map[string]any{"action": "press", "key": "Enter"}},
}

func actionArgs(tool string, extra map[string]any) map[string]any {
	args := map[string]any{}
	for _, entry := range actionToolTargets {
		if entry.tool != tool {
			continue
		}
		maps.Copy(args, entry.requiredArgs)
	}
	maps.Copy(args, extra)
	return args
}

func targetedActionArgs(tool string, extra map[string]any) map[string]any {
	args := actionArgs(tool, extra)
	if _, declared := schemaPropertiesOnce()[tool]["selector"]; declared {
		args["selector"] = "#a"
	}
	return args
}

// nodeId was read before the switch on kind, so all nine tools forwarded it, but
// only click, hover and focus declared it. On the other six that meant no
// discovery and — because validateTypedArgs keys its type map per tool — no
// validation either, so a malformed value was dropped in silence.
func TestEveryActionToolAcceptsAndValidatesNodeID(t *testing.T) {
	for _, tc := range actionToolTargets {
		t.Run(tc.tool, func(t *testing.T) {
			srv, _ := upstreamRecorder(t)
			result := callTool(t, tc.tool, targetedActionArgs(tc.tool, map[string]any{"nodeId": float64(42)}), srv)
			if result.IsError {
				t.Fatalf("a valid nodeId was rejected: %s", resultText(t, result))
			}
			body, _ := resultJSON(t, result)["body"].(map[string]any)
			if got, ok := body["nodeId"]; !ok || got != float64(42) {
				t.Errorf("outbound nodeId = %v (present: %v), want 42 — the bridge honours it for this kind", got, ok)
			}

			srv2, paths := upstreamRecorder(t)
			malformed := callTool(t, tc.tool, targetedActionArgs(tc.tool, map[string]any{"nodeId": "abc"}), srv2)
			if !malformed.IsError {
				t.Fatalf("nodeId \"abc\" was accepted and silently dropped; upstream saw %v", *paths)
			}
			if text := resultText(t, malformed); !strings.Contains(text, "nodeId") {
				t.Errorf("rejection %q does not name nodeId, so the caller cannot correct it", text)
			}
			if len(*paths) != 0 {
				t.Errorf("upstream was called %v despite the malformed argument", *paths)
			}
		})
	}
}

// The MCP layer required a selector on type, fill, select and scroll_into_view
// even though the bridge resolves those kinds from NodeID alone. Declaring nodeId
// there without relaxing this would advertise an argument that cannot be used.
func TestNodeIDAloneSatisfiesTheTargetRequirement(t *testing.T) {
	for _, tc := range actionToolTargets {
		if !tc.selectorOptionalWithNodeID {
			continue
		}
		t.Run(tc.tool, func(t *testing.T) {
			srv, _ := upstreamRecorder(t)
			result := callTool(t, tc.tool, actionArgs(tc.tool, map[string]any{"nodeId": float64(42)}), srv)
			if result.IsError {
				t.Fatalf("nodeId alone was rejected: %s", resultText(t, result))
			}
			body, _ := resultJSON(t, result)["body"].(map[string]any)
			if _, ok := body["selector"]; ok {
				t.Errorf("outbound body carries a selector the caller never sent: %v", body)
			}
			if got := body["nodeId"]; got != float64(42) {
				t.Errorf("outbound nodeId = %v, want 42", got)
			}

			srv2, paths := upstreamRecorder(t)
			neither := callTool(t, tc.tool, actionArgs(tc.tool, nil), srv2)
			if !neither.IsError {
				t.Fatalf("a call with no target at all was accepted; upstream saw %v", *paths)
			}
			if text := resultText(t, neither); !strings.Contains(text, "selector") {
				t.Errorf("the no-target rejection %q should still name selector", text)
			}
		})
	}
}

// x/y had the same shape as nodeId — read before the switch, so forwarded for all
// nine kinds with hasXY set — but the opposite correct answer: the bridge honours
// coordinates only for the pointer kinds, so the fix is to stop reading it
// elsewhere rather than to declare it everywhere. A tool that does not declare
// them refuses them by name instead of dropping them.
func TestCoordinatesReachTheWireOnlyForTheToolsThatDeclareThem(t *testing.T) {
	for _, tc := range actionToolTargets {
		t.Run(tc.tool, func(t *testing.T) {
			_, declared := schemaArgTypesOnce()[tc.tool]["x"]
			srv, paths := upstreamRecorder(t)
			result := callTool(t, tc.tool, targetedActionArgs(tc.tool, map[string]any{"x": float64(11), "y": float64(22)}), srv)
			if !declared {
				if !result.IsError || !strings.Contains(resultText(t, result), `unknown argument "x"`) {
					t.Fatalf("%s does not declare x/y but did not refuse them by name: %s", tc.tool, resultText(t, result))
				}
				if len(*paths) != 0 {
					t.Errorf("refused coordinates still reached upstream: %v", *paths)
				}
				return
			}
			if result.IsError {
				t.Fatalf("coordinates were rejected: %s", resultText(t, result))
			}
			body, _ := resultJSON(t, result)["body"].(map[string]any)
			if _, forwarded := body["hasXY"]; !forwarded {
				t.Errorf("%s declares x/y but does not forward them (body %v)", tc.tool, body)
			}
		})
	}
}

// pinchtab_fill posted the caller's string under "value", a real ActionRequest field that
// actionFill does not read, so the write was empty and the tool answered filled:true with
// len:0. The tool surface is the only place this is visible — the bridge action was always
// correct, and no unit test at that layer could see it.
func TestHandleFillForwardsTheFieldTheFillActionReads(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_fill", map[string]any{
		"ref":   "e0",
		"value": "FILLED",
	}, srv)

	body, _ := resultJSON(t, r)["body"].(map[string]any)
	if got, _ := body["text"].(string); got != "FILLED" {
		t.Fatalf("forwarded text = %q, want the caller's value; payload was %v", got, body)
	}
	if _, leftover := body["value"]; leftover {
		t.Errorf("payload still carries a value key that fill ignores: %v", body)
	}
}

// The discriminator: two tools, the same client argument name, and opposite consumers —
// actionSelect reads Value, actionFill reads Text. Both must post to the field their own
// action reads, which is what a shared "value" key silently got wrong for one of them.
func TestFillAndSelectEachPostToTheFieldTheirActionReads(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	fill, _ := resultJSON(t, callTool(t, "pinchtab_fill", map[string]any{"ref": "e0", "value": "ZZZ"}, srv))["body"].(map[string]any)
	sel, _ := resultJSON(t, callTool(t, "pinchtab_select", map[string]any{"ref": "e1", "value": "y"}, srv))["body"].(map[string]any)

	if got, _ := fill["text"].(string); got != "ZZZ" {
		t.Errorf("fill payload = %v, want text=ZZZ", fill)
	}
	if got, _ := sel["value"].(string); got != "y" {
		t.Errorf("select payload = %v, want value=y", sel)
	}
}

// The other spelling still works, since the tool has always accepted both.
func TestHandleFillAcceptsTheTextSpellingToo(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_fill", map[string]any{"ref": "e0", "text": "FROM_TEXT"}, srv)
	body, _ := resultJSON(t, r)["body"].(map[string]any)
	if got, _ := body["text"].(string); got != "FROM_TEXT" {
		t.Fatalf("forwarded text = %q, want FROM_TEXT; payload was %v", got, body)
	}
}

// The pair is the assertion. Supplied-empty must clear and absent must still refuse —
// either half alone is satisfiable by deleting the check, which would silently forward a
// fill with no text at all. Driven through the tool surface, because the bridge action
// was always able to express both and the MCP layer was the only one that could not.
func TestFillClearsOnASuppliedEmptyValueAndStillRefusesAnAbsentOne(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	for _, key := range []string{"value", "text"} {
		t.Run("supplied empty "+key+" clears", func(t *testing.T) {
			r := callTool(t, "pinchtab_fill", map[string]any{"ref": "e0", key: ""}, srv)
			if r.IsError {
				t.Fatalf("a supplied empty %s was refused: %s", key, resultText(t, r))
			}
			body, _ := resultJSON(t, r)["body"].(map[string]any)
			got, supplied := bridge.FillText(decodeActionBody(t, body))
			if !supplied {
				t.Fatalf("the bridge decodes no supplied text, so it cannot tell a clear from a request whose text never arrived: %v", body)
			}
			if got != "" {
				t.Errorf("forwarded text = %q, want the empty string that clears the field", got)
			}
		})
	}

	t.Run("absent value is refused", func(t *testing.T) {
		r := callTool(t, "pinchtab_fill", map[string]any{"ref": "e0"}, srv)
		if !r.IsError {
			body, _ := resultJSON(t, r)["body"].(map[string]any)
			t.Fatalf("a fill with no value at all was forwarded as %v; absent is not a clear", body)
		}
		message := resultText(t, r)
		if strings.Contains(message, "is missing") && strings.Contains(message, "'value'") {
			t.Errorf("refusal %q reads as a supplied parameter being missing; it must say what fill needs and how to clear", message)
		}
		if !strings.Contains(message, "clear") {
			t.Errorf("refusal %q does not tell the caller how to clear a field, which is the case it is most likely to be confused with", message)
		}
	})
}

// Whitespace is content, not a clear: the raw API fills it verbatim, so the tool must not
// trim a supplied value into the clear idiom.
func TestFillForwardsASuppliedValueVerbatim(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_fill", map[string]any{"ref": "e0", "value": "  spaced  "}, srv)
	body, _ := resultJSON(t, r)["body"].(map[string]any)
	if got, _ := body["text"].(string); got != "  spaced  " {
		t.Errorf("forwarded text = %q, want the caller's string unmodified", got)
	}
}

// The library does not enforce a declared argument type before calling a handler, so a model
// answering a numeric-looking field with 2024 rather than "2024" reaches fill with a number.
// Reporting that as a MISSING argument is the same dead end this tool was fixed to remove:
// the caller sent the argument, is told it did not, sends it the same way again.
func TestFillNamesAWrongTypedValueInsteadOfCallingItMissing(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     map[string]any
		wantSays string
	}{
		{name: "a number", args: map[string]any{"selector": "e5", "value": float64(2024)}, wantSays: "must be a string, got number"},
		{name: "a boolean", args: map[string]any{"selector": "e5", "value": true}, wantSays: "must be a string, got boolean"},
		{name: "an object", args: map[string]any{"selector": "e5", "value": map[string]any{"a": 1}}, wantSays: "must be a string, got object"},
		{name: "genuinely absent", args: map[string]any{"selector": "e5"}, wantSays: "needs a 'value' argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("fill posted a request for an unusable value instead of refusing it")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			res := callTool(t, "pinchtab_fill", tc.args, srv)

			if !res.IsError {
				t.Fatalf("accepted %v", tc.args)
			}
			text := resultText(t, res)
			if !strings.Contains(text, tc.wantSays) {
				t.Errorf("error = %q, want it to say %q so the caller knows what to change", text, tc.wantSays)
			}
			assertNamesTheRemedy(t, text, tc.wantSays)
		})
	}
}

// Naming the type is only half the repair: a caller told its number is not a string, with no
// hint that quoting fixes it, still has no next move — which was the dead end being removed.
// The rows that need the remedy are derived from the type clause rather than listed again, so
// a new wrong-type row cannot be added without it.
func assertNamesTheRemedy(t *testing.T, message, wantSays string) {
	t.Helper()

	if !strings.Contains(wantSays, "must be a string") {
		return
	}
	if !strings.Contains(message, "quote it") {
		t.Errorf("error = %q names the type but not the remedy, so the caller learns what is wrong and not what to send", message)
	}
}

// The clear idiom must keep working, since distinguishing a wrong type from an absent one
// must not re-refuse the empty string this tool was fixed to accept.
func TestFillStillSendsAnEmptyStringVerbatim(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	res := callTool(t, "pinchtab_fill", map[string]any{"selector": "e5", "value": ""}, srv)

	if res.IsError {
		t.Fatalf("an empty value was refused: %s", resultText(t, res))
	}
	if text, supplied := bridge.FillText(decodeActionJSON(t, body)); !supplied || text != "" {
		t.Errorf("posted %s, want a supplied empty text — the documented way to clear a field", body)
	}
}

// `<option value="">` is the standard placeholder, so an empty value is the one selection
// that resets a dropdown. The pair is the assertion: either half alone is satisfiable by
// deleting the check, which would forward a select with no value at all.
func TestSelectForwardsASuppliedEmptyValueAndStillRefusesAnAbsentOne(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	for _, key := range []string{"value", "option"} {
		t.Run("supplied empty "+key+" selects the placeholder", func(t *testing.T) {
			r := callTool(t, "pinchtab_select", map[string]any{"ref": "e0", key: ""}, srv)
			if r.IsError {
				t.Fatalf("a supplied empty %s was refused: %s", key, resultText(t, r))
			}
			body, _ := resultJSON(t, r)["body"].(map[string]any)
			got, supplied := bridge.SelectValue(decodeActionBody(t, body))
			if !supplied {
				t.Fatalf("the bridge decodes no supplied value, so it cannot tell a placeholder selection from a request whose value never arrived: %v", body)
			}
			if got != "" {
				t.Errorf("forwarded value = %q, want the empty string that selects the placeholder", got)
			}
		})
	}

	t.Run("absent value is refused", func(t *testing.T) {
		r := callTool(t, "pinchtab_select", map[string]any{"ref": "e0"}, srv)
		if !r.IsError {
			body, _ := resultJSON(t, r)["body"].(map[string]any)
			t.Fatalf("a select with no value at all was forwarded as %v; absent is not a placeholder", body)
		}
		message := resultText(t, r)
		if strings.Contains(message, "is missing") && strings.Contains(message, "'value'") {
			t.Errorf("refusal %q reads as a supplied parameter being missing; it must say what select needs", message)
		}
		if !strings.Contains(message, "empty-valued option") {
			t.Errorf("refusal %q does not tell the caller how to reach the placeholder, which is the case it is most likely to be confused with", message)
		}
	})
}

// The two verbs still reading the collapsing accessor answered "the argument is missing" to
// an argument the caller had sent under the wrong type — the same dead end fill was fixed to
// remove. The genuinely-absent row is the pair: collapsing the two again reds it.
func TestSelectAndTypeNameAWrongTypedArgumentInsteadOfCallingItMissing(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tool     string
		args     map[string]any
		wantSays string
	}{
		{name: "select a number", tool: "pinchtab_select", args: map[string]any{"selector": "e5", "value": float64(2024)}, wantSays: "must be a string, got number"},
		{name: "select a boolean", tool: "pinchtab_select", args: map[string]any{"selector": "e5", "value": true}, wantSays: "must be a string, got boolean"},
		{name: "select genuinely absent", tool: "pinchtab_select", args: map[string]any{"selector": "e5"}, wantSays: "needs a 'value' argument"},
		{name: "type a number", tool: "pinchtab_type", args: map[string]any{"selector": "e5", "text": float64(2024)}, wantSays: "must be a string, got number"},
		{name: "type an array", tool: "pinchtab_type", args: map[string]any{"selector": "e5", "text": []any{"a"}}, wantSays: "must be a string, got array"},
		{name: "type genuinely absent", tool: "pinchtab_type", args: map[string]any{"selector": "e5"}, wantSays: "needs a 'text' argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("an unusable argument was posted instead of refused")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			res := callTool(t, tc.tool, tc.args, srv)

			if !res.IsError {
				t.Fatalf("accepted %v", tc.args)
			}
			text := resultText(t, res)
			if !strings.Contains(text, tc.wantSays) {
				t.Errorf("error = %q, want it to say %q so the caller knows what to change", text, tc.wantSays)
			}
			assertNamesTheRemedy(t, text, tc.wantSays)
		})
	}
}

// The surfaces are compared on the WIRE, not against two copies of a rule: each builds its
// own body, and the bridge reads key presence, so only the posted bytes can show that a
// caller reaches the placeholder whichever surface it came in through. The raw API is the
// same body, asserted against a real page in internal/bridge.
func TestSelectSendsAnEmptyValueOnEverySurface(t *testing.T) {
	var mcpBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mcpBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	res := callTool(t, "pinchtab_select", map[string]any{"selector": "e5", "value": ""}, srv)
	if res.IsError {
		t.Fatalf("MCP refused an empty value: %s", resultText(t, res))
	}
	if value, supplied := bridge.SelectValue(decodeActionJSON(t, mcpBody)); !supplied || value != "" {
		t.Errorf("MCP posted %s, want a supplied empty value — supplied-ness is what the bridge reads", mcpBody)
	}

	if cliBody := cliSelectBody(t, ""); !suppliedEmptySelect(t, cliBody) {
		t.Errorf("the CLI posted %s, want a supplied empty value — the surfaces must express the same selection", cliBody)
	}
}

// cliSelectBody runs the CLI's own select builder against a recording server and returns the
// raw body it posted, so the assertion is on the wire rather than on two copies of a rule.
func cliSelectBody(t *testing.T, value string) string {
	t.Helper()
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("snap", false, "")
	cmd.Flags().Bool("snap-diff", false, "")
	cmd.Flags().Bool("text", false, "")
	browseractions.ActionSimple(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, "select", []string{"e5", value}, cmd)

	return string(raw)
}

// THE CARD'S MEASUREMENT, as a test: capture the outbound body on BOTH surfaces and
// compare them, rather than comparing constants — the two surfaces each used to build
// their own body from their own copy of the vocabulary, so only the bodies can show they
// now agree. internal/cli/actions is imported by the test binary only; production
// internal/mcp does not depend on the CLI.
func TestScrollDirectionMovesTheSameDistanceOnBothSurfaces(t *testing.T) {
	for _, keyword := range scroll.DirectionKeywords() {
		t.Run(keyword, func(t *testing.T) {
			srv := mockPinchTab()
			defer srv.Close()
			mcpBody, _ := resultJSON(t, callTool(t, "pinchtab_scroll", map[string]any{"direction": keyword}, srv))["body"].(map[string]any)

			cliBody := cliScrollBody(t, keyword)

			for _, field := range []string{"kind", "scrollX", "scrollY"} {
				if fmt.Sprint(mcpBody[field]) != fmt.Sprint(cliBody[field]) {
					t.Errorf("%q: MCP posted %s=%v, the CLI posted %s=%v — the same keyword must mean the same distance on both surfaces\nMCP: %v\nCLI: %v",
						keyword, field, mcpBody[field], field, cliBody[field], mcpBody, cliBody)
				}
			}
		})
	}
}

// cliScrollBody runs the CLI's own scroll builder against a recording server and returns
// the body it posted.
func cliScrollBody(t *testing.T, keyword string) map[string]any {
	t.Helper()
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("snap", false, "")
	cmd.Flags().Bool("snap-diff", false, "")
	cmd.Flags().Bool("text", false, "")
	cmd.Flags().Int("dy", 0, "")
	cmd.Flags().Int("dx", 0, "")
	browseractions.ActionSimple(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, "scroll", []string{keyword}, cmd)

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode the CLI's body %q: %v", raw, err)
	}
	return body
}

// Every keyword the owner accepts is reachable through MCP: horizontal scrolling used to
// be refused here while the CLI accepted it, so the vocabulary is asserted as a whole
// rather than by naming the two the handler happened to support.
func TestScrollDirectionAcceptsEveryOwnedKeyword(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	for _, keyword := range scroll.DirectionKeywords() {
		r := callTool(t, "pinchtab_scroll", map[string]any{"direction": keyword}, srv)
		if r.IsError {
			t.Errorf("direction %q was refused: %s", keyword, resultText(t, r))
			continue
		}
		body, _ := resultJSON(t, r)["body"].(map[string]any)
		want, _ := scroll.DirectionFor(keyword)
		if got, _ := body[want.Axis].(float64); got != float64(want.Delta) {
			t.Errorf("direction %q posted %s=%v, want %d", keyword, want.Axis, got, want.Delta)
		}
	}

	r := callTool(t, "pinchtab_scroll", map[string]any{"direction": "sideways"}, srv)
	if !r.IsError {
		t.Fatal("an unknown direction was accepted")
	}
	for _, keyword := range scroll.DirectionKeywords() {
		if !strings.Contains(resultText(t, r), keyword) {
			t.Errorf("the refusal %q does not name %q, so an agent cannot learn the vocabulary from it", resultText(t, r), keyword)
		}
	}
}

// A direction aimed at an element must still land AT the element. Routing the keyword to
// the page-scroll action put a selector in a body actionScroll short-circuits on, so it
// revealed the element instead of scrolling inside it — the direction, the distance and
// the SIGN all discarded, with scrolled:true in the response. The sign is asserted because
// it is the half that survives a wrong-magnitude fix: 'up' reading as a reveal looks like
// success from the outside.
func TestScrollDirectionAtAnElementStaysOnTheWheel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target map[string]any
	}{
		{name: "selector", target: map[string]any{"selector": "#feed"}},
		{name: "ref", target: map[string]any{"ref": "e5"}},
		{name: "nodeId", target: map[string]any{"nodeId": float64(42)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := mockPinchTab()
			defer srv.Close()

			args := map[string]any{"direction": "up"}
			maps.Copy(args, tc.target)
			body, _ := resultJSON(t, callTool(t, "pinchtab_scroll", args, srv))["body"].(map[string]any)

			if got, _ := body["kind"].(string); got != "mouse-wheel" {
				t.Errorf("kind = %q, want mouse-wheel; a page scroll carrying an element target is reinterpreted as scroll-into-view and the direction is dropped", got)
			}
			want, _ := scroll.DirectionFor("up")
			if got, _ := body[want.Axis].(float64); got != float64(want.Delta) {
				t.Errorf("%s = %v, want %d — the keyword magnitude and its sign must reach the element", want.Axis, body[want.Axis], want.Delta)
			}
		})
	}
}

// Both spellings carry a magnitude and nothing decides between them, so the request is
// refused rather than resolved by whichever the handler happens to read first. It used to
// drop the direction without a word.
func TestScrollDirectionRefusesACompetingDelta(t *testing.T) {
	for _, key := range []string{"deltaX", "deltaY"} {
		t.Run(key, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("a scroll carrying two magnitudes was posted instead of refused")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			res := callTool(t, "pinchtab_scroll", map[string]any{"direction": "down", key: float64(50)}, srv)

			if !res.IsError {
				t.Fatalf("accepted direction plus %s", key)
			}
			message := resultText(t, res)
			for _, want := range []string{"direction", "deltaX/deltaY"} {
				if !strings.Contains(message, want) {
					t.Errorf("refusal %q does not name %q, so the caller cannot tell which two settings collided", message, want)
				}
			}
		})
	}
}

// The tool description is where an agent learns the distance — the whole reason `steps`
// was unusable was that the magnitude appeared nowhere it reads.
func TestScrollToolDescriptionStatesTheMagnitudeAndEveryDirection(t *testing.T) {
	var scrollTool *mcp.Tool
	for i, tool := range allTools() {
		if tool.Name == "pinchtab_scroll" {
			scrollTool = &allTools()[i]
		}
	}
	if scrollTool == nil {
		t.Fatal("pinchtab_scroll is not declared")
	}
	direction, ok := scrollTool.InputSchema.Properties["direction"]
	if !ok {
		t.Fatal("pinchtab_scroll declares no direction argument")
	}
	described := fmt.Sprint(direction)

	if !strings.Contains(described, strconv.Itoa(scroll.StepPixels)) {
		t.Errorf("direction description %q states no magnitude; without it steps is only usable by a caller who already knows the default", described)
	}
	for _, keyword := range scroll.DirectionKeywords() {
		if !strings.Contains(described, keyword) {
			t.Errorf("direction description %q omits %q", described, keyword)
		}
	}
}

// The tool description derives its magnitude from the owner, so it cannot drift — but the
// agent-facing reference tables spell the number out, and those CAN. They are asserted
// against the owner too, so a change to the step reds here instead of silently leaving a
// doc that teaches the wrong distance.
func TestTheAgentFacingScrollDocsStateTheOwnersMagnitudeAndDirections(t *testing.T) {
	for _, doc := range []string{
		filepath.Join("..", "..", "docs", "reference", "mcp-tools.md"),
		filepath.Join("..", "..", "skills", "pinchtab", "references", "mcp.md"),
	} {
		raw, err := os.ReadFile(doc)
		if err != nil {
			t.Fatalf("cannot read %s, so this guard would not cover it: %v", doc, err)
		}
		row, ok := lineContaining(string(raw), "`pinchtab_scroll` |")
		if !ok {
			t.Errorf("%s no longer has a pinchtab_scroll row, so drift there is unguarded", doc)
			continue
		}
		if !strings.Contains(row, strconv.Itoa(scroll.StepPixels)) {
			t.Errorf("%s states no direction magnitude: %s", doc, row)
		}
		for _, keyword := range scroll.DirectionKeywords() {
			if !strings.Contains(row, keyword) {
				t.Errorf("%s omits direction %q: %s", doc, keyword, row)
			}
		}
	}
}

func lineContaining(text, marker string) (string, bool) {
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, marker) {
			return line, true
		}
	}
	return "", false
}

// Criterion: one owner. The handler must carry neither its own keyword switch nor its own
// magnitude literal, or the two surfaces can drift apart again exactly as they did.
func TestScrollHandlerCarriesNoOwnDirectionVocabularyOrMagnitude(t *testing.T) {
	pkg := srccensus.Load(t, ".", 15)
	text := pkg.FileSource(pkg.MustFunc(t, "buildDirectionScroll"))

	for _, keyword := range scroll.DirectionKeywords() {
		if strings.Contains(text, `case "`+keyword+`"`) {
			t.Errorf("the handler still switches on %q; the direction vocabulary has one owner, internal/scroll", keyword)
		}
	}
	if strings.Contains(text, "magnitude := 120") || strings.Contains(text, "= 120") {
		t.Error("the handler still carries a bare notch magnitude; the keyword distance comes from scroll.StepPixels")
	}
	if !strings.Contains(text, "scroll.DirectionFor(") {
		t.Error("the handler no longer reads the owner, so this guard is pinning nothing")
	}
}

// The selector aliases had this card's defect one argument over: a wrong-typed selector,
// ref, element, target or query collapsed into "required parameter 'selector' is missing",
// telling the caller an argument it sent is absent — on the key every action tool requires.
// The refusal must name the KEY the caller actually used, not always "selector".
func TestActionToolsNameAWrongTypedSelectorKeyInsteadOfCallingItMissing(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tool     string
		args     map[string]any
		wantSays string
	}{
		{name: "fill selector number", tool: "pinchtab_fill", args: map[string]any{"selector": float64(123), "value": "x"}, wantSays: "fill's 'selector' must be a string, got number"},
		{name: "fill ref number", tool: "pinchtab_fill", args: map[string]any{"ref": float64(5), "value": "x"}, wantSays: "fill's 'ref' must be a string, got number"},
		{name: "click target boolean", tool: "pinchtab_click", args: map[string]any{"target": true}, wantSays: "click's 'target' must be a string, got boolean"},
		{name: "click element array", tool: "pinchtab_click", args: map[string]any{"element": []any{"e5"}}, wantSays: "click's 'element' must be a string, got array"},
		{name: "click query number", tool: "pinchtab_click", args: map[string]any{"query": float64(5)}, wantSays: "click's 'query' must be a string, got number"},
		{name: "genuinely absent keeps its own message", tool: "pinchtab_click", args: map[string]any{}, wantSays: "required parameter 'selector' is missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("the tool posted a request for an unusable selector instead of refusing it")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			res := callTool(t, tc.tool, tc.args, srv)

			if !res.IsError {
				t.Fatalf("accepted %v", tc.args)
			}
			text := resultText(t, res)
			if !strings.Contains(text, tc.wantSays) {
				t.Errorf("error = %q, want it to say %q so the caller knows which key to fix", text, tc.wantSays)
			}
			assertNamesTheRemedy(t, text, tc.wantSays)
		})
	}
}

// The contract call, recorded as an assertion: a wrong-typed selector refuses OUTRIGHT
// even when another target could satisfy the action. Acting on nodeId or falling through
// to query while a mistyped ref is silently discarded is the wrong-target hazard — the
// caller meant SOMETHING by the key it sent.
func TestAWrongTypedSelectorRefusesInsteadOfActingOnAnotherTarget(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{name: "nodeId would have satisfied click", args: map[string]any{"nodeId": float64(3), "ref": float64(5)}},
		{name: "query would have satisfied click", args: map[string]any{"query": "the save button", "ref": float64(5)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("the tool acted on another target while discarding the mistyped selector")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			res := callTool(t, "pinchtab_click", tc.args, srv)
			if !res.IsError {
				t.Fatalf("accepted %v", tc.args)
			}
			if text := resultText(t, res); !strings.Contains(text, "click's 'ref' must be a string, got number") {
				t.Errorf("error = %q, want the mistyped ref named", text)
			}
		})
	}
}

// A later alias that yields a usable string still wins: the caller's intent is
// unambiguous, so a mistyped earlier key does not veto a valid one.
func TestAValidLaterSelectorAliasStillWinsOverAMistypedEarlierOne(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_fill", map[string]any{"selector": float64(9), "ref": "e5", "value": "x"}, srv)
	if r.IsError {
		t.Fatalf("refused despite a usable ref: %s", resultText(t, r))
	}
	body, _ := resultJSON(t, r)["body"].(map[string]any)
	if got, _ := body["selector"].(string); got != "e5" {
		t.Errorf("posted selector = %q, want the valid alias e5", got)
	}
}

func decodeActionJSON(t *testing.T, body string) bridge.ActionRequest {
	t.Helper()
	var req bridge.ActionRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("posted body %s does not decode as an action request: %v", body, err)
	}
	return req
}

func decodeActionBody(t *testing.T, body map[string]any) bridge.ActionRequest {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return decodeActionJSON(t, string(raw))
}

func suppliedEmptySelect(t *testing.T, body string) bool {
	t.Helper()
	value, supplied := bridge.SelectValue(decodeActionJSON(t, body))
	return supplied && value == ""
}

func suppliedScrollAxis(req bridge.ActionRequest, axis string) (int, bool) {
	switch axis {
	case "scrollX":
		return req.ScrollX, req.HasScroll
	case "scrollY":
		return req.ScrollY, req.HasScroll
	case "deltaX":
		return req.DeltaX, req.HasDelta
	case "deltaY":
		return req.DeltaY, req.HasDelta
	}
	return 0, false
}
