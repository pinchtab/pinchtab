package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

// The three ways CSS hides an element reach a descendant differently, which is
// why one of them was slipping through.
//
// display:none leaves the child no box, so the width/height check catches it.
// visibility:hidden inherits, so the child's own computed style reports it.
// opacity does NEITHER — the child keeps a real box and a computed opacity of
// 1 — so a button inside an opacity:0 panel read as visible and was clicked.
// Measured before the fix: `click #ancestoropacity` returned OK.
//
// nested is the case that requires walking past the first ancestor: opacity
// multiplies down the tree, so an inner opacity:1 cannot undo an outer 0.
const opacityFixtureHTML = `<!doctype html><html><body>
<button id="normal">Normal</button>
<div style="opacity:0.5"><button id="fading">MidFade</button></div>
<div style="opacity:0.01"><button id="barely">Barely</button></div>
<div style="opacity:0"><button id="ancestorzero">AncestorZero</button></div>
<div style="opacity:0"><div style="opacity:1"><button id="nested">NestedReset</button></div></div>
<div style="visibility:hidden"><button id="ancestorhidden">AncestorHidden</button></div>
<style>.row .actions{opacity:0} .row:hover .actions{opacity:1}</style>
<div class="row" style="padding:8px">Invoice 42 <span class="actions"><button id="rowaction">Edit</button></span></div>
<div id="fadepanel" style="opacity:0;transition:opacity 300ms linear"><button id="fadein">FadeIn</button></div>
<div id="shadowhost" style="opacity:0"></div>
<script>document.getElementById('shadowhost').attachShadow({mode:'open'}).innerHTML = '<button id="inshadow">InShadow</button>';</script>
</body></html>`

func TestPointerRefusesAnElementHiddenByAnAncestorsOpacity(t *testing.T) {
	ctx := newActionTab(t, 30*time.Second)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(opacityFixtureHTML))
	if err := chromedp.Run(ctx, chromedp.Navigate(dataURL)); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		selector   string
		wantHidden bool
		setup      string
		why        string
	}{
		{selector: "#normal", wantHidden: false, why: "nothing hides it"},
		{selector: "#fading", wantHidden: false, why: "a half-faded panel is still on screen; only a fully transparent one is not"},
		{selector: "#barely", wantHidden: false, why: "faint is not absent — the threshold is zero, not a judgement about legibility"},
		{selector: "#ancestorzero", wantHidden: true, why: "an opacity:0 panel hides what is inside it"},
		{selector: "#nested", wantHidden: true, why: "opacity multiplies down the tree, so an inner opacity:1 cannot undo an outer 0"},
		{selector: "#ancestorhidden", wantHidden: true, why: "visibility inherits, and was already refused"},
		{selector: "#rowaction", wantHidden: false, why: "a row action revealed by :hover is visible to anyone pointing at it"},
		{selector: "#fadein", wantHidden: false, setup: `document.getElementById('fadepanel').style.opacity = '1'`, why: "a panel fading in from opacity:0 is on its way to the screen, not off it"},
		{selector: "#inshadow", wantHidden: true, why: "an opacity:0 shadow host hides its shadow tree like any other ancestor"},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			if tc.setup != "" {
				if err := chromedp.Run(ctx, chromedp.Evaluate(tc.setup, nil)); err != nil {
					t.Fatal(err)
				}
			}
			backendNodeID, err := opacityFixtureNode(ctx, tc.selector)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = bridgecdpops.PointerPointForNode(ctx, backendNodeID, false)
			gotHidden := errors.Is(err, bridgecdpops.ErrElementHidden)

			switch {
			case tc.wantHidden && !gotHidden:
				t.Errorf("%s was accepted as clickable (err=%v); %s", tc.selector, err, tc.why)
			case !tc.wantHidden && gotHidden:
				t.Errorf("%s was refused as hidden; %s", tc.selector, tc.why)
			case !tc.wantHidden && err != nil:
				t.Errorf("%s failed for an unrelated reason: %v", tc.selector, err)
			}
		})
	}
}

// opacityFixtureNode resolves a fixture node by selector, reaching into
// #shadowhost's open shadow root, which a document query cannot.
func opacityFixtureNode(ctx context.Context, selector string) (int64, error) {
	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		obj, exception, err := runtime.Evaluate(`document.querySelector(` + "`" + selector + "`" + `) || document.getElementById('shadowhost').shadowRoot.querySelector(` + "`" + selector + "`" + `)`).Do(ctx)
		if err != nil {
			return err
		}
		if exception != nil {
			return exception
		}
		node, err := dom.DescribeNode().WithObjectID(obj.ObjectID).Do(ctx)
		if err != nil {
			return err
		}
		backendNodeID = int64(node.BackendNodeID)
		return nil
	}))
	return backendNodeID, err
}
