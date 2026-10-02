package assets_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// The extraction runs in a browser, so it is proven in one. A Go-level test can only
// drive the handler's mock, which returns whatever string it was handed and can never
// see what the script does to a DOM.
func readabilityPage(t *testing.T, html string) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(testbrowser.Tab(t), 30*time.Second)
	t.Cleanup(cancel)

	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html)))); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func extractReadability(t *testing.T, ctx context.Context) string {
	t.Helper()
	var text string
	if err := chromedp.Run(ctx, chromedp.Evaluate(assets.ReadabilityJS, &text)); err != nil {
		t.Fatal(err)
	}
	return text
}

// The reported document: a status code and a timestamp in adjacent cells came back as
// one unparseable number, and two adjacent paragraphs came back as one word. The source
// carries no whitespace between the tags, which is what every JS-rendered dashboard,
// search result and log view looks like.
const fusedFieldsPage = `<table><thead><tr><th>Code</th><th>Time</th><th>Path</th></tr></thead>` +
	`<tbody><tr><td>200</td><td>09:38:14</td><td>/health</td></tr>` +
	`<tr><td>404</td><td>10:01:02</td><td>/missing</td></tr></tbody></table>` +
	`<ul><li>alpha</li><li>beta</li></ul><p>para one</p><p>para two</p>`

func TestDefaultExtractionKeepsAdjacentFieldsApart(t *testing.T) {
	text := extractReadability(t, readabilityPage(t, fusedFieldsPage))

	for _, fused := range []string{"20009:38:14", "40410:01:02", "alphabeta", "para onepara two", "CodeTimePath"} {
		if strings.Contains(text, fused) {
			t.Errorf("the extraction still fuses %q, so the field boundary is gone and cannot be recovered:\n%s", fused, text)
		}
	}
	for _, field := range []string{"200", "09:38:14", "/health", "alpha", "beta", "para one", "para two"} {
		if !strings.Contains(text, field) {
			t.Errorf("%q is missing from the extraction; separating the fields must not drop any:\n%s", field, text)
		}
	}
}

// The cheapest permanent guard, and the one the defect would have failed: two adjacent
// block elements must never serialise with nothing between them. Asserted on the DEFAULT
// mode, since that is the one nobody has to opt into.
func TestTwoAdjacentBlocksNeverSerialiseWithNoSeparator(t *testing.T) {
	for _, tag := range []string{"p", "div", "li", "h2", "section", "td"} {
		t.Run(tag, func(t *testing.T) {
			body := "<" + tag + ">alpha</" + tag + "><" + tag + ">beta</" + tag + ">"
			if tag == "td" {
				body = "<table><tbody><tr>" + body + "</tr></tbody></table>"
			}
			text := extractReadability(t, readabilityPage(t, body))

			if !strings.Contains(text, "alpha") || !strings.Contains(text, "beta") {
				t.Fatalf("fixture did not survive extraction: %q", text)
			}
			if strings.Contains(text, "alphabeta") {
				t.Errorf("two adjacent <%s> serialised with zero separator: %q", tag, text)
			}
		})
	}
}

// A separator is inserted ONLY where the text either side would otherwise fuse, so a
// document whose source already carries whitespace between its tags is untouched. The
// oracle is the extraction this replaced — clone, strip, read as textContent — computed
// in the page, so the claim is measured rather than asserted against a hand-typed string.
const previousExtraction = `(() => {
  const root = document.body.cloneNode(true);
  root.querySelectorAll('script, style, noscript, svg, [hidden]').forEach(el => el.remove());
  return root.textContent.replace(/\n{3,}/g, '\n\n').trim();
})()`

func TestASourceThatAlreadySeparatesItsBlocksIsUnchanged(t *testing.T) {
	prettyPrinted := `<div>
  <h1>Title</h1>
  <p>First paragraph.</p>
  <p>Second paragraph.</p>
  <ul>
    <li>alpha</li>
    <li>beta</li>
  </ul>
</div>`
	ctx := readabilityPage(t, prettyPrinted)

	var before string
	if err := chromedp.Run(ctx, chromedp.Evaluate(previousExtraction, &before)); err != nil {
		t.Fatal(err)
	}
	after := extractReadability(t, ctx)

	if before == "" {
		t.Fatal("the oracle extracted nothing, so this comparison would pass vacuously")
	}
	if after != before {
		t.Errorf("a body with whitespace already between its blocks changed:\n before %q\n after  %q", before, after)
	}
}

// Whatever else it does, the extraction only ADDS whitespace: no character of the text
// is dropped, reordered or rewritten. That is what makes the change safe to default on,
// and it holds for the fused document too, where the output differs the most.
func TestTheExtractionOnlyInsertsWhitespace(t *testing.T) {
	ctx := readabilityPage(t, fusedFieldsPage)

	var before string
	if err := chromedp.Run(ctx, chromedp.Evaluate(previousExtraction, &before)); err != nil {
		t.Fatal(err)
	}
	after := extractReadability(t, ctx)

	if squeeze(before) == "" {
		t.Fatal("the oracle extracted nothing, so this comparison would pass vacuously")
	}
	if squeeze(after) != squeeze(before) {
		t.Errorf("the extraction changed more than the whitespace:\n before %q\n after  %q", squeeze(before), squeeze(after))
	}
	if after == before {
		t.Error("the fused document extracted identically, so this fixture no longer exercises the defect")
	}
}

func squeeze(s string) string {
	return strings.Join(strings.Fields(s), "")
}

const checkoutPage = `<form><label>Country <select name="c">` +
	`<option>Afghanistan</option><option>Albania</option><option>Japan</option><option>Sweden</option>` +
	`</select></label><label>Delivery<select name="d"><option>Standard (3-5 days)</option>` +
	`<option selected>Express (1 day)</option></select></label><button>Pay</button><button>Cancel</button></form>`

func TestASelectRendersOnlyItsLiveSelectedLabel(t *testing.T) {
	ctx := readabilityPage(t, checkoutPage)
	if err := chromedp.Run(ctx, chromedp.SetValue(`select[name=c]`, "Japan", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	text := extractReadability(t, ctx)

	for _, want := range []string{"Country Japan", "Delivery Express (1 day) Pay Cancel"} {
		if !strings.Contains(text, want) {
			t.Errorf("want %q in the extraction, the selected labels read from the live page with every boundary kept:\n%s", want, text)
		}
	}
	for _, unchosen := range []string{"Afghanistan", "Sweden", "Standard"} {
		if strings.Contains(text, unchosen) {
			t.Errorf("the unselected option %q leaked into the extraction:\n%s", unchosen, text)
		}
	}
}

func TestAMultiSelectJoinsItsSelectedLabelsAndAnEmptyOneRendersNothing(t *testing.T) {
	text := extractReadability(t, readabilityPage(t, `<p>Tags<select multiple>`+
		`<option selected>red</option><option>green</option><option selected>blue</option></select>`+
		`Sizes<select multiple><option>S</option><option>M</option></select>end</p>`))

	if !strings.Contains(text, "Tags red, blue Sizes end") {
		t.Errorf("want the selected labels joined by ', ' and nothing for an empty multi-select:\n%s", text)
	}
}

func TestInlineTextNeverFusesIntoTheButtonAfterIt(t *testing.T) {
	text := extractReadability(t, readabilityPage(t, `<p>Total $5<button>Pay</button></p>`))

	if !strings.Contains(text, "Total $5 Pay") {
		t.Errorf("want the text before a button kept apart from its label:\n%s", text)
	}
}

// The CI twin. Every guard above needs a browser and skips without one, so a revert of
// the fix would land green on a machine that has none. This one reds anywhere: the whole
// defect was reading innerText off a node that has no layout, so the script must not read
// innerText from its clone at all, and must carry the walk that replaced it.
func TestTheScriptDoesNotReadInnerTextFromTheDetachedClone(t *testing.T) {
	if strings.Contains(assets.ReadabilityJS, "root.innerText") {
		t.Error("the script reads innerText from the clone again; the clone is detached, so innerText has no layout to consult and silently degrades to textContent — every field boundary is lost. Serialise the clone instead")
	}
	for _, required := range []string{"BLOCKS", "queue(", "serialize(", "selectedOptions"} {
		if !strings.Contains(assets.ReadabilityJS, required) {
			t.Errorf("the separator-aware walk is gone (%q not found); without it adjacent blocks and table cells fuse — re-point this guard at whatever replaced the walk rather than deleting it", required)
		}
	}
}

// A read must not alter the page under an agent. This watches for mutations DURING the
// extraction rather than comparing the document before and after: the tempting alternative
// fix — attach the clone off-screen so it gets layout, read innerText, remove it — restores
// the document perfectly and a before/after comparison cannot see it, while the page's own
// observers fire and every image and iframe in the copy is re-requested.
var observedExtraction = `(() => {
  const seen = new MutationObserver(() => {});
  seen.observe(document, {subtree: true, childList: true, attributes: true, characterData: true});
  const text = ` + assets.ReadabilityJS + `;
  const records = seen.takeRecords();
  seen.disconnect();
  return records.length + ':' + text.length;
})()`

func TestExtractionMutatesNothingInTheLivePage(t *testing.T) {
	ctx := readabilityPage(t, `<nav>skip me</nav><main><p>alpha</p><p>beta</p></main><footer>gone</footer>`)

	var beforeHTML, observed, afterHTML string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.documentElement.outerHTML`, &beforeHTML),
		chromedp.Evaluate(observedExtraction, &observed),
		chromedp.Evaluate(`document.documentElement.outerHTML`, &afterHTML),
	); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(beforeHTML, "skip me") {
		t.Fatal("the fixture never had the stripped nodes, so an in-place strip would not show here")
	}
	mutations, extractedLen, _ := strings.Cut(observed, ":")
	if extractedLen == "0" || extractedLen == "" {
		t.Fatalf("the extraction returned nothing, so this guard checked nothing: %q", observed)
	}
	if mutations != "0" {
		t.Errorf("the extraction made %s mutation records against the live document; a read must not alter the page an agent is driving — strip and serialise a detached clone instead of attaching one", mutations)
	}
	if afterHTML != beforeHTML {
		t.Errorf("the live DOM did not survive a read:\n before %q\n after  %q", beforeHTML, afterHTML)
	}
}
