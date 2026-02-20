package bridge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRawAXValueString(t *testing.T) {
	tests := []struct {
		name string
		val  *RawAXValue
		want string
	}{
		{"nil", nil, ""},
		{"nil value", &RawAXValue{Type: "string"}, ""},
		{"string", &RawAXValue{Type: "string", Value: json.RawMessage(`"hello"`)}, "hello"},
		{"number", &RawAXValue{Type: "integer", Value: json.RawMessage(`42`)}, "42"},
		{"bool", &RawAXValue{Type: "boolean", Value: json.RawMessage(`true`)}, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.val.String()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInteractiveRoles(t *testing.T) {
	interactive := []string{"button", "link", "textbox", "checkbox", "radio", "tab", "menuitem"}
	for _, r := range interactive {
		if !InteractiveRoles[r] {
			t.Errorf("expected %q to be interactive", r)
		}
	}

	nonInteractive := []string{"heading", "paragraph", "image", "banner", "main", "navigation"}
	for _, r := range nonInteractive {
		if InteractiveRoles[r] {
			t.Errorf("expected %q to NOT be interactive", r)
		}
	}
}

func TestBuildSnapshot(t *testing.T) {
	nodes := []RawAXNode{
		{
			NodeID:           "root",
			Role:             &RawAXValue{Value: json.RawMessage(`"WebArea"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Test Page"`)},
			ChildIDs:         []string{"n1", "n2", "n3"},
			BackendDOMNodeID: 1,
		},
		{
			NodeID:           "n1",
			Role:             &RawAXValue{Value: json.RawMessage(`"button"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Submit"`)},
			BackendDOMNodeID: 10,
		},
		{
			NodeID:           "n2",
			Role:             &RawAXValue{Value: json.RawMessage(`"textbox"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Email"`)},
			BackendDOMNodeID: 20,
			Properties: []RawAXProp{
				{Name: "focused", Value: &RawAXValue{Value: json.RawMessage(`"true"`)}},
			},
		},
		{
			NodeID:  "n3",
			Ignored: true,
			Role:    &RawAXValue{Value: json.RawMessage(`"none"`)},
		},
		{
			NodeID:           "n4",
			Role:             &RawAXValue{Value: json.RawMessage(`"generic"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`""`)},
			BackendDOMNodeID: 30,
		},
	}

	flat, refs := BuildSnapshot(nodes, "", -1)

	if len(flat) != 3 {
		t.Fatalf("expected 3 nodes, got %d: %+v", len(flat), flat)
	}

	if refs["e0"] != 1 {
		t.Errorf("e0 should map to nodeID 1, got %d", refs["e0"])
	}
	if refs["e1"] != 10 {
		t.Errorf("e1 should map to nodeID 10, got %d", refs["e1"])
	}
	if refs["e2"] != 20 {
		t.Errorf("e2 should map to nodeID 20, got %d", refs["e2"])
	}

	if flat[0].Depth != 0 {
		t.Errorf("root depth should be 0, got %d", flat[0].Depth)
	}
	if flat[1].Depth != 1 {
		t.Errorf("button depth should be 1, got %d", flat[1].Depth)
	}

	if !flat[2].Focused {
		t.Error("textbox should be focused")
	}
}

func TestBuildSnapshotInteractiveFilter(t *testing.T) {
	nodes := []RawAXNode{
		{
			NodeID:           "root",
			Role:             &RawAXValue{Value: json.RawMessage(`"WebArea"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Page"`)},
			ChildIDs:         []string{"n1", "n2"},
			BackendDOMNodeID: 1,
		},
		{
			NodeID:           "n1",
			Role:             &RawAXValue{Value: json.RawMessage(`"heading"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Title"`)},
			BackendDOMNodeID: 10,
		},
		{
			NodeID:           "n2",
			Role:             &RawAXValue{Value: json.RawMessage(`"button"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Click me"`)},
			BackendDOMNodeID: 20,
		},
	}

	flat, _ := BuildSnapshot(nodes, FilterInteractive, -1)

	if len(flat) != 1 {
		t.Fatalf("expected 1 interactive node, got %d: %+v", len(flat), flat)
	}
	if flat[0].Role != "button" {
		t.Errorf("expected button, got %s", flat[0].Role)
	}
}

func TestFormatSnapshotText(t *testing.T) {
	nodes := []A11yNode{
		{Ref: "e0", Role: "WebArea", Name: "Page", Depth: 0},
		{Ref: "e1", Role: "button", Name: "Submit", Depth: 1},
		{Ref: "e2", Role: "textbox", Name: "Email", Depth: 1, Value: "test@x.com", Focused: true},
		{Ref: "e3", Role: "button", Name: "Cancel", Depth: 1, Disabled: true},
	}

	text := FormatSnapshotText(nodes)

	if !strings.Contains(text, `e0 WebArea "Page"`) {
		t.Error("missing root node")
	}
	if !strings.Contains(text, `  e1 button "Submit"`) {
		t.Error("missing indented button")
	}
	if !strings.Contains(text, `val="test@x.com"`) {
		t.Error("missing value")
	}
	if !strings.Contains(text, "[focused]") {
		t.Error("missing focused flag")
	}
	if !strings.Contains(text, "[disabled]") {
		t.Error("missing disabled flag")
	}
}

func TestDiffSnapshot(t *testing.T) {
	prev := []A11yNode{
		{Ref: "e0", Role: "button", Name: "Submit", NodeID: 10},
		{Ref: "e1", Role: "textbox", Name: "Email", NodeID: 20, Value: ""},
		{Ref: "e2", Role: "link", Name: "Old Link", NodeID: 30},
	}
	curr := []A11yNode{
		{Ref: "e0", Role: "button", Name: "Submit", NodeID: 10},
		{Ref: "e1", Role: "textbox", Name: "Email", NodeID: 20, Value: "hi"},
		{Ref: "e3", Role: "link", Name: "New Link", NodeID: 40},
	}

	added, changed, removed := DiffSnapshot(prev, curr)

	if len(added) != 1 || added[0].Name != "New Link" {
		t.Errorf("expected 1 added (New Link), got %+v", added)
	}
	if len(changed) != 1 || changed[0].Name != "Email" {
		t.Errorf("expected 1 changed (Email), got %+v", changed)
	}
	if len(removed) != 1 || removed[0].Name != "Old Link" {
		t.Errorf("expected 1 removed (Old Link), got %+v", removed)
	}
}

func TestBuildSnapshotDepthFilter(t *testing.T) {
	nodes := []RawAXNode{
		{
			NodeID:           "root",
			Role:             &RawAXValue{Value: json.RawMessage(`"WebArea"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Page"`)},
			ChildIDs:         []string{"n1"},
			BackendDOMNodeID: 1,
		},
		{
			NodeID:           "n1",
			Role:             &RawAXValue{Value: json.RawMessage(`"navigation"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Nav"`)},
			ChildIDs:         []string{"n2"},
			BackendDOMNodeID: 10,
		},
		{
			NodeID:           "n2",
			Role:             &RawAXValue{Value: json.RawMessage(`"link"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Home"`)},
			BackendDOMNodeID: 20,
		},
	}

	flat, _ := BuildSnapshot(nodes, "", 1)

	if len(flat) != 2 {
		t.Fatalf("expected 2 nodes at depth<=1, got %d: %+v", len(flat), flat)
	}
}
