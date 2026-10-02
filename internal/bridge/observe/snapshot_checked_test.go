package observe

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// checkedFixtureHTML covers every case the field has to answer for: a checked and
// an unchecked native box, a native INDETERMINATE box (which reports mixed with no
// aria attribute at all), a radio group where one option is on, a custom
// role=checkbox and a menuitemcheckbox, and two controls — a textbox and a button
// — that must not gain the field.
const checkedFixtureHTML = `<body>
<input id="name" type="text" value="Alice">
<input id="on" type="checkbox" checked>
<input id="off" type="checkbox">
<input id="ind" type="checkbox">
<input id="r1" type="radio" name="group" checked>
<input id="r2" type="radio" name="group">
<div id="custom" role="checkbox" aria-checked="mixed" tabindex="0">Custom</div>
<div id="menu" role="menuitemcheckbox" aria-checked="true" tabindex="0">Menu item</div>
<button id="go">Go</button>
<script>document.getElementById('ind').indeterminate = true;</script>
</body>`

func newCheckedFixture(t *testing.T) context.Context {
	t.Helper()
	return fixtureTab(t, checkedFixtureHTML, "#go")
}

func snapshotNodes(t *testing.T, ctx context.Context) []A11yNode {
	t.Helper()
	raw, err := FetchAXTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := BuildSnapshot(raw, "", -1)
	if len(nodes) == 0 {
		t.Fatal("snapshot produced no nodes")
	}
	return nodes
}

func nodesByRole(nodes []A11yNode, role string) []A11yNode {
	var out []A11yNode
	for _, n := range nodes {
		if n.Role == role {
			out = append(out, n)
		}
	}
	return out
}

// The reported defect: a checked and an unchecked control were byte-identical in
// the snapshot apart from their identity, so an agent could not read a radio group
// or verify its own check without one extra call per element.
func TestSnapshotReportsCheckedStateForCheckableControls(t *testing.T) {
	nodes := snapshotNodes(t, newCheckedFixture(t))

	checkboxes := nodesByRole(nodes, "checkbox")
	if len(checkboxes) != 4 {
		t.Fatalf("found %d checkbox nodes, want 4 (on, off, indeterminate, custom)", len(checkboxes))
	}
	states := map[CheckedState]int{}
	for _, n := range checkboxes {
		states[n.Checked]++
	}
	if states[CheckedTrue] != 1 || states[CheckedFalse] != 1 || states[CheckedMixed] != 2 {
		t.Errorf("checkbox states = %v, want one true, one false and two mixed (native indeterminate + aria-checked=mixed)", states)
	}

	radios := nodesByRole(nodes, "radio")
	if len(radios) != 2 {
		t.Fatalf("found %d radio nodes, want 2", len(radios))
	}
	// The whole point of the card: which option is selected is readable from ONE
	// snapshot, with no follow-up call per option.
	selected := 0
	for _, n := range radios {
		switch n.Checked {
		case CheckedTrue:
			selected++
		case CheckedFalse:
		default:
			t.Errorf("radio %s reports checked=%q; a radio always has a checkedness", n.Ref, n.Checked)
		}
	}
	if selected != 1 {
		t.Errorf("%d radios report checked=true, want exactly 1", selected)
	}

	menu := nodesByRole(nodes, "menuitemcheckbox")
	if len(menu) != 1 || menu[0].Checked != CheckedTrue {
		t.Errorf("menuitemcheckbox nodes = %+v, want one reporting checked=true", menu)
	}
}

// Absent must mean "this node has no checkedness", never "unchecked". A node that
// gained checked=false here would be a positive false statement about the page.
func TestNodesWithoutCheckednessDoNotGainTheField(t *testing.T) {
	nodes := snapshotNodes(t, newCheckedFixture(t))

	checkableRoles := map[string]bool{"checkbox": true, "radio": true, "menuitemcheckbox": true, "menuitemradio": true}
	for _, n := range nodes {
		if checkableRoles[n.Role] {
			continue
		}
		if n.Checked != "" {
			t.Errorf("%s node %s reports checked=%q; checkedness is meaningless for it", n.Role, n.Ref, n.Checked)
		}
	}

	// Named explicitly, because these two are the controls a wrong role list would
	// most likely sweep in.
	for _, role := range []string{"textbox", "button"} {
		found := nodesByRole(nodes, role)
		if len(found) == 0 {
			t.Fatalf("fixture produced no %s node, so this guard checks nothing", role)
		}
		for _, n := range found {
			if n.Checked != "" {
				t.Errorf("%s reports checked=%q, want the field absent", role, n.Checked)
			}
		}
	}
}

// The round trip the card asked for: the observation surface must change when the
// state does, in both directions, for the same ref.
func TestSnapshotFollowsCheckAndUncheckOnTheSameRef(t *testing.T) {
	ctx := newCheckedFixture(t)

	// Re-read the SAME ref each time: the card asks for the observation surface to
	// follow the state, not for two readings of two different nodes.
	stateOf := func(t *testing.T, ref string) CheckedState {
		t.Helper()
		for _, n := range nodesByRole(snapshotNodes(t, ctx), "checkbox") {
			if n.Ref == ref {
				return n.Checked
			}
		}
		t.Fatalf("checkbox %s is no longer in the snapshot", ref)
		return ""
	}

	// Find the ref of the initially-unchecked native box.
	var ref string
	for _, n := range nodesByRole(snapshotNodes(t, ctx), "checkbox") {
		if n.Checked == CheckedFalse {
			ref = n.Ref
			break
		}
	}
	if ref == "" {
		t.Fatal("fixture has no unchecked checkbox to toggle")
	}

	if err := chromedp.Run(ctx, chromedp.Click("#off", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	if state := stateOf(t, ref); state != CheckedTrue {
		t.Fatalf("after check, %s reports checked=%q, want true", ref, state)
	}

	if err := chromedp.Run(ctx, chromedp.Click("#off", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	if state := stateOf(t, ref); state != CheckedFalse {
		t.Fatalf("after uncheck, %s reports checked=%q, want false", ref, state)
	}
}

// A snapshot diff drives the change markers, so a check that does not register as
// a CHANGE leaves an agent watching diffs blind to exactly the event it acted on.
func TestSnapshotDiffTreatsACheckedChangeAsAChange(t *testing.T) {
	before := []A11yNode{{Ref: "e1", Role: "checkbox", NodeID: 7, Checked: CheckedFalse}}
	after := []A11yNode{{Ref: "e1", Role: "checkbox", NodeID: 7, Checked: CheckedTrue}}

	added, changed, removed := DiffSnapshot(before, after)
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("added %v removed %v, want neither for the same node", added, removed)
	}
	if len(changed) != 1 || changed[0].Ref != "e1" {
		t.Fatalf("changed = %v, want the checkbox whose state moved", changed)
	}
}

// The three states must be distinguishable in the rendered forms too, and a node
// WITHOUT checkedness must not look like an unchecked one.
func TestRenderedFormatsDistinguishAllThreeCheckedStates(t *testing.T) {
	nodes := []A11yNode{
		{Ref: "e1", Role: "checkbox", Checked: CheckedTrue},
		{Ref: "e2", Role: "checkbox", Checked: CheckedFalse},
		{Ref: "e3", Role: "checkbox", Checked: CheckedMixed},
		{Ref: "e4", Role: "button", Name: "Go"},
	}

	text := FormatSnapshotText(nodes)
	for _, want := range []string{"e1 checkbox [checked]", "e2 checkbox [unchecked]", "e3 checkbox [mixed]"} {
		if !strings.Contains(text, want) {
			t.Errorf("text output missing %q:\n%s", want, text)
		}
	}
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "e4 ") && strings.Contains(line, "check") {
			t.Errorf("a node without checkedness carries a checked annotation: %q", line)
		}
	}

	compact := FormatSnapshotCompact(nodes)
	for _, want := range []string{"e1:checkbox [x]", "e2:checkbox [ ]", "e3:checkbox [/]"} {
		if !strings.Contains(compact, want) {
			t.Errorf("compact output missing %q:\n%s", want, compact)
		}
	}

	// [~] is the compact diff's "changed" marker, so mixed must not reuse it: one
	// token meaning two things on the same line cannot be read.
	if strings.Contains(compact, "[~]") {
		t.Errorf("compact checked annotation collides with the diff's changed marker:\n%s", compact)
	}
}

// Absent is not a state, so an unrecognised or empty accessibility reading must
// leave the field off rather than defaulting to unchecked.
func TestUnrecognisedCheckedReadingIsNotReportedAsUnchecked(t *testing.T) {
	for _, raw := range []string{"", "undefined", "TRUE", "yes", "0"} {
		if state, ok := checkedStateFromAX(raw); ok {
			t.Errorf("checkedStateFromAX(%q) = %q, accepted; only true/false/mixed are answers", raw, state)
		}
	}
	for raw, want := range map[string]CheckedState{"true": CheckedTrue, "false": CheckedFalse, "mixed": CheckedMixed} {
		state, ok := checkedStateFromAX(raw)
		if !ok || state != want {
			t.Errorf("checkedStateFromAX(%q) = %q,%v want %q,true", raw, state, ok, want)
		}
	}

	// On the wire, absent means absent: the key must not appear at all.
	encoded, err := json.Marshal(A11yNode{Ref: "e1", Role: "button"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "checked") {
		t.Errorf("a node with no checkedness serialises a checked key: %s", encoded)
	}
	encoded, err = json.Marshal(A11yNode{Ref: "e2", Role: "checkbox", Checked: CheckedFalse})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"checked":"false"`) {
		t.Errorf("a measured false must reach the wire: %s", encoded)
	}
}

// checkboxNodeWithAXChecked builds the one shape a real browser cannot produce: a
// checkable control whose accessibility "checked" property carries something outside
// the three defined states. BuildSnapshot is a pure function over []RawAXNode, which
// is the only way to feed it such a reading.
func checkboxNodeWithAXChecked(axChecked string) []RawAXNode {
	raw, _ := json.Marshal(axChecked)
	return []RawAXNode{
		{
			NodeID:   "root",
			Role:     &RawAXValue{Value: json.RawMessage(`"WebArea"`)},
			Name:     &RawAXValue{Value: json.RawMessage(`"Form"`)},
			ChildIDs: []string{"box"},
		},
		{
			NodeID:           "box",
			Role:             &RawAXValue{Value: json.RawMessage(`"checkbox"`)},
			Name:             &RawAXValue{Value: json.RawMessage(`"Terms"`)},
			BackendDOMNodeID: 11,
			Properties: []RawAXProp{
				{Name: "checked", Value: &RawAXValue{Value: raw}},
			},
		},
	}
}

// checkedStateFromAX being correct is not the same as BuildSnapshot USING it, and only
// this shape can tell the two apart: the browser-backed tests can only produce what
// Chrome emits — true, false and mixed — so an unrecognised reading is unreachable from
// them by construction, and a helper-level test cannot show the helper is wired.
//
// Assigning the raw property value at the call site passes every other test in this
// package while putting checked:"undefined" on the wire, which the three-value contract
// in docs/reference/snapshot.md forbids and which every renderer then annotates as
// nothing at all — indistinguishable from a node the field does not apply to.
func TestBuildSnapshotRoutesCheckednessThroughTheParserSoAnUnrecognisedReadingIsDropped(t *testing.T) {
	for _, axChecked := range []string{"undefined", "", "unknown", "TRUE", "1"} {
		t.Run("ax checked="+axChecked, func(t *testing.T) {
			flat, _ := BuildSnapshot(checkboxNodeWithAXChecked(axChecked), "", -1)
			if len(flat) != 2 {
				t.Fatalf("expected 2 nodes, got %d", len(flat))
			}

			if got := flat[1].Checked; got != "" {
				t.Errorf("Checked = %q for an accessibility reading of %q; only true/false/mixed are answers, and anything else must leave the field ABSENT rather than put an undefined state on the wire", got, axChecked)
			}
			encoded, err := json.Marshal(flat[1])
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), `"checked"`) {
				t.Errorf("node serialises a checked key for an unrecognised reading: %s", encoded)
			}
		})
	}
}

// The positive half, on the same synthetic path: the three defined states must survive
// it. Without this, dropping every reading — the crudest possible bypass — would satisfy
// the test above.
func TestBuildSnapshotKeepsTheThreeDefinedCheckedStatesFromTheAXProperty(t *testing.T) {
	for _, want := range []CheckedState{CheckedTrue, CheckedFalse, CheckedMixed} {
		t.Run(string(want), func(t *testing.T) {
			flat, _ := BuildSnapshot(checkboxNodeWithAXChecked(string(want)), "", -1)
			if len(flat) != 2 {
				t.Fatalf("expected 2 nodes, got %d", len(flat))
			}
			if got := flat[1].Checked; got != want {
				t.Errorf("Checked = %q, want %q", got, want)
			}
		})
	}
}
