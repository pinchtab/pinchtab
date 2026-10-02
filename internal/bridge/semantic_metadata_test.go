package bridge

import (
	"context"
	"strings"
	"testing"

	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
)

func TestApplyNodeDOMMetadataRedactsPassword(t *testing.T) {
	node := A11yNode{
		Role:  "textbox",
		Name:  "Password",
		Value: "supersecret123",
	}
	meta := nodeDOMMetadata{
		Tag:       "input",
		InputType: "password",
		HasValue:  true,
	}
	applyNodeDOMMetadata(&node, meta)

	if node.Value != bridgeobserve.MaskedValue {
		t.Errorf("password value = %q, want redacted", node.Value)
	}
	if node.Tag != "input" {
		t.Errorf("tag = %q, want input", node.Tag)
	}
}

func TestApplyNodeDOMMetadataPreservesTextInput(t *testing.T) {
	node := A11yNode{
		Role:  "textbox",
		Name:  "Username",
		Value: "mario",
	}
	meta := nodeDOMMetadata{
		Tag:       "input",
		InputType: "text",
	}
	applyNodeDOMMetadata(&node, meta)

	if node.Value != "mario" {
		t.Errorf("text value = %q, want mario", node.Value)
	}
}

func TestApplyNodeDOMMetadataRedactsPasswordCaseInsensitive(t *testing.T) {
	node := A11yNode{
		Role:  "textbox",
		Name:  "Password",
		Value: "secret",
	}
	meta := nodeDOMMetadata{
		Tag:       "input",
		InputType: "Password",
		HasValue:  true,
	}
	applyNodeDOMMetadata(&node, meta)

	if node.Value != bridgeobserve.MaskedValue {
		t.Errorf("password value = %q, want redacted", node.Value)
	}
}

func TestApplyNodeDOMMetadataNoInputType(t *testing.T) {
	node := A11yNode{
		Role:  "textbox",
		Name:  "Search",
		Value: "query",
	}
	meta := nodeDOMMetadata{
		Tag: "input",
	}
	applyNodeDOMMetadata(&node, meta)

	if node.Value != "query" {
		t.Errorf("value = %q, want query", node.Value)
	}
}

// An empty password field must read as empty. Reporting a mask on it tells a
// form-filling agent the field is already filled, and the flow stalls.
func TestApplyNodeDOMMetadataEmptyPasswordRendersEmpty(t *testing.T) {
	empty := A11yNode{Role: "textbox", Name: "Password"}
	applyNodeDOMMetadata(&empty, nodeDOMMetadata{Tag: "input", InputType: "password", HasValue: false})

	filled := A11yNode{Role: "textbox", Name: "Password"}
	applyNodeDOMMetadata(&filled, nodeDOMMetadata{Tag: "input", InputType: "password", HasValue: true})

	if empty.Value != "" {
		t.Errorf("empty password = %q, want no value", empty.Value)
	}
	if filled.Value != bridgeobserve.MaskedValue {
		t.Errorf("filled password = %q, want %q", filled.Value, bridgeobserve.MaskedValue)
	}
	if empty.Value == filled.Value {
		t.Fatal("empty and filled password fields render identically — the snapshot cannot answer whether the field needs filling")
	}
}

// A pre-existing accessibility value must not survive on an empty sensitive
// field either: the a11y pass may have masked it before enrichment ran.
func TestApplyNodeDOMMetadataEmptyPasswordClearsExistingMask(t *testing.T) {
	node := A11yNode{Role: "textbox", Name: "Password", Value: bridgeobserve.MaskedValue}
	applyNodeDOMMetadata(&node, nodeDOMMetadata{Tag: "input", InputType: "password", HasValue: false})

	if node.Value != "" {
		t.Errorf("empty password = %q, want no value", node.Value)
	}
}

// The sensitivity rule is the union of the two signals, so a field that trips
// only the autocomplete one is masked at this site too.
func TestApplyNodeDOMMetadataMasksEitherPasswordSignal(t *testing.T) {
	tests := []struct {
		name string
		meta nodeDOMMetadata
	}{
		{name: "input type only", meta: nodeDOMMetadata{Tag: "input", InputType: "password", Autocomplete: "off", HasValue: true}},
		{name: "autocomplete current-password only", meta: nodeDOMMetadata{Tag: "input", InputType: "text", Autocomplete: "current-password", HasValue: true}},
		{name: "autocomplete new-password only", meta: nodeDOMMetadata{Tag: "input", InputType: "text", Autocomplete: "New-Password", HasValue: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := A11yNode{Role: "textbox", Name: "Secret", Value: "supersecret123"}
			applyNodeDOMMetadata(&node, tt.meta)

			if node.Value != bridgeobserve.MaskedValue {
				t.Fatalf("value = %q, want %q", node.Value, bridgeobserve.MaskedValue)
			}
		})
	}
}

// The field's own content must never be requested from the page: the DOM script
// returns an emptiness boolean, so there is no value to leak into Go.
func TestDOMMetadataFnReturnsEmptinessNotTheValue(t *testing.T) {
	if !strings.Contains(domMetadataFn, "hasValue: !!el.value") {
		t.Fatal("domMetadataFn does not return the hasValue boolean")
	}
	if strings.Contains(domMetadataFn, "value: el.value") || strings.Contains(domMetadataFn, "value: String(el.value") {
		t.Fatal("domMetadataFn returns the field value itself")
	}
}

// Enrichment is best-effort. When it cannot run, whatever the a11y pass masked
// must stay masked — nothing here may fall back to printing the value.
func TestEnrichA11yNodesLeavesMaskWhenEnrichmentCannotRun(t *testing.T) {
	nodes := []A11yNode{{
		Role:   "textbox",
		Name:   "Password",
		Value:  bridgeobserve.MaskedValue,
		NodeID: 42,
	}}

	_ = EnrichA11yNodesWithDOMMetadata(context.Background(), nodes)

	if nodes[0].Value != bridgeobserve.MaskedValue {
		t.Fatalf("value = %q, want the mask to survive a failed enrichment", nodes[0].Value)
	}
}

func TestCarryDOMMetadataReusesOnlyUnchangedNonEditableNodes(t *testing.T) {
	prev := []A11yNode{
		{NodeID: 1, Role: "button", Name: "Details", Tag: "button", Text: "Details"},
		{NodeID: 2, Role: "button", Name: "Save", Tag: "button", Text: "Save"},
		{NodeID: 3, Role: "textbox", Name: "Password", Tag: "input"},
		{NodeID: 4, Role: "link", Name: "Help"},
	}
	next := []A11yNode{
		{NodeID: 1, Role: "button", Name: "Details"},
		{NodeID: 2, Role: "button", Name: "Saved"},
		{NodeID: 3, Role: "textbox", Name: "Password"},
		{NodeID: 4, Role: "link", Name: "Help"},
		{NodeID: 5, Role: "button", Name: "Close"},
	}

	CarryDOMMetadata(prev, next)

	if next[0].Tag != "button" || next[0].Text != "Details" {
		t.Errorf("unchanged button did not carry its metadata: %+v", next[0])
	}
	for _, node := range next[1:] {
		if hasNodeDOMMetadata(node) {
			t.Errorf("node %d (%s %q) carried metadata it must re-read: %+v", node.NodeID, node.Role, node.Name, node)
		}
	}
}
