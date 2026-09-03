package actions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newActionCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("css", "", "")
	cmd.Flags().Bool("wait-nav", false, "")
	cmd.Flags().Bool("dismiss-banners", false, "")
	cmd.Flags().Bool("dismiss-known-interstitials", false, "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Float64("x", 0, "")
	cmd.Flags().Float64("y", 0, "")
	cmd.Flags().String("button", "", "")
	cmd.Flags().Int("dx", 0, "")
	cmd.Flags().Int("dy", 0, "")
	cmd.Flags().String("dialog-action", "", "")
	cmd.Flags().String("dialog-text", "", "")
	cmd.Flags().String("mode", "", "")
	cmd.Flags().Bool("submit", false, "")
	return cmd
}

func newSimpleCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("snap", false, "")
	cmd.Flags().Bool("snap-diff", false, "")
	cmd.Flags().Bool("text", false, "")
	cmd.Flags().Bool("submit", false, "")
	return cmd
}

func TestClick(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(client, m.base(), "", "click", "e5", cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "click" {
		t.Errorf("expected kind=click, got %v", body["kind"])
	}
	if body["ref"] != "e5" {
		t.Errorf("expected ref=e5, got %v", body["ref"])
	}
}

func TestClickSubmit(t *testing.T) {
	m := newMockServer()
	defer m.close()
	cmd := newActionCmd()
	if err := cmd.Flags().Set("submit", "true"); err != nil {
		t.Fatal(err)
	}

	Action(m.server.Client(), m.base(), "", "click", "e5", cmd)
	var body map[string]any
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatal(err)
	}
	if body["submit"] != true {
		t.Fatalf("submit = %v, want true", body["submit"])
	}
}

func TestPrintActionResultSubmitPendingIsNotOK(t *testing.T) {
	got := captureStdout(t, func() {
		printActionResult("click", map[string]any{
			"success": true,
			"result": map[string]any{
				"postState": map[string]any{"status": "pending"},
			},
		})
	})
	if got != "PENDING\n" {
		t.Fatalf("pending output = %q, want PENDING", got)
	}
}

func TestPrintActionResultSubmitSuccessNamesObservedSignal(t *testing.T) {
	got := captureStdout(t, func() {
		printActionResult("click", map[string]any{
			"success": true,
			"result": map[string]any{
				"postState": map[string]any{"status": "succeeded", "signal": "dialog_closed"},
			},
		})
	})
	if got != "SUCCEEDED dialog_closed\n" {
		t.Fatalf("success output = %q, want observed submit signal", got)
	}
}

func TestClickWaitNav(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("wait-nav", "true")
	Action(client, m.base(), "", "click", "e5", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["waitNav"] != true {
		t.Error("expected waitNav=true")
	}
}

func TestClickDismissBannersWithWaitNav(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("wait-nav", "true")
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Action(client, m.base(), "", "click", "e5", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["waitNav"] != true {
		t.Error("expected waitNav=true")
	}
	if body["dismissBanners"] != true {
		t.Errorf("expected dismissBanners=true, got %v", body["dismissBanners"])
	}
}

func TestClickDismissBannersWithoutWaitNavIsNoop(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Action(client, m.base(), "", "click", "e5", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if _, ok := body["dismissBanners"]; ok {
		t.Errorf("expected dismissBanners not sent without --wait-nav, got %v", body["dismissBanners"])
	}
}

func TestClickDismissKnownInterstitialsIsIndependentOfWaitNav(t *testing.T) {
	m := newMockServer()
	defer m.close()
	cmd := newActionCmd()
	_ = cmd.Flags().Set("dismiss-known-interstitials", "true")

	Action(m.server.Client(), m.base(), "", "click", "e5", cmd)
	var body map[string]any
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatal(err)
	}
	if body["dismissKnownInterstitials"] != true {
		t.Fatalf("dismissKnownInterstitials = %v, want true", body["dismissKnownInterstitials"])
	}
}

func TestClickMode(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("mode", "dom")
	Action(client, m.base(), "", "click", "e5", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["mode"] != "dom" {
		t.Errorf("expected mode=dom, got %v", body["mode"])
	}
}

func TestClickDialogAction(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("dialog-action", "accept")
	_ = cmd.Flags().Set("dialog-text", "hello")
	Action(client, m.base(), "", "click", "#alert-btn", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["dialogAction"] != "accept" {
		t.Errorf("expected dialogAction=accept, got %v", body["dialogAction"])
	}
	if body["dialogText"] != "hello" {
		t.Errorf("expected dialogText=hello, got %v", body["dialogText"])
	}
}

func TestClickDialogActionOmittedByDefault(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(client, m.base(), "", "click", "#button", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if _, present := body["dialogAction"]; present {
		t.Errorf("expected dialogAction to be omitted, got %v", body["dialogAction"])
	}
	if _, present := body["dialogText"]; present {
		t.Errorf("expected dialogText to be omitted, got %v", body["dialogText"])
	}
}

func TestType(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(client, m.base(), "", "type", []string{"e12", "hello", "world"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "type" {
		t.Errorf("expected kind=type, got %v", body["kind"])
	}
	if body["ref"] != "e12" {
		t.Errorf("expected ref=e12, got %v", body["ref"])
	}
	if body["text"] != "hello world" {
		t.Errorf("expected text='hello world', got %v", body["text"])
	}
}

func TestPress(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(client, m.base(), "", "press", []string{"Enter"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["key"] != "Enter" {
		t.Errorf("expected key=Enter, got %v", body["key"])
	}
}

func TestPressWithSnapDiffFetchesSnapshot(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()
	m.setResponse("GET", "/snapshot", 200, "snapshot")

	cmd := newSimpleCmd()
	_ = cmd.Flags().Set("snap-diff", "true")
	ActionSimple(client, m.base(), "", "press", []string{"Enter"}, cmd)

	if len(m.requests) != 2 {
		t.Fatalf("expected 2 requests (action + snapshot), got %d", len(m.requests))
	}
	if m.requests[1].Path != "/snapshot" {
		t.Fatalf("snapshot path = %q, want /snapshot", m.requests[1].Path)
	}
	if m.requests[1].Query != "filter=interactive&format=compact&diff=true" {
		t.Fatalf("snapshot query = %q", m.requests[1].Query)
	}
}

func TestClickWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", "button.submit")
	Action(client, m.base(), "", "click", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "button.submit" {
		t.Errorf("expected selector=button.submit, got %v", body["selector"])
	}
	if _, hasRef := body["ref"]; hasRef {
		t.Error("should not set ref when --css is provided")
	}
}

func TestClickWithCSS_AndWaitNav(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("wait-nav", "true")
	_ = cmd.Flags().Set("css", "#login-btn")
	Action(client, m.base(), "", "click", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "#login-btn" {
		t.Errorf("expected selector=#login-btn, got %v", body["selector"])
	}
	if body["waitNav"] != true {
		t.Error("expected waitNav=true")
	}
}

func TestMouseDownIncludesButton(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("button", "right")
	_ = cmd.Flags().Set("x", "25")
	_ = cmd.Flags().Set("y", "40")

	MouseAction(client, m.base(), "", "mouse-down", nil, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "mouse-down" {
		t.Errorf("expected kind=mouse-down, got %v", body["kind"])
	}
	if body["button"] != "right" {
		t.Errorf("expected button=right, got %v", body["button"])
	}
	if body["x"] != float64(25) || body["y"] != float64(40) {
		t.Errorf("expected x/y coordinates, got %v", body)
	}
}

func TestMouseWheelIncludesExplicitDeltas(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("dx", "120")
	_ = cmd.Flags().Set("dy", "-300")
	_ = cmd.Flags().Set("x", "10")
	_ = cmd.Flags().Set("y", "20")

	MouseAction(client, m.base(), "", "mouse-wheel", nil, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "mouse-wheel" {
		t.Errorf("expected kind=mouse-wheel, got %v", body["kind"])
	}
	if body["deltaX"] != float64(120) {
		t.Errorf("expected deltaX=120, got %v", body["deltaX"])
	}
	if body["deltaY"] != float64(-300) {
		t.Errorf("expected deltaY=-300, got %v", body["deltaY"])
	}
}

func TestMouseMoveSupportsPositionalCoordinates(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	MouseAction(client, m.base(), "", "mouse-move", []string{"100", "200"}, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "mouse-move" {
		t.Fatalf("expected kind=mouse-move, got %v", body["kind"])
	}
	if body["x"] != float64(100) || body["y"] != float64(200) {
		t.Fatalf("expected positional coordinates, got %v", body)
	}
}

func TestMouseWheelSupportsPositionalDeltaY(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("dx", "20")
	MouseAction(client, m.base(), "", "mouse-wheel", []string{"-120"}, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["deltaX"] != float64(20) {
		t.Fatalf("expected deltaX=20, got %v", body["deltaX"])
	}
	if body["deltaY"] != float64(-120) {
		t.Fatalf("expected deltaY=-120, got %v", body["deltaY"])
	}
}

// The from->to form used to post four independent pointer requests, and four requests
// cannot interpolate: the pointer jumped from source to destination in one move, which
// Chrome never reads as the start of a drag, so an HTML5 draggable saw nothing while all
// four answered OK. The destination has to travel WITH the source in one action.
func TestDragPostsOneActionCarryingItsDestination(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Drag(client, m.base(), "", []string{"e5", "e9"}, cmd)

	if len(m.requests) != 1 {
		t.Fatalf("expected 1 request, got %d; a drag assembled from several requests cannot interpolate the pointer", len(m.requests))
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "drag" {
		t.Errorf("kind = %v, want drag", body["kind"])
	}
	if body["ref"] != "e5" {
		t.Errorf("source = %+v, want ref e5", body)
	}
	if body["toSelector"] != "e9" {
		t.Errorf("destination = %+v, want toSelector e9", body)
	}
}

func TestDragToCoordinatesPostsThemAsTheDestination(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Drag(client, m.base(), "", []string{"e5", "400,320"}, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["toX"] != float64(400) || body["toY"] != float64(320) {
		t.Errorf("destination = %+v, want toX=400 toY=320", body)
	}
	if _, ok := body["toSelector"]; ok {
		t.Errorf("destination = %+v, want no toSelector when the target is a coordinate pair", body)
	}
}

func TestHoverWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", ".nav-item")
	Action(client, m.base(), "", "hover", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != ".nav-item" {
		t.Errorf("expected selector=.nav-item, got %v", body["selector"])
	}
}

func TestFocus(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(client, m.base(), "", "focus", "e5", cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "focus" {
		t.Errorf("expected kind=focus, got %v", body["kind"])
	}
	if body["ref"] != "e5" {
		t.Errorf("expected ref=e5, got %v", body["ref"])
	}
}

func TestFocusWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", "input[name='email']")
	Action(client, m.base(), "", "focus", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "input[name='email']" {
		t.Errorf("expected selector=input[name='email'], got %v", body["selector"])
	}
}

func TestClickRefStillWorks(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(client, m.base(), "", "click", "e42", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e42" {
		t.Errorf("expected ref=e42, got %v", body["ref"])
	}
	if _, hasSelector := body["selector"]; hasSelector {
		t.Error("should not set selector when using ref")
	}
}

func TestFill(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(client, m.base(), "", "fill", []string{"e3", "test value"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e3" {
		t.Errorf("expected ref=e3, got %v", body["ref"])
	}
	if body["text"] != "test value" {
		t.Errorf("expected text='test value', got %v", body["text"])
	}

	ActionSimple(client, m.base(), "", "fill", []string{"#email", "user@test.com"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "#email" {
		t.Errorf("expected selector=#email, got %v", body["selector"])
	}

	ActionSimple(client, m.base(), "", "fill", []string{"embed", "inline content"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "embed" {
		t.Errorf("expected selector=embed, got %v", body["selector"])
	}
	if _, hasRef := body["ref"]; hasRef {
		t.Errorf("expected no ref for selector embed, got %v", body["ref"])
	}
}

func TestFillSubmit(t *testing.T) {
	m := newMockServer()
	defer m.close()
	cmd := newSimpleCmd()
	if err := cmd.Flags().Set("submit", "true"); err != nil {
		t.Fatal(err)
	}

	ActionSimple(m.server.Client(), m.base(), "", "fill", []string{"#search", "needle"}, cmd)
	var body map[string]any
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatal(err)
	}
	if body["submit"] != true {
		t.Fatalf("submit = %v, want true", body["submit"])
	}
}

func TestScroll(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(client, m.base(), "", "scroll", []string{"e20"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e20" {
		t.Errorf("expected ref=e20, got %v", body["ref"])
	}

	ActionSimple(client, m.base(), "", "scroll", []string{"800"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["scrollY"] != float64(800) {
		t.Errorf("expected scrollY=800, got %v", body["scrollY"])
	}

	ActionSimple(client, m.base(), "", "scroll", []string{"down"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["scrollY"] != float64(800) {
		t.Errorf("expected scrollY=800 for direction=down, got %v", body["scrollY"])
	}

	// CSS selector auto-detection: `scroll #footer` should forward as
	// selector, matching how click/fill/hover behave for bare selectors.
	ActionSimple(client, m.base(), "", "scroll", []string{"#footer"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "#footer" {
		t.Errorf("expected selector=#footer, got %v", body["selector"])
	}
	if _, hasScrollY := body["scrollY"]; hasScrollY {
		t.Errorf("should not set scrollY for CSS selector form, got %v", body["scrollY"])
	}

	// XPath also flows through.
	ActionSimple(client, m.base(), "", "scroll", []string{"//footer"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "//footer" {
		t.Errorf("expected selector=//footer, got %v", body["selector"])
	}
}

func TestCheck(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(client, m.base(), "", "check", "e7", cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "check" {
		t.Errorf("expected kind=check, got %v", body["kind"])
	}
	if body["ref"] != "e7" {
		t.Errorf("expected ref=e7, got %v", body["ref"])
	}
	if _, hasSelector := body["selector"]; hasSelector {
		t.Error("should not set selector when using ref")
	}
}

func TestCheckWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", "input[type=checkbox]")
	Action(client, m.base(), "", "check", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "check" {
		t.Errorf("expected kind=check, got %v", body["kind"])
	}
	if body["selector"] != "input[type=checkbox]" {
		t.Errorf("expected selector=input[type=checkbox], got %v", body["selector"])
	}
	if _, hasRef := body["ref"]; hasRef {
		t.Error("should not set ref when --css is provided")
	}
}

func TestUncheck(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(client, m.base(), "", "uncheck", "e9", cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "uncheck" {
		t.Errorf("expected kind=uncheck, got %v", body["kind"])
	}
	if body["ref"] != "e9" {
		t.Errorf("expected ref=e9, got %v", body["ref"])
	}
	if _, hasSelector := body["selector"]; hasSelector {
		t.Error("should not set selector when using ref")
	}
}

func TestUncheckWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", "#agree-checkbox")
	Action(client, m.base(), "", "uncheck", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "uncheck" {
		t.Errorf("expected kind=uncheck, got %v", body["kind"])
	}
	if body["selector"] != "#agree-checkbox" {
		t.Errorf("expected selector=#agree-checkbox, got %v", body["selector"])
	}
	if _, hasRef := body["ref"]; hasRef {
		t.Error("should not set ref when --css is provided")
	}
}

func TestSelect(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(client, m.base(), "", "select", []string{"e10", "option2"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e10" {
		t.Errorf("expected ref=e10, got %v", body["ref"])
	}
	if body["value"] != "option2" {
		t.Errorf("expected value=option2, got %v", body["value"])
	}
}

func TestKeyboardType(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(client, m.base(), "", "keyboard-type", []string{"hello", "world"}, cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keyboard-type" {
		t.Errorf("expected kind=keyboard-type, got %v", body["kind"])
	}
	if body["text"] != "hello world" {
		t.Errorf("expected text='hello world', got %v", body["text"])
	}
	if _, has := body["selector"]; has {
		t.Error("keyboard-type should not have selector")
	}
	if _, has := body["ref"]; has {
		t.Error("keyboard-type should not have ref")
	}
}

func TestKeyboardInsertText(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(client, m.base(), "", "keyboard-inserttext", []string{"pasted", "text"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keyboard-inserttext" {
		t.Errorf("expected kind=keyboard-inserttext, got %v", body["kind"])
	}
	if body["text"] != "pasted text" {
		t.Errorf("expected text='pasted text', got %v", body["text"])
	}
}

func TestKeyDown(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(client, m.base(), "", "keydown", []string{"Control"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keydown" {
		t.Errorf("expected kind=keydown, got %v", body["kind"])
	}
	if body["key"] != "Control" {
		t.Errorf("expected key=Control, got %v", body["key"])
	}
}

func TestKeyUp(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(client, m.base(), "", "keyup", []string{"Shift"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keyup" {
		t.Errorf("expected kind=keyup, got %v", body["kind"])
	}
	if body["key"] != "Shift" {
		t.Errorf("expected key=Shift, got %v", body["key"])
	}
}

func TestKeyDownWithTab(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	_ = cmd.Flags().Set("tab", "abc123")
	ActionSimple(client, m.base(), "", "keydown", []string{"Alt"}, cmd)
	if m.lastPath != "/tabs/abc123/action" {
		t.Errorf("expected /tabs/abc123/action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keydown" {
		t.Errorf("expected kind=keydown, got %v", body["kind"])
	}
	if body["key"] != "Alt" {
		t.Errorf("expected key=Alt, got %v", body["key"])
	}
}

func TestKeyboardTypeWithTab(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	_ = cmd.Flags().Set("tab", "tab42")
	ActionSimple(client, m.base(), "", "keyboard-type", []string{"test"}, cmd)
	if m.lastPath != "/tabs/tab42/action" {
		t.Errorf("expected /tabs/tab42/action, got %s", m.lastPath)
	}
}

// newScrollCmd carries the flags cmd/pinchtab registers on `scroll`, so a body built here is
// built from the same inputs production reads.
func newScrollCmd() *cobra.Command {
	cmd := newSimpleCmd()
	cmd.Flags().Int("dy", 0, "")
	cmd.Flags().Int("dx", 0, "")
	return cmd
}

// Scrolling up by an exact pixel count is what had no working spelling: a positional "-300"
// cannot reach the command (cobra reads it as shorthand flags), and `-- -300` swallowed
// --tab. The flag is the reachable route, and negatives are fine as flag VALUES.
func TestScrollByPixelFlags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]string
		want  map[string]float64
	}{
		{name: "up", flags: map[string]string{"dy": "-300"}, want: map[string]float64{"scrollY": -300}},
		{name: "down", flags: map[string]string{"dy": "800"}, want: map[string]float64{"scrollY": 800}},
		{name: "left", flags: map[string]string{"dx": "-120"}, want: map[string]float64{"scrollX": -120}},
		{name: "both axes", flags: map[string]string{"dy": "-300", "dx": "40"}, want: map[string]float64{"scrollY": -300, "scrollX": 40}},
	} {
		m := newMockServer()
		cmd := newScrollCmd()
		for flag, value := range tc.flags {
			if err := cmd.Flags().Set(flag, value); err != nil {
				t.Fatal(err)
			}
		}

		ActionSimple(m.server.Client(), m.base(), "", "scroll", nil, cmd)

		var body map[string]any
		_ = json.Unmarshal([]byte(m.lastBody), &body)
		for key, want := range tc.want {
			if body[key] != want {
				t.Errorf("%s: %s = %v, want %v (body %+v)", tc.name, key, body[key], want, body)
			}
		}
		m.close()
	}
}

// The positional forms are what everything else in the docs teaches, and adding the flags
// touched the same precedence path that resolves them.
func TestScrollPositionalFormsAreUnchangedByThePixelFlags(t *testing.T) {
	for _, tc := range []struct {
		arg  string
		key  string
		want any
	}{
		{arg: "800", key: "scrollY", want: float64(800)},
		{arg: "down", key: "scrollY", want: float64(800)},
		{arg: "up", key: "scrollY", want: float64(-800)},
		{arg: "right", key: "scrollX", want: float64(800)},
		{arg: "left", key: "scrollX", want: float64(-800)},
		{arg: "e12", key: "ref", want: "e12"},
		{arg: "#footer", key: "selector", want: "#footer"},
		{arg: "text:Load more", key: "selector", want: "text:Load more"},
	} {
		m := newMockServer()
		ActionSimple(m.server.Client(), m.base(), "", "scroll", []string{tc.arg}, newScrollCmd())

		var body map[string]any
		_ = json.Unmarshal([]byte(m.lastBody), &body)
		if body[tc.key] != tc.want {
			t.Errorf("scroll %q: %s = %v, want %v (body %+v)", tc.arg, tc.key, body[tc.key], tc.want, body)
		}
		m.close()
	}
}

// ActionSimple is exported and reachable without cobra's Args hook, so the CLI's
// refusal of the both-specified form is a friendly early error, not the thing that
// keeps this correct. The rule has to live in the builder: a positional wins
// outright. Before it did, the flags were assigned first and the positional then
// overwrote scrollY — so `800` with `--dx -100` built a diagonal scroll out of one
// axis from each spelling, resolved by statement order rather than by any rule.
func TestScrollPositionalWinsOverThePixelFlagsWithoutCobrasArgsHook(t *testing.T) {
	for _, tc := range []struct {
		name   string
		arg    string
		flags  map[string]string
		want   map[string]any
		absent []string
	}{
		{
			name:   "same axis",
			arg:    "800",
			flags:  map[string]string{"dy": "-300"},
			want:   map[string]any{"scrollY": float64(800)},
			absent: []string{"scrollX"},
		},
		{
			name:   "other axis must not survive as half a diagonal",
			arg:    "800",
			flags:  map[string]string{"dx": "-100"},
			want:   map[string]any{"scrollY": float64(800)},
			absent: []string{"scrollX"},
		},
		{
			name:   "a direction keyword also wins",
			arg:    "left",
			flags:  map[string]string{"dy": "-300"},
			want:   map[string]any{"scrollX": float64(-800)},
			absent: []string{"scrollY"},
		},
		{
			name:   "a selector wins and carries no delta",
			arg:    "e12",
			flags:  map[string]string{"dy": "-300", "dx": "40"},
			want:   map[string]any{"ref": "e12"},
			absent: []string{"scrollX", "scrollY"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockServer()
			defer m.close()
			cmd := newScrollCmd()
			for flag, value := range tc.flags {
				if err := cmd.Flags().Set(flag, value); err != nil {
					t.Fatal(err)
				}
			}

			ActionSimple(m.server.Client(), m.base(), "", "scroll", []string{tc.arg}, cmd)

			var body map[string]any
			if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
				t.Fatalf("decode body: %v (%s)", err, m.lastBody)
			}
			for key, want := range tc.want {
				if body[key] != want {
					t.Errorf("%s = %v, want %v (body %+v)", key, body[key], want, body)
				}
			}
			for _, key := range tc.absent {
				if _, present := body[key]; present {
					t.Errorf("%s is present (%v); the positional is the whole argument, so no flag axis may survive alongside it (body %+v)", key, body[key], body)
				}
			}
		})
	}
}

// The repro from the card: clicking an ordinary link. It used to print an error and
// exit 1 after the click had already worked; it now prints where the tab landed and
// exits 0. captureStdout would not see an exit, so the assertion is on the output —
// output.Error is the only path that exits, and reaching this branch means it did not.
func TestPrintActionResultANavigatingClickReportsWhereItLanded(t *testing.T) {
	navigated := map[string]any{
		"success": true,
		"result": map[string]any{
			"clicked":     true,
			"navigated":   true,
			"url":         "https://www.iana.org/help/example-domains",
			"previousUrl": "https://example.com/",
			"refsStale":   true,
		},
	}

	out := captureStdout(t, func() { printActionResult("click", navigated) })
	if !strings.Contains(out, "https://www.iana.org/help/example-domains") {
		t.Errorf("stdout = %q; the caller cannot see where the click landed", out)
	}

	hint := captureStderr(t, func() { printActionResult("click", navigated) })
	if !strings.Contains(hint, "snap") {
		t.Errorf("stderr = %q; nothing tells the caller its refs are dead", hint)
	}
}

// A click that did not move the page keeps the plain answer: the landed-URL line is
// the navigation report, not decoration on every click.
func TestPrintActionResultAClickThatDidNotNavigateStaysPlain(t *testing.T) {
	got := captureStdout(t, func() {
		printActionResult("click", map[string]any{
			"success": true,
			"result":  map[string]any{"clicked": true},
		})
	})
	if strings.Contains(got, "navigated") {
		t.Errorf("output = %q; a click that moved nothing reported a navigation", got)
	}
}
