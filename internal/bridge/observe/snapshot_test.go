package observe

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
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
