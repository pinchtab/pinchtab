package engine

import (
	"encoding/json"
	"testing"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
)

func TestBuildLPSnapshot_FiltersIgnored(t *testing.T) {
	tab := &lpTab{refMap: make(map[string]int64)}
	nodes := []*accessibility.Node{
		makeAXNode("1", "button", "Submit", false, 10),
		makeAXNode("2", "heading", "Title", true, 0), // ignored
		makeAXNode("3", "link", "Home", false, 20),
	}

	result := buildLPSnapshot(tab, nodes, "")
	if len(result) != 2 {
		t.Fatalf("expected 2 nodes (1 ignored), got %d", len(result))
	}
	if result[0].Role != "button" {
		t.Errorf("first node role = %q, want button", result[0].Role)
	}
	if result[1].Role != "link" {
		t.Errorf("second node role = %q, want link", result[1].Role)
	}
}

func TestBuildLPSnapshot_SkipsNonSemantic(t *testing.T) {
	tab := &lpTab{refMap: make(map[string]int64)}
	nodes := []*accessibility.Node{
		makeAXNode("1", "RootWebArea", "", false, 0),
		makeAXNode("2", "GenericContainer", "", false, 0),
		makeAXNode("3", "none", "", false, 0),
		makeAXNode("4", "button", "OK", false, 30),
	}

	result := buildLPSnapshot(tab, nodes, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 semantic node, got %d", len(result))
	}
	if result[0].Name != "OK" {
		t.Errorf("node name = %q, want OK", result[0].Name)
	}
}

func TestBuildLPSnapshot_InteractiveFilter(t *testing.T) {
	tab := &lpTab{refMap: make(map[string]int64)}
	nodes := []*accessibility.Node{
		makeAXNode("1", "heading", "Title", false, 0),
		makeAXNode("2", "button", "Submit", false, 10),
		makeAXNode("3", "textbox", "Email", false, 20),
		makeAXNode("4", "article", "Content", false, 0),
	}

	result := buildLPSnapshot(tab, nodes, "interactive")
	if len(result) != 2 {
		t.Fatalf("expected 2 interactive nodes, got %d", len(result))
	}
	for _, n := range result {
		if !n.Interactive {
			t.Errorf("non-interactive node in filtered result: %+v", n)
		}
	}
}

func TestBuildLPSnapshot_RefMap(t *testing.T) {
	tab := &lpTab{refMap: make(map[string]int64)}
	nodes := []*accessibility.Node{
		makeAXNode("1", "button", "OK", false, 42),
		makeAXNode("2", "link", "Home", false, 99),
	}

	result := buildLPSnapshot(tab, nodes, "")
	if len(result) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result))
	}

	// Check refs are sequential.
	if result[0].Ref != "e0" {
		t.Errorf("first ref = %q, want e0", result[0].Ref)
	}
	if result[1].Ref != "e1" {
		t.Errorf("second ref = %q, want e1", result[1].Ref)
	}

	// Check backend node IDs in refMap.
	if id, ok := tab.refMap["e0"]; !ok || id != 42 {
		t.Errorf("refMap[e0] = %d, want 42", id)
	}
	if id, ok := tab.refMap["e1"]; !ok || id != 99 {
		t.Errorf("refMap[e1] = %d, want 99", id)
	}
}

func TestAxValueStr(t *testing.T) {
	tests := []struct {
		name string
		v    *accessibility.Value
		want string
	}{
		{"nil", nil, ""},
		{"string value", makeAXValue("hello"), "hello"},
		{"empty string", makeAXValue(""), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := axValueStr(tt.v)
			if got != tt.want {
				t.Errorf("axValueStr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsInteractiveRole(t *testing.T) {
	interactive := []string{"button", "link", "textbox", "checkbox", "radio", "combobox", "menuitem", "tab", "switch"}
	for _, role := range interactive {
		if !isInteractiveRole(role) {
			t.Errorf("isInteractiveRole(%q) = false, want true", role)
		}
	}

	nonInteractive := []string{"heading", "article", "navigation", "generic", "main", "region", "list"}
	for _, role := range nonInteractive {
		if isInteractiveRole(role) {
			t.Errorf("isInteractiveRole(%q) = true, want false", role)
		}
	}
}

func TestQuadCenter(t *testing.T) {
	// Rectangle: (10,20) (30,20) (30,40) (10,40) → center (20, 30)
	q := []float64{10, 20, 30, 20, 30, 40, 10, 40}
	x, y := quadCenter(q)
	if x != 20 || y != 30 {
		t.Errorf("quadCenter = (%v, %v), want (20, 30)", x, y)
	}
}

func TestQuadCenter_Short(t *testing.T) {
	x, y := quadCenter([]float64{1, 2})
	if x != 0 || y != 0 {
		t.Errorf("quadCenter with short quad = (%v, %v), want (0, 0)", x, y)
	}
}

func TestLightpandaEngine_Name(t *testing.T) {
	// NewLightpandaEngine requires a URL but we can test Name() without
	// actually connecting.
	lp, err := NewLightpandaEngine("ws://127.0.0.1:19222")
	if err != nil {
		t.Fatalf("NewLightpandaEngine: %v", err)
	}
	defer func() { _ = lp.Close() }()

	if lp.Name() != "lightpanda" {
		t.Errorf("Name() = %q, want %q", lp.Name(), "lightpanda")
	}
}

func TestLightpandaEngine_Capabilities(t *testing.T) {
	lp, err := NewLightpandaEngine("ws://127.0.0.1:19222")
	if err != nil {
		t.Fatalf("NewLightpandaEngine: %v", err)
	}
	defer func() { _ = lp.Close() }()

	caps := lp.Capabilities()
	if len(caps) != 6 {
		t.Errorf("expected 6 capabilities, got %d: %v", len(caps), caps)
	}

	capSet := make(map[Capability]bool)
	for _, c := range caps {
		capSet[c] = true
	}
	for _, want := range []Capability{CapNavigate, CapSnapshot, CapText, CapClick, CapType, CapEvaluate} {
		if !capSet[want] {
			t.Errorf("missing capability %q", want)
		}
	}
}

func TestLightpandaEngine_RequiresURL(t *testing.T) {
	_, err := NewLightpandaEngine("")
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestLightpandaEngine_SnapshotWithoutNavigate(t *testing.T) {
	lp, err := NewLightpandaEngine("ws://127.0.0.1:19222")
	if err != nil {
		t.Fatalf("NewLightpandaEngine: %v", err)
	}
	defer func() { _ = lp.Close() }()

	_, err = lp.Snapshot(nil, "")
	if err == nil {
		t.Error("expected error for snapshot without navigate")
	}
}

func TestLightpandaEngine_ClickWithoutNavigate(t *testing.T) {
	lp, err := NewLightpandaEngine("ws://127.0.0.1:19222")
	if err != nil {
		t.Fatalf("NewLightpandaEngine: %v", err)
	}
	defer func() { _ = lp.Close() }()

	err = lp.Click(nil, "e0")
	if err == nil {
		t.Error("expected error for click without navigate")
	}
}

func TestLightpandaEngine_TextWithoutNavigate(t *testing.T) {
	lp, err := NewLightpandaEngine("ws://127.0.0.1:19222")
	if err != nil {
		t.Fatalf("NewLightpandaEngine: %v", err)
	}
	defer func() { _ = lp.Close() }()

	_, err = lp.Text(nil)
	if err == nil {
		t.Error("expected error for text without navigate")
	}
}

// --- Router tests for Lightpanda mode ---

func TestRouterLightpandaMode(t *testing.T) {
	r := NewRouter(ModeLightpanda, nil)
	lp := &fakeEngine{name: "lightpanda"}
	r.RegisterEngine(lp)

	// DOM operations → lightpanda
	for _, op := range []Capability{CapNavigate, CapSnapshot, CapText, CapClick, CapType, CapEvaluate} {
		eng := r.Route(op, "https://example.com")
		if eng == nil {
			t.Errorf("lightpanda mode should route %s to lightpanda, got chrome", op)
			continue
		}
		if eng.Name() != "lightpanda" {
			t.Errorf("lightpanda mode route(%s) = %q, want lightpanda", op, eng.Name())
		}
	}

	// Chrome-only operations → chrome
	for _, op := range []Capability{CapScreenshot, CapPDF, CapCookies} {
		eng := r.Route(op, "https://example.com")
		if eng != nil {
			t.Errorf("lightpanda mode should route %s to chrome, got %q", op, eng.Name())
		}
	}
}

func TestRouterLightpandaModeNoEngine(t *testing.T) {
	r := NewRouter(ModeLightpanda, nil)
	// No engine registered — should fall through to chrome.
	eng := r.Route(CapNavigate, "https://example.com")
	if eng != nil {
		t.Errorf("should fall through to chrome when lightpanda not registered, got %q", eng.Name())
	}
}

func TestLightpandaCapabilityRule(t *testing.T) {
	rule := LightpandaCapabilityRule{}
	if rule.Name() != "lightpanda-capability" {
		t.Errorf("Name() = %q, want lightpanda-capability", rule.Name())
	}

	// Chrome-only operations.
	for _, op := range []Capability{CapScreenshot, CapPDF, CapCookies} {
		if d := rule.Decide(op, ""); d != UseChrome {
			t.Errorf("Decide(%s) = %d, want UseChrome", op, d)
		}
	}

	// Other operations → undecided.
	for _, op := range []Capability{CapNavigate, CapSnapshot, CapText, CapClick, CapType, CapEvaluate} {
		if d := rule.Decide(op, ""); d != Undecided {
			t.Errorf("Decide(%s) = %d, want Undecided", op, d)
		}
	}
}

func TestDefaultLightpandaRule(t *testing.T) {
	rule := DefaultLightpandaRule{}
	if rule.Name() != "default-lightpanda" {
		t.Errorf("Name() = %q, want default-lightpanda", rule.Name())
	}

	for _, op := range []Capability{CapNavigate, CapSnapshot, CapText, CapClick, CapType, CapEvaluate} {
		if d := rule.Decide(op, ""); d != UseLightpanda {
			t.Errorf("Decide(%s) = %d, want UseLightpanda", op, d)
		}
	}

	// Unknown capability → undecided.
	if d := rule.Decide("unknown", ""); d != Undecided {
		t.Errorf("Decide(unknown) = %d, want Undecided", d)
	}
}

// --- helpers ---

func makeAXNode(id, role, name string, ignored bool, backendID int64) *accessibility.Node {
	n := &accessibility.Node{
		NodeID:  accessibility.NodeID(id),
		Ignored: ignored,
	}
	if role != "" {
		n.Role = makeAXValue(role)
	}
	if name != "" {
		n.Name = makeAXValue(name)
	}
	if backendID > 0 {
		n.BackendDOMNodeID = cdp.BackendNodeID(backendID)
	}
	return n
}

func makeAXValue(s string) *accessibility.Value {
	raw, _ := json.Marshal(s)
	return &accessibility.Value{
		Type:  "string",
		Value: raw,
	}
}
