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
	cmd.Flags().Int("wheel-delta-x", 0, "")
	cmd.Flags().Int("wheel-delta-y", 0, "")
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

	Action(client, m.base(), "", "mousedown", "", cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "mousedown" {
		t.Errorf("expected kind=mousedown, got %v", body["kind"])
	}
	if body["button"] != "right" {
		t.Errorf("expected button=right, got %v", body["button"])
	}
	if body["hasXY"] != true {
		t.Errorf("expected hasXY=true, got %v", body["hasXY"])
	}
}

func TestMouseWheelIncludesExplicitDeltas(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("wheel-delta-x", "120")
	_ = cmd.Flags().Set("wheel-delta-y", "-300")
	_ = cmd.Flags().Set("x", "10")
	_ = cmd.Flags().Set("y", "20")

	Action(client, m.base(), "", "mousewheel", "", cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "mousewheel" {
		t.Errorf("expected kind=mousewheel, got %v", body["kind"])
	}
	if body["wheelDeltaX"] != float64(120) {
		t.Errorf("expected wheelDeltaX=120, got %v", body["wheelDeltaX"])
	}
	if body["wheelDeltaY"] != float64(-300) {
		t.Errorf("expected wheelDeltaY=-300, got %v", body["wheelDeltaY"])
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
