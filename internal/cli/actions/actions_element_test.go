package actions

import (
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

func newActionCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("css", "", "")
	cmd.Flags().Bool("wait-nav", false, "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Float64("x", 0, "")
	cmd.Flags().Float64("y", 0, "")
	cmd.Flags().String("button", "", "")
	cmd.Flags().Int("dx", 0, "")
	cmd.Flags().Int("dy", 0, "")
	cmd.Flags().String("dialog-action", "", "")
	cmd.Flags().String("dialog-text", "", "")
	return cmd
}

func newSimpleCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
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

func TestDragPostsMouseSequence(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Drag(client, m.base(), "", []string{"e5", "400,320"}, cmd)

	if len(m.requests) != 4 {
		t.Fatalf("expected 4 requests, got %d", len(m.requests))
	}

	var bodies []map[string]any
	for _, req := range m.requests {
		var body map[string]any
		_ = json.Unmarshal([]byte(req.Body), &body)
		bodies = append(bodies, body)
	}
	if bodies[0]["kind"] != "mouse-move" || bodies[0]["ref"] != "e5" {
		t.Fatalf("unexpected first request: %+v", bodies[0])
	}
	if bodies[1]["kind"] != "mouse-down" {
		t.Fatalf("unexpected second request: %+v", bodies[1])
	}
	if bodies[2]["kind"] != "mouse-move" || bodies[2]["x"] != float64(400) || bodies[2]["y"] != float64(320) {
		t.Fatalf("unexpected third request: %+v", bodies[2])
	}
	if bodies[3]["kind"] != "mouse-up" {
		t.Fatalf("unexpected fourth request: %+v", bodies[3])
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

// ── Keyboard command tests ─────────────────────────────────────────────

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
	// Should not have selector or ref
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
