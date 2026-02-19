package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestFormatSnapshotCompact_Basic(t *testing.T) {
	nodes := []A11yNode{
		{Ref: "e0", Role: "button", Name: "Submit"},
		{Ref: "e1", Role: "textbox", Name: "Email", Value: "test@example.com"},
	}
	got := formatSnapshotCompact(nodes)
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
	got := formatSnapshotCompact(nodes)
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
	got := formatSnapshotCompact(nil)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFormatSnapshotCompact_NoName(t *testing.T) {
	nodes := []A11yNode{{Ref: "e0", Role: "generic"}}
	got := formatSnapshotCompact(nodes)
	if got != "e0:generic\n" {
		t.Errorf("got %q", got)
	}
}

func TestTruncateToTokens_NoTruncation(t *testing.T) {
	nodes := []A11yNode{
		{Ref: "e0", Role: "button", Name: "OK"},
		{Ref: "e1", Role: "link", Name: "Home"},
	}
	result, truncated := truncateToTokens(nodes, 1000, "compact")
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
	result, truncated := truncateToTokens(nodes, 50, "compact")
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

	jsonResult, _ := truncateToTokens(nodes, 100, "json")
	compactResult, _ := truncateToTokens(nodes, 100, "compact")
	if len(jsonResult) > len(compactResult) {
		t.Errorf("JSON should truncate sooner: json=%d compact=%d", len(jsonResult), len(compactResult))
	}
}

func TestTruncateToTokens_Empty(t *testing.T) {
	result, truncated := truncateToTokens(nil, 100, "compact")
	if truncated {
		t.Error("expected no truncation on empty")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result))
	}
}

func TestFilterSubtree_Found(t *testing.T) {
	nodes := []rawAXNode{
		{NodeID: "root", BackendDOMNodeID: 1, ChildIDs: []string{"child1", "child2"}},
		{NodeID: "child1", BackendDOMNodeID: 2, ChildIDs: []string{"grandchild"}},
		{NodeID: "child2", BackendDOMNodeID: 3},
		{NodeID: "grandchild", BackendDOMNodeID: 4},
		{NodeID: "other", BackendDOMNodeID: 5},
	}

	result := filterSubtree(nodes, 2)
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
	nodes := []rawAXNode{
		{NodeID: "root", BackendDOMNodeID: 1},
		{NodeID: "child", BackendDOMNodeID: 2},
	}

	result := filterSubtree(nodes, 999)
	if len(result) != 2 {
		t.Errorf("expected all nodes returned, got %d", len(result))
	}
}

func TestFilterSubtree_RootScope(t *testing.T) {
	nodes := []rawAXNode{
		{NodeID: "root", BackendDOMNodeID: 1, ChildIDs: []string{"child"}},
		{NodeID: "child", BackendDOMNodeID: 2, ChildIDs: []string{"grandchild"}},
		{NodeID: "grandchild", BackendDOMNodeID: 3},
	}

	result := filterSubtree(nodes, 1)
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
	added, changed, removed := diffSnapshot(prev, curr)
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
	_, _, removed := diffSnapshot(prev, curr)
	if len(removed) != 1 || removed[0].Name != "Old" {
		t.Errorf("expected 1 removed node, got %v", removed)
	}
}

func TestDiffSnapshot_Changed(t *testing.T) {
	prev := []A11yNode{{Ref: "e0", Role: "textbox", Name: "Email", NodeID: 1, Value: "old"}}
	curr := []A11yNode{{Ref: "e0", Role: "textbox", Name: "Email", NodeID: 1, Value: "new"}}
	_, changed, _ := diffSnapshot(prev, curr)
	if len(changed) != 1 || changed[0].Value != "new" {
		t.Errorf("expected 1 changed node, got %v", changed)
	}
}

func TestDiffSnapshot_Empty(t *testing.T) {
	added, changed, removed := diffSnapshot(nil, nil)
	if len(added)+len(changed)+len(removed) != 0 {
		t.Error("expected all empty for nil inputs")
	}
}

func TestRawAXValue_String_Normal(t *testing.T) {
	v := &rawAXValue{Type: "string", Value: json.RawMessage(`"hello"`)}
	if got := v.String(); got != "hello" {
		t.Errorf("expected hello, got %q", got)
	}
}

func TestRawAXValue_String_Nil(t *testing.T) {
	var v *rawAXValue
	if got := v.String(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRawAXValue_String_NilValue(t *testing.T) {
	v := &rawAXValue{Type: "string"}
	if got := v.String(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRawAXValue_String_NonString(t *testing.T) {
	v := &rawAXValue{Type: "number", Value: json.RawMessage(`42`)}

	if got := v.String(); got != "42" {
		t.Errorf("expected 42, got %q", got)
	}
}

func TestHandleSnapshot_InvalidFilter(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	req := httptest.NewRequest("GET", "/snapshot?filter=bogus", nil)
	w := httptest.NewRecorder()
	b.handleSnapshot(w, req)
	if w.Code == 0 {
		t.Error("expected a response code")
	}
}

func TestHandleScreenshot_RawParam(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	req := httptest.NewRequest("GET", "/screenshot?raw=true&tabId=nonexistent", nil)
	w := httptest.NewRecorder()
	b.handleScreenshot(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleNavigate_WithTimeout(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	body := `{"url":"https://example.com","timeout":5,"tabId":"nonexistent"}`
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleNavigate_WithBlockImages(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	body := `{"url":"https://example.com","blockImages":true,"tabId":"nonexistent"}`
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleNavigate_WaitTitle(t *testing.T) {
	b := &Bridge{}
	b.TabManager = &TabManager{tabs: make(map[string]*TabEntry), snapshots: make(map[string]*refCache)}
	body := `{"url":"https://example.com","waitTitle":true,"tabId":"nonexistent"}`
	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	b.handleNavigate(w, req)
	if w.Code != 404 && w.Code != 400 {
		t.Errorf("expected 404 or 400, got %d", w.Code)
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
