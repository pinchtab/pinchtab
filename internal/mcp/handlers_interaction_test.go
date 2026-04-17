package mcp

import (
	"strings"
	"testing"
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

	r := callTool(t, "pinchtab_press", map[string]any{
		"key": "Enter",
	}, srv)

	text := resultText(t, r)
	if !strings.Contains(text, "Enter") {
		t.Errorf("expected Enter in response, got %s", text)
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

func TestHandleScrollDirectionUsesMouseWheel(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_scroll", map[string]any{
		"direction": "down",
		"steps":     float64(2),
	}, srv)

	resp := resultJSON(t, r)
	body, _ := resp["body"].(map[string]any)
	if got, _ := body["kind"].(string); got != "mouse-wheel" {
		t.Fatalf("kind = %q, want mouse-wheel", got)
	}
	if got, _ := body["deltaY"].(float64); got != 240 {
		t.Fatalf("deltaY = %v, want 240", got)
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
