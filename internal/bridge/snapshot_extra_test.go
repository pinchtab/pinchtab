package bridge

import (
	"encoding/json"
	"testing"
)

func TestFormatSnapshotCompact_Basic(t *testing.T) {
	nodes := []A11yNode{
		{Ref: "e0", Role: "button", Name: "Submit"},
		{Ref: "e1", Role: "textbox", Name: "Email", Value: "test@example.com"},
	}
	got := FormatSnapshotCompact(nodes)
	want := "e0:button \"Submit\"\ne1:textbox \"Email\" val=\"test@example.com\"\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatSnapshotCompact_FocusedDisabled(t *testing.T) {
	nodes := []A11yNode{
		{Ref: "e0", Role: "button", Name: "OK", Focused: true},
		{Ref: "e1", Role: "button", Name: "Cancel", Disabled: true},
		{Ref: "e2", Role: "textbox", Focused: true, Disabled: true},
	}
	got := FormatSnapshotCompact(nodes)
	if !contains(got, "e0:button \"OK\" *\n") {
		t.Errorf("expected focused marker *, got:\n%s", got)
	}
	if !contains(got, "e1:button \"Cancel\" -\n") {
		t.Errorf("expected disabled marker -, got:\n%s", got)
	}
	if !contains(got, "e2:textbox * -\n") {
		t.Errorf("expected both markers, got:\n%s", got)
	}
}

func TestFormatSnapshotCompact_Empty(t *testing.T) {
	got := FormatSnapshotCompact(nil)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFormatSnapshotCompact_NoName(t *testing.T) {
	nodes := []A11yNode{{Ref: "e0", Role: "generic"}}
	got := FormatSnapshotCompact(nodes)
	if got != "e0:generic\n" {
		t.Errorf("got %q", got)
	}
}

func TestTruncateToTokens_NoTruncation(t *testing.T) {
	nodes := []A11yNode{
		{Ref: "e0", Role: "button", Name: "OK"},
		{Ref: "e1", Role: "link", Name: "Home"},
	}
	result, truncated := TruncateToTokens(nodes, 1000, "compact")
	if truncated {
		t.Error("expected no truncation")
	}
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}
}

func TestTruncateToTokens_Truncates(t *testing.T) {
	nodes := make([]A11yNode, 100)
	for i := range nodes {
		nodes[i] = A11yNode{Ref: "e0", Role: "button", Name: "A long button name here"}
	}
	result, truncated := TruncateToTokens(nodes, 50, "compact")
	if !truncated {
		t.Error("expected truncation")
	}
	if len(result) >= 100 {
		t.Errorf("expected fewer than 100 nodes, got %d", len(result))
	}
}

func TestTruncateToTokens_Formats(t *testing.T) {
	nodes := make([]A11yNode, 50)
	for i := range nodes {
		nodes[i] = A11yNode{Ref: "e0", Role: "button", Name: "Click me"}
	}

	jsonResult, _ := TruncateToTokens(nodes, 100, "json")
	compactResult, _ := TruncateToTokens(nodes, 100, "compact")
	if len(jsonResult) > len(compactResult) {
		t.Errorf("JSON should truncate sooner: json=%d compact=%d", len(jsonResult), len(compactResult))
	}
}

func TestTruncateToTokens_Empty(t *testing.T) {
	result, truncated := TruncateToTokens(nil, 100, "compact")
	if truncated {
		t.Error("expected no truncation on empty")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result))
	}
}

func TestFilterSubtree_Found(t *testing.T) {
	nodes := []RawAXNode{
		{NodeID: "root", BackendDOMNodeID: 1, ChildIDs: []string{"child1", "child2"}},
		{NodeID: "child1", BackendDOMNodeID: 2, ChildIDs: []string{"grandchild"}},
		{NodeID: "child2", BackendDOMNodeID: 3},
		{NodeID: "grandchild", BackendDOMNodeID: 4},
		{NodeID: "other", BackendDOMNodeID: 5},
	}

	result := FilterSubtree(nodes, 2)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}
	ids := map[string]bool{}
	for _, n := range result {
		ids[n.NodeID] = true
	}
	if !ids["child1"] || !ids["grandchild"] {
		t.Errorf("expected child1 and grandchild, got %v", ids)
	}
}

func TestFilterSubtree_NotFound(t *testing.T) {
	nodes := []RawAXNode{
		{NodeID: "root", BackendDOMNodeID: 1},
		{NodeID: "child", BackendDOMNodeID: 2},
	}

	result := FilterSubtree(nodes, 999)
	if len(result) != 2 {
		t.Errorf("expected all nodes returned, got %d", len(result))
	}
}

func TestFilterSubtree_RootScope(t *testing.T) {
	nodes := []RawAXNode{
		{NodeID: "root", BackendDOMNodeID: 1, ChildIDs: []string{"child"}},
		{NodeID: "child", BackendDOMNodeID: 2, ChildIDs: []string{"grandchild"}},
		{NodeID: "grandchild", BackendDOMNodeID: 3},
	}

	result := FilterSubtree(nodes, 1)
	if len(result) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(result))
	}
}

func TestDiffSnapshot_Added(t *testing.T) {
	prev := []A11yNode{{Ref: "e0", Role: "button", Name: "OK", NodeID: 1}}
	curr := []A11yNode{
		{Ref: "e0", Role: "button", Name: "OK", NodeID: 1},
		{Ref: "e1", Role: "link", Name: "New", NodeID: 2},
	}
	added, changed, removed := DiffSnapshot(prev, curr)
	if len(added) != 1 || added[0].Name != "New" {
		t.Errorf("expected 1 added node, got %v", added)
	}
	if len(changed) != 0 {
		t.Errorf("expected 0 changed, got %d", len(changed))
	}
	if len(removed) != 0 {
		t.Errorf("expected 0 removed, got %d", len(removed))
	}
}

func TestDiffSnapshot_Removed(t *testing.T) {
	prev := []A11yNode{
		{Ref: "e0", Role: "button", Name: "OK", NodeID: 1},
		{Ref: "e1", Role: "link", Name: "Old", NodeID: 2},
	}
	curr := []A11yNode{{Ref: "e0", Role: "button", Name: "OK", NodeID: 1}}
	_, _, removed := DiffSnapshot(prev, curr)
	if len(removed) != 1 || removed[0].Name != "Old" {
		t.Errorf("expected 1 removed node, got %v", removed)
	}
}

func TestDiffSnapshot_Changed(t *testing.T) {
	prev := []A11yNode{{Ref: "e0", Role: "textbox", Name: "Email", NodeID: 1, Value: "old"}}
	curr := []A11yNode{{Ref: "e0", Role: "textbox", Name: "Email", NodeID: 1, Value: "new"}}
	_, changed, _ := DiffSnapshot(prev, curr)
	if len(changed) != 1 || changed[0].Value != "new" {
		t.Errorf("expected 1 changed node, got %v", changed)
	}
}

func TestDiffSnapshot_Empty(t *testing.T) {
	added, changed, removed := DiffSnapshot(nil, nil)
	if len(added)+len(changed)+len(removed) != 0 {
		t.Error("expected all empty for nil inputs")
	}
}

func TestRawAXValue_String_Normal(t *testing.T) {
	v := &RawAXValue{Type: "string", Value: json.RawMessage(`"hello"`)}
	if got := v.String(); got != "hello" {
		t.Errorf("expected hello, got %q", got)
	}
}

func TestRawAXValue_String_Nil(t *testing.T) {
	var v *RawAXValue
	if got := v.String(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRawAXValue_String_NilValue(t *testing.T) {
	v := &RawAXValue{Type: "string"}
	if got := v.String(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRawAXValue_String_NonString(t *testing.T) {
	v := &RawAXValue{Type: "number", Value: json.RawMessage(`42`)}
	if got := v.String(); got != "42" {
		t.Errorf("expected 42, got %q", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
