package observe

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const hydratedListFixtureHTML = `<body>
<style>.row { content-visibility: auto; contain-intrinsic-size: auto 64px; }</style>
<h2 id="list-title">Search results</h2>
<ul id="issues" role="list" aria-labelledby="list-title"></ul>
<script>
for (const title of ["Hydrated issue alpha", "Hydrated issue beta", "Hydrated issue gamma"]) {
  const row = document.createElement("div");
  row.className = "row";
  row.innerHTML = '<li role="listitem"><h3><a href="#">' + title + '</a></h3></li>';
  document.getElementById("issues").appendChild(row);
}
</script>
</body>`

func newBackgroundTabFixture(t *testing.T, html string) context.Context {
	t.Helper()
	foregroundCtx := deadlineTab(t)

	var targetID target.ID
	if err := chromedp.Run(foregroundCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		targetID, err = target.CreateTarget("about:blank").WithFocus(false).Do(ctx)
		return err
	})); err != nil {
		t.Fatal(err)
	}
	tabCtx, cancelTab := chromedp.NewContext(foregroundCtx, chromedp.WithTargetID(targetID))
	t.Cleanup(cancelTab)

	var visibility string
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(htmlDataURL(html)),
		chromedp.Evaluate(`document.visibilityState`, &visibility),
	); err != nil {
		t.Fatal(err)
	}
	if visibility != "hidden" {
		t.Fatalf("fixture tab visibilityState = %q, want hidden: the background-tab precondition no longer holds", visibility)
	}
	return tabCtx
}

func TestSnapshotOfAHiddenTabIncludesContentVisibilityAutoRows(t *testing.T) {
	ctx := newBackgroundTabFixture(t, hydratedListFixtureHTML)

	nodes := snapshotNodes(t, ctx)
	want := map[string]bool{"Hydrated issue alpha": false, "Hydrated issue beta": false, "Hydrated issue gamma": false}
	for _, n := range nodes {
		if n.Role == "link" {
			if _, ok := want[n.Name]; ok {
				want[n.Name] = true
			}
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("snapshot of a background tab is missing link %q that sits in a content-visibility:auto row", name)
		}
	}
}

func rawAXNode(id, role, name string, childIDs ...string) RawAXNode {
	quoted := func(s string) *RawAXValue {
		v, _ := json.Marshal(s)
		return &RawAXValue{Value: v}
	}
	return RawAXNode{NodeID: id, Role: quoted(role), Name: quoted(name), ChildIDs: childIDs}
}

func TestInteractiveSnapshotNamesLiveRegionsFromTheirText(t *testing.T) {
	raw := []RawAXNode{
		rawAXNode("root", "RootWebArea", "Form", "save", "alert", "status", "empty"),
		rawAXNode("save", "button", "Save"),
		rawAXNode("alert", "alert", "", "alert-text"),
		rawAXNode("alert-text", "StaticText", "Enter a valid email address", "alert-inline"),
		rawAXNode("alert-inline", "InlineTextBox", "Enter a valid email address"),
		rawAXNode("status", "status", "", "status-wrap"),
		rawAXNode("status-wrap", "generic", "", "status-count", "status-link"),
		rawAXNode("status-count", "StaticText", " 3 items saved. "),
		rawAXNode("status-link", "link", "Undo", "status-link-text"),
		rawAXNode("status-link-text", "StaticText", "Undo"),
		rawAXNode("empty", "status", ""),
	}

	nodes, _ := BuildSnapshot(raw, FilterInteractive, -1)

	got := make([]string, 0, len(nodes))
	for _, n := range nodes {
		got = append(got, n.Role+" "+strconv.Quote(n.Name))
	}
	want := []string{
		`button "Save"`,
		`alert "Enter a valid email address"`,
		`status "3 items saved. Undo"`,
		`link "Undo"`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("interactive snapshot = %q, want %q", got, want)
	}
}

func usersTableAXNodes() []RawAXNode {
	return []RawAXNode{
		rawAXNode("table", "table", "Users", "header-row", "row"),
		rawAXNode("header-row", "row", "", "col-name", "col-actions"),
		rawAXNode("col-name", "columnheader", "Name", "col-name-text"),
		rawAXNode("col-name-text", "StaticText", "Name"),
		rawAXNode("col-actions", "columnheader", "Actions", "col-actions-text"),
		rawAXNode("col-actions-text", "StaticText", "Actions"),
		rawAXNode("row", "row", "", "select-cell", "name-cell", "owner-cell", "actions-cell", "empty-cell", "hidden-control-cell"),
		rawAXNode("select-cell", "cell", "Select Ann", "select-box"),
		rawAXNode("select-box", "checkbox", "Select Ann"),
		rawAXNode("name-cell", "cell", "Cy Young", "name-link"),
		rawAXNode("name-link", "link", "Cy Young", "name-link-text"),
		rawAXNode("name-link-text", "StaticText", "Cy Young"),
		rawAXNode("owner-cell", "cell", "Ann Edit", "owner-text", "owner-edit"),
		rawAXNode("owner-text", "StaticText", "Ann"),
		rawAXNode("owner-edit", "link", "Edit", "owner-edit-text"),
		rawAXNode("owner-edit-text", "StaticText", "Edit"),
		rawAXNode("actions-cell", "cell", "Edit Delete", "edit", "delete"),
		rawAXNode("edit", "link", "Edit", "edit-text"),
		rawAXNode("edit-text", "StaticText", "Edit"),
		rawAXNode("delete", "button", "Delete", "delete-text"),
		rawAXNode("delete-text", "StaticText", "Delete"),
		rawAXNode("empty-cell", "cell", ""),
		rawAXNode("hidden-control-cell", "cell", "", "archive"),
		hiddenAXNode(rawAXNode("archive", "button", "Archive")),
	}
}

func hiddenAXNode(n RawAXNode) RawAXNode {
	n.Properties = append(n.Properties, RawAXProp{Name: "hidden", Value: &RawAXValue{Value: json.RawMessage("true")}})
	return n
}

func roleNameLines(nodes []A11yNode) []string {
	lines := make([]string, 0, len(nodes))
	for _, n := range nodes {
		lines = append(lines, n.Role+" "+strconv.Quote(n.Name))
	}
	return lines
}

func TestInteractiveSnapshotDropsTableCellsNamedOnlyByTheirControls(t *testing.T) {
	nodes, _ := BuildSnapshot(usersTableAXNodes(), FilterInteractive, -1)

	want := []string{
		`columnheader "Name"`,
		`columnheader "Actions"`,
		`checkbox "Select Ann"`,
		`link "Cy Young"`,
		`cell "Ann Edit"`,
		`link "Edit"`,
		`link "Edit"`,
		`button "Delete"`,
		`cell ""`,
		`cell ""`,
		`button "Archive"`,
	}
	if got := roleNameLines(nodes); !slices.Equal(got, want) {
		t.Fatalf("interactive snapshot = %q, want %q", got, want)
	}
}

func TestFullSnapshotKeepsTableCellsNamedByTheirControls(t *testing.T) {
	nodes, _ := BuildSnapshot(usersTableAXNodes(), "", -1)

	got := roleNameLines(nodes)
	for _, cell := range []string{`cell "Select Ann"`, `cell "Cy Young"`, `cell "Ann Edit"`, `cell "Edit Delete"`, `cell ""`} {
		if !slices.Contains(got, cell) {
			t.Fatalf("full snapshot = %q, missing %s", got, cell)
		}
	}
}

func TestUnfilteredSnapshotLeavesLiveRegionNamesToTheirChildren(t *testing.T) {
	raw := []RawAXNode{
		rawAXNode("alert", "alert", "", "alert-text"),
		rawAXNode("alert-text", "StaticText", "Enter a valid email address"),
	}

	nodes, _ := BuildSnapshot(raw, "", -1)

	if len(nodes) != 2 || nodes[0].Name != "" || nodes[1].Name != "Enter a valid email address" {
		t.Fatalf("unfiltered snapshot = %+v, want an unnamed alert followed by its text", nodes)
	}
}

func TestSnapshotDiffTreatsARenamedNodeAsChanged(t *testing.T) {
	before := []A11yNode{
		{Ref: "e1", Role: "textbox", Name: "Name", NodeID: 5},
		{Ref: "e14", Role: "status", Name: "Thanks Bob", NodeID: 9},
	}
	after := []A11yNode{
		{Ref: "e1", Role: "textbox", Name: "Name", NodeID: 5},
		{Ref: "e14", Role: "status", Name: "Thanks Dee", NodeID: 9},
	}

	added, changed, removed := DiffSnapshot(before, after)
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("added %v removed %v, want neither for the same node", added, removed)
	}
	if len(changed) != 1 || changed[0].Name != "Thanks Dee" {
		t.Fatalf("changed = %v, want the renamed status node", changed)
	}

	rendered := FormatSnapshotCompactDiff(after, added, changed, removed)
	if !strings.Contains(rendered, `"Thanks Dee" [~]`) || strings.Contains(rendered, "# removed:") {
		t.Fatalf("compact diff = %q, want the status line marked [~] and no removed footer", rendered)
	}
}

func TestSnapshotDiffFormatsListOnlyAddedAndChangedNodes(t *testing.T) {
	before := []A11yNode{
		{Ref: "e1", Role: "textbox", Name: "Name", NodeID: 5},
		{Ref: "e2", Role: "status", Name: "Thanks Bob", NodeID: 9},
		{Ref: "e3", Role: "button", Name: "Send", NodeID: 11},
	}
	after := []A11yNode{
		{Ref: "e1", Role: "textbox", Name: "Name", NodeID: 5},
		{Ref: "e2", Role: "status", Name: "Thanks Dee", NodeID: 9},
		{Ref: "e4", Role: "button", Name: "Close", Depth: 1, NodeID: 14},
	}

	added, changed, removed := DiffSnapshot(before, after)
	if got, want := FormatSnapshotTextDiff(after, added, changed, removed), "e2 status \"Thanks Dee\" [~]\n  e4 button \"Close\" [+]\n# removed: e3\n"; got != want {
		t.Errorf("text diff = %q, want %q", got, want)
	}
	if got, want := FormatSnapshotCompactDiff(after, added, changed, removed), "e2:status \"Thanks Dee\" [~]\ne4:button \"Close\" [+]\n# removed: e3\n"; got != want {
		t.Errorf("compact diff = %q, want %q", got, want)
	}
}

func TestSnapshotDiffFormatsAreEmptyWithoutChanges(t *testing.T) {
	nodes := []A11yNode{{Ref: "e1", Role: "textbox", Name: "Name", NodeID: 5}}
	added, changed, removed := DiffSnapshot(nodes, nodes)
	if got := FormatSnapshotCompactDiff(nodes, added, changed, removed); got != "" {
		t.Errorf("compact diff = %q, want empty", got)
	}
	if got := FormatSnapshotTextDiff(nodes, added, changed, removed); got != "" {
		t.Errorf("text diff = %q, want empty", got)
	}
}

func TestSnapshotDiffKeepsNodesWithoutANodeIDApart(t *testing.T) {
	before := []A11yNode{
		{Ref: "e1", Role: "StaticText", Name: "Alpha"},
		{Ref: "e2", Role: "StaticText", Name: "Beta"},
	}
	after := []A11yNode{
		{Ref: "e1", Role: "StaticText", Name: "Alpha"},
		{Ref: "e2", Role: "StaticText", Name: "Gamma"},
	}

	added, changed, removed := DiffSnapshot(before, after)
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want none", changed)
	}
	if len(added) != 1 || added[0].Name != "Gamma" {
		t.Fatalf("added = %v, want Gamma", added)
	}
	if len(removed) != 1 || removed[0].Name != "Beta" {
		t.Fatalf("removed = %v, want Beta", removed)
	}
}

func TestSnapshotDiffKeepsEqualNodeIDsInDifferentFramesApart(t *testing.T) {
	before := []A11yNode{
		{Ref: "e1", Role: "button", Name: "Save", NodeID: 4, FrameID: "main"},
		{Ref: "e2", Role: "link", Name: "Help", NodeID: 4, FrameID: "child"},
	}

	added, changed, removed := DiffSnapshot(before, before)
	if len(added)+len(changed)+len(removed) != 0 {
		t.Fatalf("added %v changed %v removed %v, want an unchanged snapshot to diff empty", added, changed, removed)
	}
}

const dateTimeFixtureHTML = `<form id="dates">
<label>Start date <input type="date" value="2026-10-02"></label>
<label>End date <input type="date"></label>
<label>Opens at <input type="time" value="09:30"></label>
<label>Closes at <input type="time"></label>
<label>Meeting <input type="datetime-local"></label>
<label>Billing month <input type="month"></label>
<label>Sprint week <input type="week"></label>
</form>`

func interactiveSnapshot(t *testing.T, ctx context.Context) []A11yNode {
	t.Helper()
	raw, err := FetchAXTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := BuildSnapshot(raw, FilterInteractive, -1)
	return nodes
}

func TestInteractiveSnapshotListsDateTimeInputsByLabelAndISOValue(t *testing.T) {
	ctx := fixtureTab(t, dateTimeFixtureHTML, "dates")
	labels := map[string]bool{
		"Start date": true, "End date": true, "Opens at": true, "Closes at": true,
		"Meeting": true, "Billing month": true, "Sprint week": true,
	}

	var got []string
	roles := map[string]bool{}
	for _, n := range interactiveSnapshot(t, ctx) {
		if n.Role == "spinbutton" {
			t.Errorf("interactive snapshot kept segment spinbutton %q, want it folded into its date/time host", n.Name)
		}
		if labels[n.Name] {
			got = append(got, n.Role+" "+strconv.Quote(n.Name)+" val="+strconv.Quote(n.Value))
			roles[n.Role] = true
		}
	}
	want := []string{
		`Date "Start date" val="2026-10-02"`,
		`Date "End date" val=""`,
		`InputTime "Opens at" val="09:30"`,
		`InputTime "Closes at" val=""`,
		`DateTime "Meeting" val=""`,
		`DateTime "Billing month" val=""`,
		`DateTime "Sprint week" val=""`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("date/time hosts = %q, want %q", got, want)
	}
	if !maps.Equal(roles, dateTimeHostRoles) {
		t.Fatalf("Chrome host roles = %v, want dateTimeHostRoles to list exactly those: %v", roles, dateTimeHostRoles)
	}
	for role := range dateTimeHostRoles {
		if !InteractiveRoles[role] {
			t.Errorf("InteractiveRoles is missing date/time host role %q", role)
		}
	}
}

func TestFullSnapshotKeepsDateTimeSegments(t *testing.T) {
	ctx := fixtureTab(t, dateTimeFixtureHTML, "dates")

	if len(nodesByRole(snapshotNodes(t, ctx), "spinbutton")) == 0 {
		t.Fatal("full snapshot dropped the date/time segment spinbuttons, want them kept outside the interactive filter")
	}
}

const contentEditableFixtureHTML = `<body id="editors">
<div contenteditable="true" aria-label="Notes">old</div>
<div contenteditable="plaintext-only" aria-label="Plain">raw</div>
<div aria-label="Panel">static</div>
<div contenteditable="true" aria-label="Rich"><span>inner</span></div>
<div tabindex="0" aria-label="Card">x</div>
<div contenteditable="true" aria-label="Doc"><p>a</p><div aria-label="Para">nested</div></div>
</body>`

func TestInteractiveSnapshotListsRolelessContentEditableAsTextbox(t *testing.T) {
	ctx := fixtureTab(t, contentEditableFixtureHTML, "editors")

	var got []string
	for _, n := range interactiveSnapshot(t, ctx) {
		got = append(got, n.Role+" "+strconv.Quote(n.Name)+" val="+strconv.Quote(n.Value))
	}
	want := []string{
		`textbox "Notes" val="old"`,
		`textbox "Plain" val="raw"`,
		`textbox "Rich" val="inner"`,
		`textbox "Doc" val="a\n\nnested"`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("interactive snapshot = %q, want %q", got, want)
	}
}
