package bridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/selector"
)

// Every trap that made the wrapped and unwrapped resolvers disagree, on one
// page: a <style> and a <script> whose source text mentions "Publish draft"
// with a smaller subtree than the real button, a second control behind an open
// shadow boundary, and a <div> wrapping a <button> that share a label.
const unifiedSelectorFixtureHTML = `<!doctype html>
<html>
<head>
<style>/* Publish draft button styling */
#real { color: rebeccapurple; }</style>
</head>
<body>
<script>const hint = "Publish draft";</script>
<button id="real">Publish draft</button>
<div id="decoy">Archive record</div>
<div id="wrap"><button id="inner">Archive record</button></div>
<div id="host"></div>
<script>
document.getElementById('host').attachShadow({mode: 'open'}).innerHTML =
	'<button id="shadowbtn">Send invoice</button>';
</script>
</body>
</html>`

func newSelectorFixture(t *testing.T) context.Context {
	t.Helper()
	ctx := newActionTab(t, 30*time.Second)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(unifiedSelectorFixtureHTML))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#real", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func resolve(t *testing.T, ctx context.Context, raw string) int64 {
	t.Helper()
	nodeID, err := ResolveUnifiedSelector(ctx, selector.Parse(raw), nil)
	if err != nil {
		t.Fatalf("resolve %q: %v", raw, err)
	}
	return nodeID
}

// describeNode reports what a selector actually landed on ("button#real",
// "style"), so a failure names the wrong element instead of printing two opaque
// node ids.
func describeNode(t *testing.T, ctx context.Context, backendNodeID int64) string {
	t.Helper()
	var out string
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var raw json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &raw); err != nil {
			return err
		}
		var resolved struct {
			Object struct {
				Description string `json:"description"`
			} `json:"object"`
		}
		if err := json.Unmarshal(raw, &resolved); err != nil {
			return err
		}
		out = resolved.Object.Description
		return nil
	}))
	if err != nil {
		t.Fatalf("describe node %d: %v", backendNodeID, err)
	}
	return out
}

// The regression guard for the whole card: a wrapper may choose among matches,
// never change which matches exist. Before unification text: found the button,
// first: found the <style>, and nth:0: was a third implementation again.
func TestWrappedAndPlainTextSelectorsResolveTheSameNode(t *testing.T) {
	ctx := newSelectorFixture(t)

	plain := resolve(t, ctx, "text:Publish draft")
	for _, raw := range []string{"first:text:Publish draft", "last:text:Publish draft", "nth:0:text:Publish draft"} {
		if got := resolve(t, ctx, raw); got != plain {
			t.Errorf("%s resolved to %s, want the same node as text:Publish draft (%s)",
				raw, describeNode(t, ctx, got), describeNode(t, ctx, plain))
		}
	}

	if want := resolve(t, ctx, "css:#real"); plain != want {
		t.Errorf("text:Publish draft resolved to %s, want button#real (%s)",
			describeNode(t, ctx, plain), describeNode(t, ctx, want))
	}
}

// css: was rerouted through the deep walker for shadow support and text: was
// left behind, so the same element was reachable by one selector kind and not
// the other.
func TestTextSelectorPiercesOpenShadowRootLikeCSS(t *testing.T) {
	ctx := newSelectorFixture(t)

	byCSS := resolve(t, ctx, "css:#shadowbtn")
	for _, raw := range []string{"text:Send invoice", "first:text:Send invoice", "nth:0:text:Send invoice"} {
		if got := resolve(t, ctx, raw); got != byCSS {
			t.Errorf("%s resolved to %s, want button#shadowbtn (%s)",
				raw, describeNode(t, ctx, got), describeNode(t, ctx, byCSS))
		}
	}
}

// A CSS comment or a script string mentioning a label is not a control. These
// elements have tiny subtrees, so before the exclusion they outranked the real
// button on the leaf-most rule.
func TestTextSelectorNeverResolvesNonRenderedSourceText(t *testing.T) {
	ctx := newSelectorFixture(t)

	for _, raw := range []string{
		"text:Publish draft",
		"first:text:Publish draft",
		"last:text:Publish draft",
		"nth:0:text:Publish draft",
	} {
		got := describeNode(t, ctx, resolve(t, ctx, raw))
		for _, forbidden := range []string{"style", "script", "noscript", "template"} {
			if got == forbidden || len(got) >= len(forbidden) && got[:len(forbidden)] == forbidden {
				t.Errorf("%s resolved to %s — source text is not rendered text", raw, got)
			}
		}
	}
}

// semanticWeight came from the deleted resolver and had to be carried over, or
// unification would make the common case worse than it was.
//
// The fixture has to make the two rules disagree to pin the second one. #wrap
// loses to #inner on subtree size alone, so a wrapping div proves nothing about
// semantic weight; #decoy is a bare div with the same label and the same zero
// descendants, so it ties #inner on size and only semanticWeight breaks it —
// and #decoy comes first in the document, so DOM order alone picks it.
func TestTextSelectorPrefersControlOverPlainElementOfEqualSize(t *testing.T) {
	ctx := newSelectorFixture(t)

	// The first: row is gone on purpose, not adjusted: positional wrappers index
	// the document now, so first:text: is #decoy. The control preference this test
	// is named for lives on the BARE text: selector, which is the row that stays.
	want := resolve(t, ctx, "css:#inner")
	if got := resolve(t, ctx, "text:Archive record"); got != want {
		t.Errorf("text:Archive record resolved to %s, want button#inner (%s)",
			describeNode(t, ctx, got), describeNode(t, ctx, want))
	}

	// The wrapper is a candidate the size rule must have already discarded.
	if got := resolve(t, ctx, "text:Archive record"); got == resolve(t, ctx, "css:#wrap") {
		t.Error("text:Archive record resolved to the wrapping div, not the button inside it")
	}
}

// The deadline used to live in the deleted text resolver, so the wrapped forms
// never had one. resolveSelectorAt is now the only way any css/xpath/text
// selector is resolved — plain or wrapped, frame-scoped or dialog-scoped — so
// bounding it here bounds every text path.
func TestResolveSelectorAtBoundsTextLookupsOnly(t *testing.T) {
	for _, tc := range []struct {
		kind    selector.Kind
		bounded bool
	}{
		{selector.KindText, true},
		{selector.KindCSS, false},
		{selector.KindXPath, false},
	} {
		var deadline time.Time
		var hasDeadline bool
		_, err := resolveSelectorAt(context.Background(), selector.Selector{Kind: tc.kind, Value: "x"}, 0, false, false, "",
			func(ctx context.Context, _ []map[string]any) (int64, error) {
				deadline, hasDeadline = ctx.Deadline()
				return 7, nil
			})
		if err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		if hasDeadline != tc.bounded {
			t.Errorf("%s selector: deadline present = %v, want %v", tc.kind, hasDeadline, tc.bounded)
			continue
		}
		if tc.bounded && time.Until(deadline) > textLookupDeadline {
			t.Errorf("%s selector: deadline is %s away, want at most %s", tc.kind, time.Until(deadline), textLookupDeadline)
		}
	}
}

// Kinds resolved elsewhere must still be rejected rather than reaching the
// browser with a kind the resolver's switch does not implement.
func TestResolveSelectorAtRejectsUnwrappableKinds(t *testing.T) {
	called := false
	_, err := resolveSelectorAt(context.Background(), selector.Selector{Kind: selector.KindSemantic, Value: "Save"}, 0, false, false, "",
		func(context.Context, []map[string]any) (int64, error) {
			called = true
			return 0, nil
		})
	if err == nil {
		t.Fatal("semantic selector should not be resolvable through the unified resolver")
	}
	if called {
		t.Error("an unsupported kind must be rejected before the browser is asked")
	}
}

// document.evaluate cannot cross a shadow boundary. XPath gains consistency and
// isolated-world execution from the unification; it does not gain shadow
// support, and this pins that boundary rather than leaving it ambiguous.
func TestXPathDoesNotPierceShadowRoots(t *testing.T) {
	ctx := newSelectorFixture(t)

	if nodeID, err := ResolveUnifiedSelector(ctx, selector.Parse(`xpath://button[@id="shadowbtn"]`), nil); err == nil {
		t.Fatalf("xpath resolved a shadow-root element to %s; document.evaluate cannot cross shadow boundaries",
			describeNode(t, ctx, nodeID))
	}

	// The same XPath against a light-DOM element still works, so the failure
	// above is the shadow boundary and not a broken XPath path.
	light := resolve(t, ctx, `xpath://button[@id="real"]`)
	if want := resolve(t, ctx, "css:#real"); light != want {
		t.Errorf("xpath light-DOM lookup resolved to %s, want button#real (%s)",
			describeNode(t, ctx, light), describeNode(t, ctx, want))
	}
}

// The decided grammar, on the fixture where the two orders disagree: #decoy is a
// weight-0 div that PRECEDES the weight-0.25 #inner button, and the two tie on
// subtree size. Before this, nth:1 resolved to an element earlier in the document
// than nth:0, because pick() indexed a weight-sorted list.
func TestPositionalWrappersOverTextIndexDocumentOrder(t *testing.T) {
	ctx := newSelectorFixture(t)

	first := resolve(t, ctx, "css:#decoy")
	last := resolve(t, ctx, "css:#inner")

	for _, tc := range []struct {
		raw  string
		want int64
		what string
	}{
		{"first:text:Archive record", first, "div#decoy, the document-first candidate"},
		{"nth:0:text:Archive record", first, "div#decoy, the document-first candidate"},
		{"nth:1:text:Archive record", last, "button#inner, the candidate AFTER nth:0"},
		{"last:text:Archive record", last, "button#inner, the document-last candidate"},
	} {
		if got := resolve(t, ctx, tc.raw); got != tc.want {
			t.Errorf("%s resolved to %s, want %s", tc.raw, describeNode(t, ctx, got), tc.what)
		}
	}

	// The defect stated as an ordering property rather than as two node ids: an
	// index must never walk backwards through the document.
	if resolve(t, ctx, "nth:0:text:Archive record") == resolve(t, ctx, "nth:1:text:Archive record") {
		t.Fatal("nth:0 and nth:1 resolved to the same node; the fixture no longer has two candidates")
	}
	if resolve(t, ctx, "first:text:Archive record") != resolve(t, ctx, "nth:0:text:Archive record") {
		t.Error("first:text: and nth:0:text: must be the same candidate")
	}

	// The decision itself: a bare text: may differ from all three, and here it
	// does. Leaving this implicit is how option B ships by accident.
	plain := resolve(t, ctx, "text:Archive record")
	if plain == first {
		t.Error("text:Archive record resolved to the document-first div; the control ranking on the bare selector is gone")
	}
	if plain != last {
		t.Errorf("text:Archive record resolved to %s, want button#inner — the bare selector still ranks by control-likeness",
			describeNode(t, ctx, plain))
	}
}

// The change must not read as a general reordering: css: and xpath: indexed
// document order before and still do, on the same two elements.
func TestPositionalWrappersOverCSSAndXPathStillIndexDocumentOrder(t *testing.T) {
	ctx := newSelectorFixture(t)

	decoy := resolve(t, ctx, "css:#decoy")
	inner := resolve(t, ctx, "css:#inner")

	for _, tc := range []struct {
		raw  string
		want int64
	}{
		{"first:css:#decoy,#inner", decoy},
		{"last:css:#decoy,#inner", inner},
		{"nth:1:css:#decoy,#inner", inner},
		{`first:xpath://*[@id="decoy" or @id="inner"]`, decoy},
		{`last:xpath://*[@id="decoy" or @id="inner"]`, inner},
	} {
		if got := resolve(t, ctx, tc.raw); got != tc.want {
			t.Errorf("%s resolved to %s, want %s", tc.raw, describeNode(t, ctx, got), describeNode(t, ctx, tc.want))
		}
	}
}
