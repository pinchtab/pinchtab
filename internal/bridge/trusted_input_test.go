package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

type inputRecordingExecutor struct {
	methods  []string
	focusErr error
}

func (e *inputRecordingExecutor) Execute(_ context.Context, method string, _, _ any) error {
	e.methods = append(e.methods, method)
	if method == emulation.CommandSetFocusEmulationEnabled {
		return e.focusErr
	}
	return nil
}

func (e *inputRecordingExecutor) count(method string) int {
	n := 0
	for _, m := range e.methods {
		if m == method {
			n++
		}
	}
	return n
}

func TestTrustedInputEnablesFocusEmulationOncePerAction(t *testing.T) {
	exec := &inputRecordingExecutor{}
	ctx := bridgecdpops.WithTrustedInput(cdp.WithExecutor(context.Background(), exec))

	for range 20 {
		if err := bridgecdpops.MouseEventAction(map[string]any{"type": "mouseMoved", "x": 1, "y": 1}).Do(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := bridgecdpops.KeyEventAction(map[string]any{"type": "keyDown", "key": "a"}).Do(ctx); err != nil {
		t.Fatal(err)
	}
	if err := bridgecdpops.InsertTextAction("abc").Do(ctx); err != nil {
		t.Fatal(err)
	}

	if got := exec.count(emulation.CommandSetFocusEmulationEnabled); got != 1 {
		t.Errorf("focus emulation enabled %d times in one action, want once", got)
	}
	if exec.methods[0] != emulation.CommandSetFocusEmulationEnabled {
		t.Errorf("first CDP call = %s, want focus emulation before any input", exec.methods[0])
	}
	if got := exec.count("Input.dispatchMouseEvent"); got != 20 {
		t.Errorf("mouse dispatches = %d, want 20", got)
	}
}

func TestTrustedInputOutsideAnActionEnablesFocusEmulationPerDispatch(t *testing.T) {
	exec := &inputRecordingExecutor{}
	ctx := cdp.WithExecutor(context.Background(), exec)

	for range 2 {
		if err := bridgecdpops.KeyEventAction(map[string]any{"type": "keyDown", "key": "a"}).Do(ctx); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{emulation.CommandSetFocusEmulationEnabled, "Input.dispatchKeyEvent", emulation.CommandSetFocusEmulationEnabled, "Input.dispatchKeyEvent"}
	if !slices.Equal(exec.methods, want) {
		t.Errorf("CDP calls = %v, want %v", exec.methods, want)
	}
}

func TestTrustedInputDoesNotDispatchWhenFocusEmulationFails(t *testing.T) {
	exec := &inputRecordingExecutor{focusErr: errors.New("emulation refused")}
	ctx := bridgecdpops.WithTrustedInput(cdp.WithExecutor(context.Background(), exec))

	if err := bridgecdpops.InsertTextAction("abc").Do(ctx); err == nil {
		t.Fatal("dispatch reported success although focus emulation failed")
	}
	if err := bridgecdpops.InsertTextAction("abc").Do(ctx); err == nil {
		t.Fatal("a failed enable was remembered as enabled")
	}
	if got := exec.count("Input.insertText"); got != 0 {
		t.Errorf("insertText dispatched %d times without focus emulation", got)
	}
}

const trustedInputFixtureHTML = `<!doctype html><html><body style="margin:0">
<div id="hover" style="position:absolute;left:20px;top:20px;width:120px;height:60px">hover</div>
<div id="from" style="position:absolute;left:20px;top:120px;width:60px;height:60px">from</div>
<div id="to" style="position:absolute;left:220px;top:120px;width:60px;height:60px">to</div>
<input id="field" style="position:absolute;left:20px;top:220px;width:200px">
<script>
window.events = [];
for (const type of ['mouseover', 'mousemove', 'mousedown', 'mouseup', 'keydown']) {
  document.addEventListener(type, e => window.events.push({type, trusted: e.isTrusted, target: e.target.id}), true);
}
</script>
</body></html>`

const trustedInputStepDeadline = 30 * time.Second

type recordedInputEvent struct {
	Type    string `json:"type"`
	Trusted bool   `json:"trusted"`
	Target  string `json:"target"`
}

func serverCreatedTab(t *testing.T, html string) context.Context {
	t.Helper()
	browserCtx := testbrowser.Launch(t)
	tm := NewTabManager(browserCtx, &config.RuntimeConfig{}, nil, func(context.Context, string) error { return nil })
	_, tabCtx, cancelTab, err := tm.CreateTab("data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html)))
	if err != nil {
		t.Fatalf("CreateTab: %v", err)
	}
	t.Cleanup(cancelTab)
	return tabCtx
}

func inputEventsSince(t *testing.T, ctx context.Context, mark int) []recordedInputEvent {
	t.Helper()
	var events []recordedInputEvent
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.events`, &events)); err != nil {
		t.Fatal(err)
	}
	return events[mark:]
}

func switchFocusEmulationOff(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetFocusEmulationEnabled(false).Do(ctx)
	})); err != nil {
		t.Fatal(err)
	}
}

type trustedLanding struct {
	eventType string
	target    string
}

func requireTrustedLandings(t *testing.T, step string, events []recordedInputEvent, want ...trustedLanding) {
	t.Helper()
	for _, w := range want {
		if !slices.Contains(events, recordedInputEvent{Type: w.eventType, Trusted: true, Target: w.target}) {
			t.Errorf("%s: no trusted %s reached #%s; events = %+v", step, w.eventType, w.target, events)
		}
	}
}

func TestTrustedHoverDragAndTypingLandOnATabManagerCreatedTab(t *testing.T) {
	tabCtx := serverCreatedTab(t, trustedInputFixtureHTML)
	nodeID := func(css string) int64 {
		id, err := ResolveCSSToNodeID(tabCtx, css)
		if err != nil {
			t.Fatalf("resolve %s: %v", css, err)
		}
		return id
	}
	hover, field := nodeID("#hover"), nodeID("#field")

	steps := []struct {
		name string
		run  func(context.Context) error
		want []trustedLanding
	}{
		{"hover", func(ctx context.Context) error { return bridgecdpops.HoverByNodeID(ctx, hover) }, []trustedLanding{{"mouseover", "hover"}}},
		{"drag", func(ctx context.Context) error { return bridgecdpops.DragBetweenPoints(ctx, 50, 150, 250, 150, "") },
			[]trustedLanding{{"mousedown", "from"}, {"mousemove", "to"}, {"mouseup", "to"}}},
		{"typing", func(ctx context.Context) error { return TypeByNodeID(ctx, field, "ab") }, []trustedLanding{{"keydown", "field"}}},
		{"named key", func(ctx context.Context) error { return DispatchNamedKey(ctx, "Enter", 0) }, []trustedLanding{{"keydown", "field"}}},
	}
	for _, step := range steps {
		switchFocusEmulationOff(t, tabCtx)
		mark := len(inputEventsSince(t, tabCtx, 0))
		stepCtx, cancel := context.WithTimeout(tabCtx, trustedInputStepDeadline)
		start := time.Now()
		err := step.run(stepCtx)
		cancel()
		if err != nil {
			t.Fatalf("%s: %v after %v", step.name, err, time.Since(start))
		}
		requireTrustedLandings(t, step.name, inputEventsSince(t, tabCtx, mark), step.want...)
	}

	var value string
	if err := chromedp.Run(tabCtx, chromedp.Value("#field", &value, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	if value != "ab" {
		t.Errorf("typed value = %q, want ab", value)
	}
}
