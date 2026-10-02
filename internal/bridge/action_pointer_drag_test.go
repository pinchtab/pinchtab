package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

// Two independent pairs in one page: #mouseSrc/#mouseDst answer plain mousedown/mouseup,
// #h5Src/#h5Dst are real HTML5 drag-and-drop. A page-level record is what has to be
// asserted — the action's own dragged:true was already there while nothing happened.
const dragFixtureHTML = `<style>div{width:120px;height:80px;display:inline-block;border:1px solid #000}</style>
<div id="h5Src" draggable="true">h5 src</div><div id="h5Dst">h5 dst</div><br>
<div id="mouseSrc">mouse src</div><div id="mouseDst">mouse dst</div>
<script>
window.h5 = 'no';
window.mouse = 'no';
window.pressedButton = -1;
window.heldButtons = -1;
const h5Src = document.getElementById('h5Src'), h5Dst = document.getElementById('h5Dst');
h5Src.addEventListener('dragstart', e => { window.h5 = 'started'; e.dataTransfer.setData('text/plain', 'payload'); });
h5Dst.addEventListener('dragover', e => e.preventDefault());
h5Dst.addEventListener('drop', e => { e.preventDefault(); window.h5 = 'DROPPED'; });
const mouseSrc = document.getElementById('mouseSrc'), mouseDst = document.getElementById('mouseDst');
let pressed = false;
mouseSrc.addEventListener('mousedown', e => { pressed = true; window.pressedButton = e.button; });
mouseSrc.addEventListener('contextmenu', e => e.preventDefault());
document.addEventListener('mousemove', e => { if (pressed && window.heldButtons < 0) window.heldButtons = e.buttons; });
mouseDst.addEventListener('mouseup', () => { if (pressed) window.mouse = 'DROPPED'; });
</script>`

type dragFixture struct {
	ctx context.Context
	b   *Bridge
}

func newDragFixture(t *testing.T) *dragFixture {
	t.Helper()
	return &dragFixture{ctx: newActionTab(t, 60*time.Second), b: &Bridge{}}
}

// Each trial reloads, so one trial cannot inherit another's drop.
func (f *dragFixture) reload(t *testing.T) {
	t.Helper()
	url := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(dragFixtureHTML))
	if err := chromedp.Run(f.ctx, chromedp.Navigate(url)); err != nil {
		t.Fatal(err)
	}
}

func (f *dragFixture) state(t *testing.T) (html5 string, mouse string) {
	t.Helper()
	if err := chromedp.Run(f.ctx,
		chromedp.Evaluate(`window.h5`, &html5),
		chromedp.Evaluate(`window.mouse`, &mouse),
	); err != nil {
		t.Fatal(err)
	}
	return html5, mouse
}

func (f *dragFixture) nodeID(t *testing.T, selector string) int64 {
	t.Helper()
	var nodes []*cdp.Node
	if err := chromedp.Run(f.ctx, chromedp.Nodes(selector, &nodes, chromedp.ByQuery)); err != nil {
		t.Fatalf("%s: %v", selector, err)
	}
	if len(nodes) == 0 {
		t.Fatalf("%s matched nothing", selector)
	}
	return int64(nodes[0].BackendNodeID)
}

func (f *dragFixture) point(t *testing.T, selector string) (float64, float64) {
	t.Helper()
	x, y, err := bridgecdpops.PointerPointForNode(f.ctx, f.nodeID(t, selector), true)
	if err != nil {
		t.Fatalf("%s: %v", selector, err)
	}
	return x, y
}

// The defect: `drag <from> <to>` reported three OKs and did nothing on an HTML5 draggable,
// while the offset form worked. Both forms now express the same drag, so both complete it.
func TestDragCompletesAnHTML5DropInBothForms(t *testing.T) {
	f := newDragFixture(t)

	for _, tc := range []struct {
		name string
		body func(t *testing.T) ActionRequest
	}{
		{
			name: "target form",
			body: func(t *testing.T) ActionRequest {
				return ActionRequest{Kind: ActionDrag, Selector: "#h5Src", ToSelector: "#h5Dst"}
			},
		},
		{
			// What the handler hands the bridge once it has resolved a ref or a semantic
			// selector: the destination as a node id.
			name: "target form by node id",
			body: func(t *testing.T) ActionRequest {
				return ActionRequest{Kind: ActionDrag, NodeID: f.nodeID(t, "#h5Src"), ToNodeID: f.nodeID(t, "#h5Dst")}
			},
		},
		{
			name: "target form by coordinates",
			body: func(t *testing.T) ActionRequest {
				x, y := f.point(t, "#h5Dst")
				return ActionRequest{Kind: ActionDrag, Selector: "#h5Src", ToX: x, ToY: y, HasToXY: true}
			},
		},
		{
			name: "offset form",
			body: func(t *testing.T) ActionRequest {
				srcX, srcY := f.point(t, "#h5Src")
				dstX, dstY := f.point(t, "#h5Dst")
				return ActionRequest{Kind: ActionDrag, Selector: "#h5Src", DragX: int(dstX - srcX), DragY: int(dstY - srcY)}
			},
		},
	} {
		f.reload(t)
		req := tc.body(t)

		result, err := f.b.actionDrag(f.ctx, req)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if result["dragged"] != true {
			t.Errorf("%s: result = %v, want dragged:true", tc.name, result)
		}

		html5, _ := f.state(t)
		if html5 != "DROPPED" {
			t.Errorf("%s: page recorded h5=%q, want DROPPED; %q means dragstart never fired at all", tc.name, html5, "no")
		}
	}
}

// The mouse-event pair works today and must keep working: the fix cannot be a trade of one
// drag style for the other.
func TestDragStillCompletesAMouseDrivenDrop(t *testing.T) {
	f := newDragFixture(t)
	f.reload(t)

	if _, err := f.b.actionDrag(f.ctx, ActionRequest{Kind: ActionDrag, Selector: "#mouseSrc", ToSelector: "#mouseDst"}); err != nil {
		t.Fatal(err)
	}

	_, mouse := f.state(t)
	if mouse != "DROPPED" {
		t.Errorf("page recorded mouse=%q, want DROPPED", mouse)
	}
}

// The two forms are documented as forms of one command, so the same source and destination
// must produce the same page state whichever one expresses it.
func TestBothDragFormsProduceTheSameOutcome(t *testing.T) {
	f := newDragFixture(t)

	f.reload(t)
	if _, err := f.b.actionDrag(f.ctx, ActionRequest{Kind: ActionDrag, Selector: "#h5Src", ToSelector: "#h5Dst"}); err != nil {
		t.Fatal(err)
	}
	targetH5, targetMouse := f.state(t)

	f.reload(t)
	srcX, srcY := f.point(t, "#h5Src")
	dstX, dstY := f.point(t, "#h5Dst")
	if _, err := f.b.actionDrag(f.ctx, ActionRequest{Kind: ActionDrag, Selector: "#h5Src", DragX: int(dstX - srcX), DragY: int(dstY - srcY)}); err != nil {
		t.Fatal(err)
	}
	offsetH5, offsetMouse := f.state(t)

	if targetH5 != offsetH5 || targetMouse != offsetMouse {
		t.Errorf("target form left (h5=%q mouse=%q), offset form left (h5=%q mouse=%q); the help presents them as forms of one command", targetH5, targetMouse, offsetH5, offsetMouse)
	}
}

// A drop onto an overlapping or same-position target is a legitimate request. The offset
// form has to refuse 0,0 — it means "no movement asked for" — but the target form must not
// inherit a refusal written for a different question.
func TestATargetDragOntoTheSourceIsNotRefusedAsAZeroOffset(t *testing.T) {
	f := newDragFixture(t)
	f.reload(t)

	if _, err := f.b.actionDrag(f.ctx, ActionRequest{Kind: ActionDrag, Selector: "#h5Src", ToSelector: "#h5Src"}); err != nil {
		t.Fatalf("a drag onto its own source was refused: %v", err)
	}

	if _, err := f.b.actionDrag(f.ctx, ActionRequest{Kind: ActionDrag, Selector: "#h5Src"}); err == nil {
		t.Error("the offset form accepted a drag with no offset and no destination, so a caller who forgot both reads it as success")
	}
}

// The individual pointer primitives are fine for what they are, and the fix must not have
// changed what they do for everyone else: four requests still move the pointer in one jump,
// which is exactly why `drag` can no longer be assembled out of them.
func TestThePointerPrimitivesStillDispatchOneJumpEach(t *testing.T) {
	f := newDragFixture(t)

	f.reload(t)
	f.handAssembledDrag(t, "#mouseSrc", "#mouseDst")
	if _, mouse := f.state(t); mouse != "DROPPED" {
		t.Errorf("the primitives stopped completing a mouse-driven drop: mouse=%q", mouse)
	}

	f.reload(t)
	f.handAssembledDrag(t, "#h5Src", "#h5Dst")
	if html5, _ := f.state(t); html5 != "no" {
		t.Errorf("hand-assembled pointer steps recorded h5=%q; if they now complete an HTML5 drop, this test records an obsolete limitation and `drag` no longer needs to own the sequence", html5)
	}
}

// The reason `drag` has to own the sequence is the button, not the number of steps: every
// `mouse move` is its own request and knows of no press, so its moves report no button held
// and Chrome never enters the drag pipeline. Measured both ways — one interpolated move with
// the button held completes the drop, while these twenty without it do not — so a caller who
// reads the limitation as "one jump" and answers it with small steps still gets nothing.
func TestManySmallHandAssembledStepsStillCompleteNoHTML5Drop(t *testing.T) {
	f := newDragFixture(t)
	const moves = 20

	// The mouse pair first, so the negative below cannot pass by the sequence never landing:
	// the same twenty steps DO complete a mouse-driven drop.
	f.reload(t)
	f.handAssembledDragInSteps(t, "#mouseSrc", "#mouseDst", moves)
	if _, mouse := f.state(t); mouse != "DROPPED" {
		t.Fatalf("%d hand-assembled moves did not complete even a mouse-driven drop (mouse=%q), so the HTML5 reading below would prove nothing", moves, mouse)
	}

	f.reload(t)
	f.handAssembledDragInSteps(t, "#h5Src", "#h5Dst", moves)
	if html5, _ := f.state(t); html5 != "no" {
		t.Errorf("%d hand-assembled moves recorded h5=%q; if the primitives now complete an HTML5 drop, the docs' held-button explanation is obsolete", moves, html5)
	}
}

func (f *dragFixture) handAssembledDragInSteps(t *testing.T, from, to string, moves int) {
	t.Helper()

	srcX, srcY := f.point(t, from)
	dstX, dstY := f.point(t, to)
	at := func(i int) ActionRequest {
		p := float64(i) / float64(moves)
		return ActionRequest{X: srcX + p*(dstX-srcX), Y: srcY + p*(dstY-srcY), HasXY: true}
	}

	if _, err := f.b.actionMouseMove(f.ctx, at(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.actionMouseDown(f.ctx, at(0)); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= moves; i++ {
		if _, err := f.b.actionMouseMove(f.ctx, at(i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.b.actionMouseUp(f.ctx, at(moves)); err != nil {
		t.Fatal(err)
	}
}

func (f *dragFixture) buttonRecord(t *testing.T) (pressed int, held int) {
	t.Helper()
	if err := chromedp.Run(f.ctx,
		chromedp.Evaluate(`window.pressedButton`, &pressed),
		chromedp.Evaluate(`window.heldButtons`, &held),
	); err != nil {
		t.Fatal(err)
	}
	return pressed, held
}

// dragCmd registers --button with a default, so the flag reaches the body on every drag and
// "is it set" proves nothing — only the page can say which button was actually held. Both
// forms have to honour it: the press, and the interpolated moves that carry the held mask.
func TestDragHoldsTheRequestedButton(t *testing.T) {
	f := newDragFixture(t)

	for _, tc := range []struct {
		button      string
		wantPressed int
		wantHeld    int
	}{
		{button: "", wantPressed: 0, wantHeld: 1},
		{button: "left", wantPressed: 0, wantHeld: 1},
		{button: "right", wantPressed: 2, wantHeld: 2},
		{button: "middle", wantPressed: 1, wantHeld: 4},
	} {
		for _, form := range []struct {
			name string
			req  func(t *testing.T) ActionRequest
		}{
			{
				name: "target form",
				req: func(t *testing.T) ActionRequest {
					return ActionRequest{Kind: ActionDrag, Selector: "#mouseSrc", ToSelector: "#mouseDst", Button: tc.button}
				},
			},
			{
				name: "offset form",
				req: func(t *testing.T) ActionRequest {
					srcX, srcY := f.point(t, "#mouseSrc")
					dstX, dstY := f.point(t, "#mouseDst")
					return ActionRequest{Kind: ActionDrag, Selector: "#mouseSrc", DragX: int(dstX - srcX), DragY: int(dstY - srcY), Button: tc.button}
				},
			},
		} {
			f.reload(t)
			if _, err := f.b.actionDrag(f.ctx, form.req(t)); err != nil {
				t.Fatalf("button %q %s: %v", tc.button, form.name, err)
			}

			pressed, held := f.buttonRecord(t)
			if pressed != tc.wantPressed {
				t.Errorf("button %q %s: page saw mousedown button %d, want %d", tc.button, form.name, pressed, tc.wantPressed)
			}
			if held != tc.wantHeld {
				t.Errorf("button %q %s: page saw buttons=%d during the drag, want %d; the moves must carry the same button as the press", tc.button, form.name, held, tc.wantHeld)
			}
		}
	}
}

// The layer TestDragHoldsTheRequestedButton cannot reach: it restates three names and the
// expected codes, so a name added to the vocabulary is invisible to it, and the browserless
// table test that DOES enumerate the vocabulary stubs the wire. This drives every name the
// vocabulary advertises through a real browser and asserts the page reads a DISTINCT press
// button and held mask for each, which is the reading a name dispatched as another name's
// row cannot produce — without restating any code the vocabulary owns.
func TestEveryButtonInTheVocabularyReachesThePageAsItsOwnPair(t *testing.T) {
	names := bridgecdpops.MouseButtons()
	if len(names) < 3 {
		t.Fatalf("vocabulary = %v; a distinctness sweep over fewer than the three documented buttons passes vacuously", names)
	}

	f := newDragFixture(t)
	seenPress, seenHeld := map[int]string{}, map[int]string{}
	for _, name := range names {
		f.reload(t)
		if _, err := f.b.actionDrag(f.ctx, ActionRequest{Kind: ActionDrag, Selector: "#mouseSrc", ToSelector: "#mouseDst", Button: name}); err != nil {
			t.Fatalf("button %q: %v", name, err)
		}

		pressed, held := f.buttonRecord(t)
		if pressed < 0 {
			t.Errorf("button %q: the page saw no mousedown at all", name)
			continue
		}
		if held <= 0 {
			t.Errorf("button %q: the page read buttons=%d during the moves, so the drag moved with nothing held", name, held)
			continue
		}
		if held&(held-1) != 0 {
			t.Errorf("button %q: the page read buttons=%d, which is more than one bit — the moves held a different button than the press", name, held)
		}

		// Two separate distinctness maps, not one over the pair: a press that keeps its own
		// code while every move holds the same mask is exactly the split this card removed,
		// and a combined key would read as distinct and hide it.
		if other, clash := seenPress[pressed]; clash {
			t.Errorf("button %q pressed as button=%d, the same code the page read for %q — one was dispatched as the other", name, pressed, other)
		}
		if other, clash := seenHeld[held]; clash {
			t.Errorf("button %q moved with buttons=%d, the same mask the page read for %q — the press said %q while the moves held %q's button", name, held, other, name, other)
		}
		seenPress[pressed], seenHeld[held] = name, name
	}
}

// A drag cannot be both a destination and an offset. Picking one silently discards the
// other, which is a confidently wrong drag reported as success.
func TestDragRefusesADestinationAndAnOffsetTogether(t *testing.T) {
	f := newDragFixture(t)
	f.reload(t)

	_, err := f.b.actionDrag(f.ctx, ActionRequest{Kind: ActionDrag, Selector: "#h5Src", ToSelector: "#h5Dst", DragX: 180})
	if err == nil {
		t.Fatal("a drag carrying both a destination and an offset was accepted, so one of them was silently dropped")
	}
	if html5, _ := f.state(t); html5 != "no" {
		t.Errorf("the refused drag still moved the pointer: h5=%q", html5)
	}
}

func TestAnUnsatisfiableDragBodyRefusesWithTheCallerErrorSentinelWithoutActing(t *testing.T) {
	for _, tc := range []struct {
		name     string
		req      ActionRequest
		wantSays []string
	}{
		{
			name:     "a destination and an offset together",
			req:      ActionRequest{Kind: ActionDrag, Selector: "#src", ToSelector: "#dst", DragX: 180},
			wantSays: []string{"toSelector/toNodeId/toX+toY", "dragX/dragY"},
		},
		{
			name:     "an offset drag with no offset",
			req:      ActionRequest{Kind: ActionDrag, Selector: "#src"},
			wantSays: []string{"dragX or dragY"},
		},
		{
			name:     "no target at all",
			req:      ActionRequest{Kind: ActionDrag, DragX: 180},
			wantSays: []string{"need selector, ref, or nodeId"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (&Bridge{}).actionDrag(context.Background(), tc.req)
			if result != nil {
				t.Errorf("result = %v; a refusal must not report having acted", result)
			}
			if !errors.Is(err, ErrInvalidActionRequest) {
				t.Fatalf("err = %v, want the ErrInvalidActionRequest sentinel; without it the handler reports this caller error as a retryable server fault", err)
			}
			for _, says := range tc.wantSays {
				if !strings.Contains(err.Error(), says) {
					t.Errorf("err = %q, want it to carry %q; the useful half of the message must survive the classification", err, says)
				}
			}
		})
	}
}

// handAssembledDrag is the sequence the CLI used to post as four independent requests: one
// move to the source, a press, ONE move to the destination, a release.
func (f *dragFixture) handAssembledDrag(t *testing.T, from, to string) {
	t.Helper()

	srcX, srcY := f.point(t, from)
	dstX, dstY := f.point(t, to)
	for _, step := range []struct {
		kind   string
		action ActionFunc
		req    ActionRequest
	}{
		{kind: ActionMouseMove, action: f.b.actionMouseMove, req: ActionRequest{X: srcX, Y: srcY, HasXY: true}},
		{kind: ActionMouseDown, action: f.b.actionMouseDown, req: ActionRequest{X: srcX, Y: srcY, HasXY: true}},
		{kind: ActionMouseMove, action: f.b.actionMouseMove, req: ActionRequest{X: dstX, Y: dstY, HasXY: true}},
		{kind: ActionMouseUp, action: f.b.actionMouseUp, req: ActionRequest{X: dstX, Y: dstY, HasXY: true}},
	} {
		if _, err := step.action(f.ctx, step.req); err != nil {
			t.Fatalf("%s: %v", step.kind, err)
		}
	}
}
