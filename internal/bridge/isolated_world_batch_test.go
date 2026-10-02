package bridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const containmentFixtureHTML = `<body>
<div id="dlg" role="dialog" aria-modal="true" style="position:fixed;inset:20px;background:#fff">
<button id="inside">Confirm</button>
</div>
<button id="outside">Background</button>
</body>`

// cdpCounter counts the protocol messages the browser SENDS, per method, from
// chromedp's own debug stream. Counting at the wire is the only place the saving
// is real: the same operation still runs the same Go code either way.
type cdpCounter struct {
	mu       sync.Mutex
	counting bool
	byMethod map[string]int
}

func (c *cdpCounter) debugf(format string, args ...any) {
	if len(args) == 0 {
		return
	}
	if !strings.Contains(format, "->") {
		return
	}
	var line []byte
	switch payload := args[len(args)-1].(type) {
	case []byte:
		line = payload
	case string:
		line = []byte(payload)
	default:
		return
	}
	var msg struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(line, &msg); err != nil || msg.Method == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.counting {
		return
	}
	c.byMethod[msg.Method]++
}

func (c *cdpCounter) start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counting = true
	c.byMethod = map[string]int{}
}

func (c *cdpCounter) stop() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counting = false
	out := make(map[string]int, len(c.byMethod))
	maps.Copy(out, c.byMethod)
	return out
}

func newCountedContainmentFixture(t *testing.T) (context.Context, *cdpCounter) {
	t.Helper()

	counter := &cdpCounter{byMethod: map[string]int{}}
	alloc, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), testbrowser.DevToolsURL(t), chromedp.NoModifyURL)
	ctx, cancelTab := chromedp.NewContext(alloc, chromedp.WithDebugf(counter.debugf))
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelTab()
		cancelAlloc()
	})

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(containmentFixtureHTML))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#inside", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx, counter
}

// TestOneContainmentCheckCostsOneIsolatedContext pins the message count for the
// logical operation, filtered to the methods the check itself issues so an
// unrelated CDP call elsewhere cannot red it. The count is what stops the
// per-node resolve growing back: the frame tree read and the world creation are
// per-operation, only DOM.resolveNode is per node.
func TestOneContainmentCheckCostsOneIsolatedContext(t *testing.T) {
	ctx, counter := newCountedContainmentFixture(t)

	dialogNodeID, open, err := TopmostModalNodeID(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if !open {
		t.Fatal("fixture dialog was not detected")
	}
	insideNodeID, err := ResolveCSSToNodeID(ctx, "#inside")
	if err != nil {
		t.Fatal(err)
	}

	counter.start()
	within, err := BackendNodeWithinScope(ctx, dialogNodeID, insideNodeID)
	counted := counter.stop()
	if err != nil {
		t.Fatalf("BackendNodeWithinScope: %v", err)
	}
	if !within {
		t.Fatal("button inside the dialog reported as outside it")
	}

	want := map[string]int{
		"Page.getFrameTree":        1,
		"Page.createIsolatedWorld": 1,
		"DOM.resolveNode":          2,
		"Runtime.callFunctionOn":   1,
	}
	total := 0
	for method, wantCount := range want {
		if counted[method] != wantCount {
			t.Errorf("%s issued %d times, want %d", method, counted[method], wantCount)
		}
		total += counted[method]
	}
	if total != 5 {
		t.Errorf("one containment check issued %d of its own CDP messages, want 5 (%v)", total, counted)
	}
}

func TestBackendNodeWithinScopeStillAnswersBothWays(t *testing.T) {
	ctx, _ := newCountedContainmentFixture(t)

	dialogNodeID, open, err := TopmostModalNodeID(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if !open {
		t.Fatal("fixture dialog was not detected")
	}

	for _, tc := range []struct {
		selector string
		want     bool
	}{
		{"#inside", true},
		{"#outside", false},
		{"#dlg", true},
	} {
		nodeID, err := ResolveCSSToNodeID(ctx, tc.selector)
		if err != nil {
			t.Fatalf("resolve %s: %v", tc.selector, err)
		}
		got, err := BackendNodeWithinScope(ctx, dialogNodeID, nodeID)
		if err != nil {
			t.Fatalf("BackendNodeWithinScope(%s): %v", tc.selector, err)
		}
		if got != tc.want {
			t.Errorf("BackendNodeWithinScope(%s) = %v, want %v", tc.selector, got, tc.want)
		}
	}
}

// TestBackendNodeWithinScopeResolvesItsNodesOnce is the browserless half of the
// count: CI installs no browser, so the wire assertion above never runs there. A
// second resolve inside this function is exactly the regrowth the card is about,
// and it is visible in the AST without a browser.
func TestBackendNodeWithinScopeResolvesItsNodesOnce(t *testing.T) {
	pkg := srccensus.Load(t, ".", 70)
	body, ok := pkg.Func("BackendNodeWithinScope")
	if !ok {
		t.Fatalf("BackendNodeWithinScope is gone from %s; this guard names it", pkg.Dir())
	}

	callsWithin := func(callee string) int {
		count := 0
		for _, site := range pkg.CallsAllowingNone(callee) {
			if pkg.Contains(body, site) {
				count++
			}
		}
		return count
	}

	if n := callsWithin("cdptk.IsolatedNodeObjectIDs"); n != 1 {
		t.Errorf("BackendNodeWithinScope calls IsolatedNodeObjectIDs %d times, want exactly 1: one isolated context serves the whole check", n)
	}
	if n := callsWithin("cdptk.IsolatedNodeObjectID"); n != 0 {
		t.Errorf("BackendNodeWithinScope calls the single-node IsolatedNodeObjectID %d times; each call recomputes the top-frame context", n)
	}
}
