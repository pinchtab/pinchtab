package bridge

import (
	"context"
	"encoding/base64"
	"os/exec"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestClickPreservesFloatingFlyoutWithoutScroll(t *testing.T) {
	chromePath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 15*time.Second)
	defer cancelTimeout()

	html := `<style>
		#portal { position: fixed; inset: 10px auto auto 10px; }
		#scroller { height: 50px; width: 200px; overflow: auto; }
		#spacer { height: 200px; }
	</style>
	<div id="portal" role="menu">
		<div id="scroller"><div id="spacer"></div><button id="item" role="menuitem">Content contains</button></div>
	</div>
	<script>
		window.selected = "";
		window.flyoutDetached = 0;
		document.getElementById("item").addEventListener("click", () => window.selected = "Content contains");
		document.getElementById("scroller").addEventListener("scroll", () => {
			window.flyoutDetached++;
			document.getElementById("portal").remove();
		}, {once: true});
	</script>`
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx, chromedp.Navigate(dataURL)); err != nil {
		t.Fatal(err)
	}
	node, err := firstNodeBySelector(ctx, "#item")
	if err != nil {
		t.Fatal(err)
	}
	b := New(context.Background(), nil, &config.RuntimeConfig{})
	if _, err := b.Actions[ActionClick](ctx, ActionRequest{NodeID: int64(node.BackendNodeID)}); err != nil {
		t.Fatalf("click floating menu item: %v", err)
	}

	var state struct {
		Selected string `json:"selected"`
		Detached int    `json:"detached"`
		Present  bool   `json:"present"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({
		selected: window.selected,
		detached: window.flyoutDetached,
		present: !!document.getElementById("portal")
	})`, &state)); err != nil {
		t.Fatal(err)
	}
	if state.Selected != "Content contains" || state.Detached != 0 || !state.Present {
		t.Fatalf("state after click = %+v, want intended selection with flyout preserved", state)
	}
}

func TestDefaultClickUsesNoScrollPathForFloatingFlyout(t *testing.T) {
	origFlyout := clickFloatingFlyoutItemAction
	origPointer := clickByNodeIDAction
	origDOM := jsClickByBackendNodeAction
	t.Cleanup(func() {
		clickFloatingFlyoutItemAction = origFlyout
		clickByNodeIDAction = origPointer
		jsClickByBackendNodeAction = origDOM
	})

	clickFloatingFlyoutItemAction = func(context.Context, int64) (bool, error) { return true, nil }
	clickByNodeIDAction = func(context.Context, int64) error {
		t.Fatal("floating flyout item reached scrolling pointer path")
		return nil
	}
	jsClickByBackendNodeAction = func(context.Context, int64) error {
		t.Fatal("atomic flyout click should not be repeated")
		return nil
	}

	if err := clickByNodeIDWithMode(context.Background(), 42, "default"); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultClickKeepsPointerPathForOrdinaryNode(t *testing.T) {
	origFlyout := clickFloatingFlyoutItemAction
	origPointer := clickByNodeIDAction
	t.Cleanup(func() {
		clickFloatingFlyoutItemAction = origFlyout
		clickByNodeIDAction = origPointer
	})

	clickFloatingFlyoutItemAction = func(context.Context, int64) (bool, error) { return false, nil }
	pointerCalled := false
	clickByNodeIDAction = func(context.Context, int64) error {
		pointerCalled = true
		return nil
	}

	if err := clickByNodeIDWithMode(context.Background(), 42, "default"); err != nil {
		t.Fatal(err)
	}
	if !pointerCalled {
		t.Fatal("ordinary node did not use pointer path")
	}
}
