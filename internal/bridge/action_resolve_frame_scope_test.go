package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// The frame question the world unification could silently get wrong: a scoped
// resolution must run in the document of the frame it was ASKED about, not the top
// frame's. Every other frame test in this package passes an empty frame id, which is
// the top frame — so ignoring the parameter entirely left them all green, and this
// property had no behavioural cover at all.
//
// The fixture makes the two documents distinguishable: #in-frame-only exists solely
// inside the child frame, so resolving it against the top document is
// ErrSelectorNoMatch rather than a wrong-but-plausible answer.
func TestAFrameScopedResolutionRunsInThatFramesDocument(t *testing.T) {
	ctx := newChildFrameFixture(t)

	childFrameID := childFrameIDForTest(t, ctx)

	nodeID, err := ResolveUnifiedSelectorInFrame(ctx, selector.Parse("css:#in-frame-only"), nil, childFrameID)
	if err != nil {
		t.Fatalf("resolving #in-frame-only in frame %q failed: %v — the element exists only in that frame, so this is what a resolution running in the TOP frame's document reports", childFrameID, err)
	}

	var text string
	if err := callFunctionOnNodeForTest(ctx, nodeID, `function() { return this.textContent; }`, &text); err != nil {
		t.Fatal(err)
	}
	if text != "child" {
		t.Errorf("resolved node text = %q, want %q — the handle came from the wrong document", text, "child")
	}
}

// The counter-direction on the same fixture: an element that exists ONLY in the top
// document must not be reachable from the child frame's scope. Without this, honouring
// the frame could be faked by resolving against both documents.
func TestAFrameScopedResolutionDoesNotSeeTheTopDocument(t *testing.T) {
	ctx := newChildFrameFixture(t)

	childFrameID := childFrameIDForTest(t, ctx)

	if _, err := ResolveUnifiedSelectorInFrame(ctx, selector.Parse("css:#top-only"), nil, childFrameID); err == nil {
		t.Errorf("#top-only resolved inside the child frame's scope, but it exists only in the top document — the scoped resolution is reading a document it was not asked about")
	}
}

func newChildFrameFixture(t *testing.T) context.Context {
	t.Helper()
	chromePath := testbrowser.Path(t)
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 20*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	})

	html := `<body>
	<button id="top-only">top</button>
	<iframe id="child" width="300" height="200" srcdoc="<body><button id='in-frame-only'>child</button></body>"></iframe>
	</body>`
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#child", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// childFrameIDForTest returns the id of the fixture's only child frame, failing
// rather than falling back to the top frame — a fallback would make both tests above
// exercise the very substitution they exist to detect.
func childFrameIDForTest(t *testing.T, ctx context.Context) string {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		tree, err := bridgeobserve.FetchFrameTree(ctx)
		if err != nil {
			t.Fatalf("fetch frame tree: %v", err)
		}
		if len(tree.ChildFrames) > 0 && tree.ChildFrames[0].Frame.ID != "" {
			return tree.ChildFrames[0].Frame.ID
		}
		if time.Now().After(deadline) {
			t.Fatalf("the fixture's child frame never appeared in the frame tree, so this guard would test the top frame instead: %+v", tree)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
