package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/selector"
)

func TestDialogScopeReproducesGlobalEscapeAndContainsActionsAndReads(t *testing.T) {
	chromePath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}
	profile, err := os.MkdirTemp("", "pinchtab-modal-scope-")
	if err != nil {
		t.Fatal(err)
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 20*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
		_ = os.RemoveAll(profile)
	})

	html := `<style>
		[role=dialog] { position: fixed; inset: 20px; background: white; }
		#lower { z-index: 10; }
		#top { z-index: 20; }
	</style>
	<button id="background-action" onclick="window.backgroundClicks++">Content contains</button>
	<table><tr><td>Background policy grid</td><td>Select row</td></tr></table>
	<iframe id="child" srcdoc="<button id='frame-background'>Content contains</button>"></iframe>
	<div id="lower" role="dialog" aria-modal="true"><button>Content contains</button></div>
	<div id="top" role="dialog" aria-modal="true">
		<h2>Sensitive info picker</h2>
	</div>
	<div id="shadow-host"></div>
	<script>
		window.backgroundClicks = 0; window.dialogClicks = 0;
		const root = document.getElementById("shadow-host").attachShadow({mode:"open"});
		root.innerHTML = '<style>#shadow-top { position:fixed; inset:30px; z-index:30; background:white }</style>' +
			'<div id="shadow-top" role="dialog" aria-modal="true">' +
			'<h2>Sensitive info picker</h2>' +
			'<button id="dialog-action">Content contains</button>' +
			'<div role="row">Dialog option Select row</div></div>';
		root.getElementById("dialog-action").addEventListener("click", () => window.dialogClicks++);
	</script>`
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx, chromedp.Navigate(dataURL)); err != nil {
		t.Fatal(err)
	}

	// Verbatim PT-C018 failure boundary: the old global text resolver chooses
	// the same-labeled background control that appears first in the document.
	globalNodeID, err := ResolveTextToNodeIDInFrame(ctx, "", "Content contains")
	if err != nil {
		t.Fatal(err)
	}
	var globalID string
	if err := callFunctionOnNodeForTest(ctx, globalNodeID, `function() { return this.id; }`, &globalID); err != nil {
		t.Fatal(err)
	}
	if globalID != "background-action" {
		t.Fatalf("unscoped reproduction resolved %q, want background-action", globalID)
	}

	frameTree, err := FetchFrameTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	frameIDs := frameIDs(frameTree)
	if len(frameIDs) < 2 {
		t.Fatalf("frame ids = %v, want top frame plus child", frameIDs)
	}
	childFrameID := frameIDs[1]
	frameBackgroundNodeID, err := ResolveTextToNodeIDInFrame(ctx, childFrameID, "Content contains")
	if err != nil {
		t.Fatal(err)
	}
	var frameBackgroundID string
	if err := callFunctionOnNodeForTest(ctx, frameBackgroundNodeID, `function() { return this.id; }`, &frameBackgroundID); err != nil {
		t.Fatal(err)
	}
	if frameBackgroundID != "frame-background" {
		t.Fatalf("frame precondition resolved %q, want frame-background", frameBackgroundID)
	}

	modalNodeID, open, err := TopmostModalNodeID(ctx, childFrameID)
	if err != nil || !open {
		t.Fatalf("topmost dialog = (%d, %v, %v), want visible dialog", modalNodeID, open, err)
	}
	var modalID string
	if err := callFunctionOnNodeForTest(ctx, modalNodeID, `function() { return this.id; }`, &modalID); err != nil {
		t.Fatal(err)
	}
	if modalID != "shadow-top" {
		t.Fatalf("topmost dialog id = %q, want shadow-top", modalID)
	}

	scopedNodeID, err := ResolveUnifiedSelectorWithinNode(ctx, selector.Parse("text:Content contains"), nil, modalNodeID)
	if err != nil {
		t.Fatal(err)
	}
	var scopedID string
	if err := callFunctionOnNodeForTest(ctx, scopedNodeID, `function() { return this.id; }`, &scopedID); err != nil {
		t.Fatal(err)
	}
	if scopedID != "dialog-action" {
		t.Fatalf("dialog-scoped text resolved %q, want dialog-action", scopedID)
	}
	if err := JSClickByBackendNode(ctx, scopedNodeID); err != nil {
		t.Fatal(err)
	}
	var clickState struct {
		Background int `json:"background"`
		Dialog     int `json:"dialog"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({background: window.backgroundClicks, dialog: window.dialogClicks})`, &clickState)); err != nil {
		t.Fatal(err)
	}
	if clickState.Background != 0 || clickState.Dialog != 1 {
		t.Fatalf("click state = %+v, want only dialog action", clickState)
	}

	backgroundRef := RefCache{Targets: map[string]RefTarget{"e0": {BackendNodeID: globalNodeID}, "e1": {BackendNodeID: scopedNodeID}}}
	if _, err := ResolveUnifiedSelectorWithinNode(ctx, selector.Parse("ref:e0"), &backgroundRef, modalNodeID); !errors.Is(err, ErrSelectorOutsideScope) {
		t.Fatalf("background ref error = %v, want ErrSelectorOutsideScope", err)
	}
	if got, err := ResolveUnifiedSelectorWithinNode(ctx, selector.Parse("ref:e1"), &backgroundRef, modalNodeID); err != nil || got != scopedNodeID {
		t.Fatalf("dialog ref = (%d, %v), want %d", got, err, scopedNodeID)
	}

	var dialogText string
	if err := callFunctionOnNodeForTest(ctx, modalNodeID, `function() { return this.innerText; }`, &dialogText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dialogText, "Dialog option") || strings.Contains(dialogText, "Background policy grid") {
		t.Fatalf("dialog text leaked background content: %q", dialogText)
	}
	rawNodes, err := FetchAXTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	flat, _ := BuildSnapshot(FilterSubtree(rawNodes, modalNodeID), "", -1)
	var snapshotText strings.Builder
	for _, node := range flat {
		snapshotText.WriteString(node.Name)
		snapshotText.WriteByte('\n')
	}
	if got := snapshotText.String(); !strings.Contains(got, "Dialog option") || strings.Contains(got, "Background policy grid") {
		t.Fatalf("dialog snapshot leaked background content: %q", got)
	}
}

func TestTopmostModalUsesBrowserPaintOrderAndRejectsFalseOwners(t *testing.T) {
	chromePath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}
	profile, err := os.MkdirTemp("", "pinchtab-modal-order-")
	if err != nil {
		t.Fatal(err)
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 20*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
		_ = os.RemoveAll(profile)
	})

	navigate := func(html string) {
		t.Helper()
		dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
		if err := chromedp.Run(ctx, chromedp.Navigate(dataURL)); err != nil {
			t.Fatal(err)
		}
	}
	modalID := func() string {
		t.Helper()
		nodeID, open, err := TopmostModalNodeID(ctx, "")
		if err != nil || !open {
			t.Fatalf("topmost dialog = (%d, %v, %v), want one", nodeID, open, err)
		}
		var id string
		if err := callFunctionOnNodeForTest(ctx, nodeID, `function() { return this.id; }`, &id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	t.Run("native top layer follows showModal order", func(t *testing.T) {
		navigate(`<style>dialog { width:300px; height:180px; padding:0 }</style>
			<dialog id="first">first</dialog><dialog id="second">second</dialog>
			<script>second.showModal(); first.showModal();</script>`)
		if got := modalID(); got != "first" {
			t.Fatalf("topmost dialog = %q, want last showModal() dialog first", got)
		}
	})

	t.Run("nested inner owns interaction despite outer z index", func(t *testing.T) {
		navigate(`<div id="outer" role="dialog" aria-modal="true"
			style="position:fixed;inset:0;z-index:100;background:white">
			<div id="inner" role="dialog" aria-modal="true"
				style="position:absolute;inset:40px;background:white">inner</div>
		</div>`)
		if got := modalID(); got != "inner" {
			t.Fatalf("topmost dialog = %q, want nested inner", got)
		}
	})

	t.Run("modeless and offscreen dialogs do not own scope", func(t *testing.T) {
		navigate(`<dialog id="modeless" open>modeless</dialog>
			<div id="offscreen" role="dialog" aria-modal="true"
				style="position:fixed;left:-10000px;top:0;width:100px;height:100px">offscreen</div>
			<div id="live" role="dialog" aria-modal="true"
				style="position:fixed;inset:20px;background:white">live</div>`)
		if got := modalID(); got != "live" {
			t.Fatalf("topmost dialog = %q, want live", got)
		}
	})

	t.Run("main world DOM poisoning cannot hide modal", func(t *testing.T) {
		navigate(`<div id="guarded" role="dialog" aria-modal="true"
			style="position:fixed;inset:20px;background:white">guarded</div>
			<script>
				document.querySelectorAll = () => [];
				Document.prototype.querySelectorAll = () => [];
				Element.prototype.getBoundingClientRect = () => ({left:-1e4,top:0,right:-9e3,bottom:1,width:1,height:1});
			</script>`)
		if got := modalID(); got != "guarded" {
			t.Fatalf("topmost dialog = %q, want guarded", got)
		}
	})
}

func TestDialogScopeContainmentIncludesOpenShadowDescendants(t *testing.T) {
	chromePath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}
	profile, err := os.MkdirTemp("", "pinchtab-modal-shadow-containment-")
	if err != nil {
		t.Fatal(err)
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 20*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
		_ = os.RemoveAll(profile)
	})

	html := `<div id="dialog" role="dialog" aria-modal="true"
		style="position:fixed;inset:20px;background:white"><div id="host"></div></div>
		<script>host.attachShadow({mode:'open'}).innerHTML='<button id="shadow-button">Save</button>'</script>`
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html)))); err != nil {
		t.Fatal(err)
	}
	modalNodeID, open, err := TopmostModalNodeID(ctx, "")
	if err != nil || !open {
		t.Fatalf("topmost dialog = (%d, %v, %v)", modalNodeID, open, err)
	}
	buttonNodeID, err := ResolveCSSToNodeID(ctx, "#shadow-button")
	if err != nil {
		t.Fatal(err)
	}
	inside, err := BackendNodeWithinScope(ctx, modalNodeID, buttonNodeID)
	if err != nil || !inside {
		t.Fatalf("shadow button containment = (%v, %v), want true", inside, err)
	}
}

// callFunctionOnNodeForTest keeps the real-browser assertions on the same CDP
// primitive used by the handler without constructing a full Bridge instance.
func callFunctionOnNodeForTest(ctx context.Context, backendNodeID int64, fn string, result any) error {
	return new(Bridge).CallFunctionOnNode(ctx, backendNodeID, fn, nil, result)
}
