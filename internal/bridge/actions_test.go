package bridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func TestClickAction_UsesCoordinatePathIncludingZeroZero(t *testing.T) {
	t.Parallel()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.Actions[ActionClick](ctx, ActionRequest{HasXY: true, X: 0, Y: 0})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected coordinate path, got selector/ref validation error: %v", err)
	}
}

func TestDoubleClickAction_UsesCoordinatePathIncludingZeroZero(t *testing.T) {
	t.Parallel()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.Actions[ActionDoubleClick](ctx, ActionRequest{HasXY: true, X: 0, Y: 0})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected coordinate path, got selector/ref validation error: %v", err)
	}
}

func TestHoverAction_UsesCoordinatePath(t *testing.T) {
	t.Parallel()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.Actions[ActionHover](ctx, ActionRequest{HasXY: true, X: 12.5, Y: 34.5})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected coordinate path, got selector/ref validation error: %v", err)
	}
}

func TestMouseMoveAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionMouseMove]; !ok {
		t.Fatal("ActionMouseMove not registered in action registry")
	}
}

func TestMouseDownAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionMouseDown]; !ok {
		t.Fatal("ActionMouseDown not registered in action registry")
	}
}

func TestMouseUpAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionMouseUp]; !ok {
		t.Fatal("ActionMouseUp not registered in action registry")
	}
}

func TestMouseWheelAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionMouseWheel]; !ok {
		t.Fatal("ActionMouseWheel not registered in action registry")
	}
}

func TestMouseDownAction_UsesCoordinatePath(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.Actions[ActionMouseDown](ctx, ActionRequest{HasXY: true, X: 0, Y: 0, Button: "right"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected coordinate path, got selector/ref validation error: %v", err)
	}
}

func TestMouseUpAction_UsesCoordinatePath(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.Actions[ActionMouseUp](ctx, ActionRequest{HasXY: true, X: 0, Y: 0})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected coordinate path, got selector/ref validation error: %v", err)
	}
}

func TestMouseWheelAction_UsesExplicitWheelDeltas(t *testing.T) {
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops

	called := false
	ops.scrollByCoordinate = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) (bridgecdpops.ScrollPosition, error) {
		called = true
		if x != 50 || y != 75 {
			t.Fatalf("wheel coordinates = (%v, %v), want (50, 75)", x, y)
		}
		if deltaX != 123 || deltaY != -456 {
			t.Fatalf("wheel delta = (%d, %d), want (123, -456)", deltaX, deltaY)
		}
		return bridgecdpops.ScrollPosition{}, nil
	}
	ops.scrollViewportCenter = func(context.Context) (float64, float64, error) {
		t.Fatal("viewport center should not be used when explicit coordinates are provided")
		return 0, 0, nil
	}

	res, err := b.Actions[ActionMouseWheel](context.Background(), ActionRequest{
		HasXY:  true,
		X:      50,
		Y:      75,
		DeltaX: 123,
		DeltaY: -456,
	})
	if err != nil {
		t.Fatalf("mouse wheel returned error: %v", err)
	}
	if !called {
		t.Fatal("expected wheel path to be used")
	}
	if !res["wheel"].(bool) {
		t.Fatalf("expected wheel=true in result payload, got %#v", res)
	}
}

func TestClickAction_ForwardsModifiers(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops

	var gotModifiers int
	called := false
	ops.clickByCoordinate = func(ctx context.Context, x, y float64, modifiers int) error {
		called = true
		gotModifiers = modifiers
		return nil
	}

	// Shift+click from the screencast UI: modifier bitmask 8 must reach the
	// CDP pointer dispatch so the page sees a held Shift.
	if _, err := b.Actions[ActionClick](context.Background(), ActionRequest{
		HasXY:     true,
		X:         40,
		Y:         60,
		Modifiers: 8,
	}); err != nil {
		t.Fatalf("click returned error: %v", err)
	}
	if !called {
		t.Fatal("expected coordinate click path to be used")
	}
	if gotModifiers != 8 {
		t.Fatalf("click modifiers = %d, want 8 (Shift)", gotModifiers)
	}
}

func TestMouseWheelAction_ForwardsModifiers(t *testing.T) {
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops

	var gotModifiers int
	ops.scrollByCoordinate = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) (bridgecdpops.ScrollPosition, error) {
		gotModifiers = modifiers
		return bridgecdpops.ScrollPosition{}, nil
	}

	// Shift+wheel (horizontal scroll intent): the bitmask must reach the wheel
	// dispatch.
	if _, err := b.Actions[ActionMouseWheel](context.Background(), ActionRequest{
		HasXY:     true,
		X:         10,
		Y:         20,
		DeltaY:    120,
		Modifiers: 8,
	}); err != nil {
		t.Fatalf("mouse wheel returned error: %v", err)
	}
	if gotModifiers != 8 {
		t.Fatalf("wheel modifiers = %d, want 8 (Shift)", gotModifiers)
	}
}

func TestMouseActions_TrackCurrentPointerPosition(t *testing.T) {
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops

	moveCalled := false
	upCalled := false
	ops.mouseMoveByCoordinate = func(ctx context.Context, x, y float64) error {
		moveCalled = true
		if x != 15 || y != 25 {
			t.Fatalf("move coordinates = (%v, %v), want (15, 25)", x, y)
		}
		return nil
	}
	ops.mouseUpByCoordinate = func(ctx context.Context, x, y float64, button string, modifiers int) error {
		upCalled = true
		if x != 15 || y != 25 {
			t.Fatalf("up coordinates = (%v, %v), want (15, 25)", x, y)
		}
		if button != "left" {
			t.Fatalf("button = %q, want left", button)
		}
		return nil
	}

	if _, err := b.Actions[ActionMouseMove](context.Background(), ActionRequest{
		TabID: "tab1",
		HasXY: true,
		X:     15,
		Y:     25,
	}); err != nil {
		t.Fatalf("mouse move returned error: %v", err)
	}
	if _, err := b.Actions[ActionMouseUp](context.Background(), ActionRequest{TabID: "tab1"}); err != nil {
		t.Fatalf("mouse up returned error: %v", err)
	}
	if !moveCalled || !upCalled {
		t.Fatalf("expected move and up actions to be called, got move=%v up=%v", moveCalled, upCalled)
	}
}

func TestMouseDownAction_UsesTrackedPointerWhenTargetMissing(t *testing.T) {
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops
	b.rememberPointerPosition("tab-current", 33, 44)

	ops.mouseDownByCoordinate = func(ctx context.Context, x, y float64, button string, modifiers int) error {
		if x != 33 || y != 44 {
			t.Fatalf("down coordinates = (%v, %v), want (33, 44)", x, y)
		}
		if button != "right" {
			t.Fatalf("button = %q, want right", button)
		}
		return nil
	}

	if _, err := b.Actions[ActionMouseDown](context.Background(), ActionRequest{
		TabID:  "tab-current",
		Button: "right",
	}); err != nil {
		t.Fatalf("mouse down returned error: %v", err)
	}
}

func TestMouseWheelAction_UsesViewportCenterWhenPointerMissing(t *testing.T) {
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops

	ops.scrollViewportCenter = func(context.Context) (float64, float64, error) {
		return 300, 200, nil
	}
	called := false
	ops.scrollByCoordinate = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) (bridgecdpops.ScrollPosition, error) {
		called = true
		if x != 300 || y != 200 {
			t.Fatalf("wheel coordinates = (%v, %v), want (300, 200)", x, y)
		}
		if deltaX != 0 || deltaY != 120 {
			t.Fatalf("wheel delta = (%d, %d), want (0, 120)", deltaX, deltaY)
		}
		return bridgecdpops.ScrollPosition{}, nil
	}
	if _, err := b.Actions[ActionMouseWheel](context.Background(), ActionRequest{TabID: "tab-missing"}); err != nil {
		t.Fatalf("unexpected wheel error: %v", err)
	}
	if !called {
		t.Fatal("expected wheel action to use viewport center fallback")
	}
}

func TestActionRequestUnmarshal_UsesCanonicalMouseFields(t *testing.T) {
	var req ActionRequest
	if err := json.Unmarshal([]byte(`{"kind":"mouse-wheel","x":0,"y":0,"deltaX":12,"deltaY":-34}`), &req); err != nil {
		t.Fatalf("unmarshal action request: %v", err)
	}
	if req.Kind != ActionMouseWheel {
		t.Fatalf("kind = %q, want %q", req.Kind, ActionMouseWheel)
	}
	if !req.HasXY {
		t.Fatal("expected HasXY=true when x/y keys are present")
	}
	if req.DeltaX != 12 || req.DeltaY != -34 {
		t.Fatalf("wheel deltas = (%d, %d), want (12, -34)", req.DeltaX, req.DeltaY)
	}
}

//go:fix inline
func boolPtr(v bool) *bool {
	return new(v)
}

func TestEffectiveHumanizePrecedence(t *testing.T) {
	tests := []struct {
		name       string
		config     *config.RuntimeConfig
		req        ActionRequest
		want       bool
		justifying string
	}{
		{
			name:       "default false",
			config:     &config.RuntimeConfig{},
			want:       false,
			justifying: "raw input remains the fast default",
		},
		{
			name:       "config true",
			config:     &config.RuntimeConfig{Humanize: true},
			want:       true,
			justifying: "instance default opt-in enables humanized input",
		},
		{
			name:       "request true overrides config false",
			config:     &config.RuntimeConfig{Humanize: false},
			req:        ActionRequest{Humanize: new(true)},
			want:       true,
			justifying: "per-request override can opt in",
		},
		{
			name:       "request false overrides config true",
			config:     &config.RuntimeConfig{Humanize: true},
			req:        ActionRequest{Humanize: new(false)},
			want:       false,
			justifying: "per-request override can force raw input",
		},
		{
			name:       "nil bridge config defaults false",
			config:     nil,
			want:       false,
			justifying: "nil config stays safe and fast",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &Bridge{Config: tc.config}
			if got := b.effectiveHumanize(tc.req); got != tc.want {
				t.Fatalf("effectiveHumanize = %v, want %v (%s)", got, tc.want, tc.justifying)
			}
		})
	}
}

func TestRemovedHumanActionKindsAreUnknown(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	for _, kind := range []string{"humanClick", "humanType"} {
		t.Run(kind, func(t *testing.T) {
			_, err := b.ExecuteAction(context.Background(), kind, ActionRequest{Kind: kind, Text: "hi"})
			if err == nil {
				t.Fatalf("expected %s to be rejected", kind)
			}
			if !errors.Is(err, ErrUnknownAction) || !strings.Contains(err.Error(), kind) {
				t.Fatalf("expected ErrUnknownAction naming %s, got: %v", kind, err)
			}
		})
	}
}

func TestExecuteAction_ClickRejectsModeAndEffectiveHumanizeTogether(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config *config.RuntimeConfig
		req    ActionRequest
	}{
		{
			name:   "request humanize true",
			config: &config.RuntimeConfig{},
			req: ActionRequest{
				Kind:     ActionClick,
				Ref:      "e5",
				Mode:     "dom",
				Humanize: new(true),
			},
		},
		{
			name:   "instance humanize default",
			config: &config.RuntimeConfig{Humanize: true},
			req: ActionRequest{
				Kind: ActionClick,
				Ref:  "e5",
				Mode: "dispatch",
			},
		},
		{
			name:   "coordinates with instance humanize default",
			config: &config.RuntimeConfig{Humanize: true},
			req: ActionRequest{
				Kind:  ActionClick,
				HasXY: true,
				X:     10,
				Y:     20,
				Mode:  "dom",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := New(context.TODO(), nil, tc.config)
			_, err := b.ExecuteAction(context.Background(), ActionClick, tc.req)
			if err == nil {
				t.Fatal("expected error when mode and humanize are both set")
			}
			if !strings.Contains(err.Error(), "mutually exclusive") {
				t.Fatalf("expected mutually exclusive error, got: %v", err)
			}
		})
	}
}

func TestExecuteAction_ClickModeAllowedWhenHumanizeOverrideDisablesDefault(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()

	called := false
	ops.jsClickByBackendNode = func(ctx context.Context, nodeID int64) error {
		called = true
		if nodeID != 42 {
			t.Fatalf("nodeID = %d, want 42", nodeID)
		}
		return nil
	}

	b := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})
	b.pointerOps = ops
	res, err := b.ExecuteAction(context.Background(), ActionClick, ActionRequest{
		Kind:     ActionClick,
		NodeID:   42,
		Mode:     "dom",
		Humanize: new(false),
	})
	if err != nil {
		t.Fatalf("expected raw mode click to be allowed when humanize=false overrides default, got: %v", err)
	}
	if !called {
		t.Fatal("expected dom mode click path to run")
	}
	if clicked, _ := res["clicked"].(bool); !clicked {
		t.Fatalf("expected clicked result, got %#v", res)
	}
}

func TestClickAction_HumanizeOptInUsesHumanizedPath(t *testing.T) {
	t.Parallel()
	raw := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := raw.Actions[ActionClick](ctx, ActionRequest{
		Kind:     ActionClick,
		Humanize: new(false),
		HasXY:    true,
		X:        10,
		Y:        20,
	})
	if err == nil {
		t.Fatal("expected cancelled raw coordinate click to fail")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("humanize=false should force raw click coordinate path, got: %v", err)
	}

}

func TestClickAction_HumanizedCoordinateClick(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()

	var clicked [][2]float64
	ops.humanClickCoordinate = func(ctx context.Context, x, y float64) error {
		clicked = append(clicked, [2]float64{x, y})
		return nil
	}

	b := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})
	b.pointerOps = ops
	res, err := b.ExecuteAction(context.Background(), ActionClick, ActionRequest{
		Kind:  ActionClick,
		HasXY: true,
		X:     10,
		Y:     20,
	})
	if err != nil {
		t.Fatalf("humanized coordinate click returned error: %v", err)
	}
	if clicked, _ := res["clicked"].(bool); !clicked {
		t.Fatalf("expected clicked=true, got %#v", res)
	}
	if human, _ := res["human"].(bool); !human {
		t.Fatalf("expected human=true, got %#v", res)
	}
	if len(clicked) != 1 || clicked[0] != [2]float64{10, 20} {
		t.Fatalf("humanized click points = %v, want [[10 20]]", clicked)
	}
}

func TestClickAction_HumanizedRoutesThroughClickTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		req         ActionRequest
		wantNode    []cdp.BackendNodeID
		wantPoints  [][2]float64
		wantInvalid bool
	}{
		{name: "nodeId", req: ActionRequest{NodeID: 42}, wantNode: []cdp.BackendNodeID{42}},
		{name: "coordinates", req: ActionRequest{HasXY: true, X: 3, Y: 4}, wantPoints: [][2]float64{{3, 4}}},
		{name: "no target", req: ActionRequest{}, wantInvalid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ops := defaultPointerOps()
			var nodes []cdp.BackendNodeID
			var points [][2]float64
			ops.clickElement = func(ctx context.Context, id cdp.BackendNodeID) error {
				nodes = append(nodes, id)
				return nil
			}
			ops.humanClickCoordinate = func(ctx context.Context, x, y float64) error {
				points = append(points, [2]float64{x, y})
				return nil
			}
			b := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})
			b.pointerOps = ops

			req := tc.req
			req.Kind = ActionClick
			res, err := b.ExecuteAction(context.Background(), ActionClick, req)

			if tc.wantInvalid {
				if !errors.Is(err, ErrInvalidActionRequest) {
					t.Fatalf("err = %v, want invalid action request", err)
				}
			} else if err != nil {
				t.Fatalf("humanized click returned error: %v", err)
			} else if res["clicked"] != true || res["human"] != true {
				t.Fatalf("result = %#v, want clicked and human", res)
			}
			if len(nodes) != len(tc.wantNode) || (len(nodes) > 0 && nodes[0] != tc.wantNode[0]) {
				t.Fatalf("clickElement calls = %v, want %v", nodes, tc.wantNode)
			}
			if len(points) != len(tc.wantPoints) || (len(points) > 0 && points[0] != tc.wantPoints[0]) {
				t.Fatalf("humanClickCoordinate calls = %v, want %v", points, tc.wantPoints)
			}
		})
	}
}

func TestClickAction_HumanizedHonoursWaitNav(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()
	ops.humanClickCoordinate = func(ctx context.Context, x, y float64) error { return nil }
	ops.clickElement = func(ctx context.Context, backendNodeID cdp.BackendNodeID) error { return nil }

	var waits []time.Duration
	ops.waitNav = func(ctx context.Context, delay time.Duration) { waits = append(waits, delay) }

	tests := []struct {
		name      string
		req       ActionRequest
		wantWaits int
	}{
		{name: "coordinates with waitNav", req: ActionRequest{HasXY: true, X: 1, Y: 2, WaitNav: true}, wantWaits: 1},
		{name: "node with waitNav and dialog detection", req: ActionRequest{NodeID: 42, TabID: "tab-1", WaitNav: true}, wantWaits: 1},
		{name: "without waitNav", req: ActionRequest{NodeID: 42}, wantWaits: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			waits = nil
			b := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true, WaitNavDelay: 7 * time.Millisecond})
			b.pointerOps = ops
			tc.req.Kind = ActionClick
			res, err := b.ExecuteAction(context.Background(), ActionClick, tc.req)
			if err != nil {
				t.Fatalf("humanized click returned error: %v", err)
			}
			if human, _ := res["human"].(bool); !human {
				t.Fatalf("expected human=true, got %#v", res)
			}
			if len(waits) != tc.wantWaits {
				t.Fatalf("waitNav calls = %d, want %d", len(waits), tc.wantWaits)
			}
			for _, d := range waits {
				if d != 7*time.Millisecond {
					t.Fatalf("waitNav delay = %v, want 7ms", d)
				}
			}
		})
	}
}

func TestClickAction_HumanizedDialogActionArmsAutoHandler(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()

	b := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})
	b.pointerOps = ops
	dm := b.GetDialogManager()

	ops.clickElement = func(ctx context.Context, backendNodeID cdp.BackendNodeID) error {
		if backendNodeID != 42 {
			return errors.New("unexpected backend node id")
		}
		armed := dm.TakeAutoHandler("tab-dialog")
		if armed == nil {
			return errors.New("dialog auto-handler was not armed")
		}
		if armed.Action != "accept" || armed.Text != "typed response" {
			return errors.New("dialog auto-handler had wrong action or text")
		}
		return nil
	}

	res, err := b.ExecuteAction(context.Background(), ActionClick, ActionRequest{
		Kind:         ActionClick,
		TabID:        "tab-dialog",
		NodeID:       42,
		DialogAction: "accept",
		DialogText:   "typed response",
	})
	if err != nil {
		t.Fatalf("humanized click with dialogAction returned error: %v", err)
	}
	if clicked, _ := res["clicked"].(bool); !clicked {
		t.Fatalf("expected clicked result, got %#v", res)
	}
	if human, _ := res["human"].(bool); !human {
		t.Fatalf("expected human=true result, got %#v", res)
	}
	if dm.HasAutoHandler("tab-dialog") {
		t.Fatal("dialog auto-handler should be consumed or cleaned up after click")
	}
}

func TestClickByNodeIDWithJSFallback_UsesTrustedPathFirst(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()

	var trustedCalled bool
	var fallbackCalled bool
	ops.clickByNodeID = func(ctx context.Context, nodeID int64) error {
		trustedCalled = true
		if nodeID != 42 {
			t.Fatalf("nodeID = %d, want 42", nodeID)
		}
		return nil
	}
	ops.jsClickByBackendNode = func(context.Context, int64) error {
		fallbackCalled = true
		return nil
	}

	if err := ops.clickByNodeIDWithJSFallback(context.Background(), 42); err != nil {
		t.Fatalf("clickByNodeIDWithJSFallback error = %v", err)
	}
	if !trustedCalled {
		t.Fatal("trusted CDP click path was not called")
	}
	if fallbackCalled {
		t.Fatal("JS fallback should not run when trusted click succeeds")
	}
}

func TestClickByNodeIDWithJSFallback_FallsBackOnlyOnTimeout(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()

	var fallbackCalled bool
	ops.clickByNodeID = func(context.Context, int64) error {
		return context.DeadlineExceeded
	}
	ops.jsClickByBackendNode = func(context.Context, int64) error {
		fallbackCalled = true
		return nil
	}

	if err := ops.clickByNodeIDWithJSFallback(context.Background(), 42); err != nil {
		t.Fatalf("clickByNodeIDWithJSFallback error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("JS fallback should run after trusted click timeout")
	}
}

func TestClickByNodeIDWithJSFallback_DoesNotFallbackOnOtherErrors(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()

	boom := errors.New("boom")
	var fallbackCalled bool
	ops.clickByNodeID = func(context.Context, int64) error {
		return boom
	}
	ops.jsClickByBackendNode = func(context.Context, int64) error {
		fallbackCalled = true
		return nil
	}

	err := ops.clickByNodeIDWithJSFallback(context.Background(), 42)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want boom", err)
	}
	if fallbackCalled {
		t.Fatal("JS fallback should not run for non-timeout trusted click errors")
	}
}

func TestClickByNodeIDWithJSFallback_SkipsFallbackOnCancelledCtx(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()

	ctx, cancel := context.WithCancel(context.Background())
	var fallbackCalled bool
	ops.clickByNodeID = func(context.Context, int64) error {
		cancel() // simulate dialog-detection cancelling the context
		return context.DeadlineExceeded
	}
	ops.jsClickByBackendNode = func(context.Context, int64) error {
		fallbackCalled = true
		return nil
	}

	err := ops.clickByNodeIDWithJSFallback(ctx, 42)
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
	if fallbackCalled {
		t.Fatal("JS fallback should not run when parent context is cancelled")
	}
}

func TestClickSubmitUsesOneJSTransactionWithoutFallback(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()

	jsCalls := 0
	trustedCalls := 0
	flyoutCalls := 0
	ops.jsClickByBackendNode = func(_ context.Context, nodeID int64) error {
		jsCalls++
		if nodeID != 42 {
			t.Fatalf("nodeID = %d, want 42", nodeID)
		}
		return context.DeadlineExceeded
	}
	ops.clickByNodeID = func(context.Context, int64) error {
		trustedCalls++
		return nil
	}
	ops.clickFloatingFlyoutItem = func(context.Context, int64) (bool, error) {
		flyoutCalls++
		return false, nil
	}

	b := New(context.Background(), nil, &config.RuntimeConfig{Humanize: true})
	b.pointerOps = ops
	_, err := b.actionClick(context.Background(), ActionRequest{
		Kind:   ActionClick,
		NodeID: 42,
		Submit: true,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("submit click error = %v, want deadline", err)
	}
	if jsCalls != 1 || trustedCalls != 0 || flyoutCalls != 0 {
		t.Fatalf("dispatch counts = js:%d trusted:%d flyout:%d, want 1/0/0", jsCalls, trustedCalls, flyoutCalls)
	}
}

func TestValidateSubmitAction(t *testing.T) {
	trueValue := true
	tests := []struct {
		name    string
		kind    string
		req     ActionRequest
		wantErr bool
	}{
		{name: "fill unchanged", kind: ActionFill, req: ActionRequest{Submit: true}},
		{name: "click element", kind: ActionClick, req: ActionRequest{Submit: true, NodeID: 1}},
		{name: "coordinates", kind: ActionClick, req: ActionRequest{Submit: true, HasXY: true}, wantErr: true},
		{name: "wait nav", kind: ActionClick, req: ActionRequest{Submit: true, WaitNav: true}, wantErr: true},
		{name: "mode", kind: ActionClick, req: ActionRequest{Submit: true, Mode: "dom"}, wantErr: true},
		{name: "humanize", kind: ActionClick, req: ActionRequest{Submit: true, Humanize: &trueValue}, wantErr: true},
		{name: "other action", kind: ActionType, req: ActionRequest{Submit: true}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSubmitAction(tt.kind, tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateSubmitAction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTypeAction_HumanizeOptInUsesHumanizedPath(t *testing.T) {
	raw := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := raw.Actions[ActionType](ctx, ActionRequest{
		Kind:     ActionType,
		Selector: "#name",
		Text:     "hi",
		Humanize: new(false),
	})
	if err == nil {
		t.Fatal("expected cancelled raw type to fail")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("humanize=false should force raw type path, got: %v", err)
	}

	humanized := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err = humanized.Actions[ActionType](context.Background(), ActionRequest{
		Kind:     ActionType,
		Text:     "hi",
		Humanize: new(true),
	})
	if err == nil {
		t.Fatal("expected humanized targetless type to fail")
	}
	if !strings.Contains(err.Error(), "need selector, ref, or nodeId") {
		t.Fatalf("humanized type should require selector/ref/nodeId, got: %v", err)
	}
}

func TestScrollAction_UsesCoordinateWheelPath(t *testing.T) {
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops

	called := false
	ops.scrollByCoordinate = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) (bridgecdpops.ScrollPosition, error) {
		called = true
		if x != 12.5 || y != 34.5 {
			t.Fatalf("wheel coordinates = (%v, %v), want (12.5, 34.5)", x, y)
		}
		if deltaX != 0 || deltaY != 50 {
			t.Fatalf("wheel delta = (%d, %d), want (0, 50)", deltaX, deltaY)
		}
		return bridgecdpops.ScrollPosition{X: 3, Y: 40, Moved: true}, nil
	}
	ops.scrollViewportCenter = func(context.Context) (float64, float64, error) {
		t.Fatal("viewport center should not be used when explicit coordinates are provided")
		return 0, 0, nil
	}

	result, err := b.Actions[ActionScroll](context.Background(), ActionRequest{
		HasXY:   true,
		X:       12.5,
		Y:       34.5,
		ScrollY: 50,
	})
	if err != nil {
		t.Fatalf("scroll returned error: %v", err)
	}
	if !called {
		t.Fatal("expected coordinate wheel path to be used")
	}
	if result["deltaX"] != 0 || result["deltaY"] != 50 {
		t.Fatalf("unexpected result payload: %#v", result)
	}
	if result["scrollX"] != 3.0 || result["scrollY"] != 40.0 || result["scrolled"] != true {
		t.Fatalf("result does not carry the scroller position: %#v", result)
	}
	for _, legacy := range []string{"x", "y"} {
		if _, ok := result[legacy]; ok {
			t.Fatalf("result still carries the legacy %q key: %#v", legacy, result)
		}
	}
}

func TestScrollAction_UsesViewportCenterWhenCoordinatesMissing(t *testing.T) {
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops

	ops.scrollViewportCenter = func(context.Context) (float64, float64, error) {
		return 400, 300, nil
	}

	called := false
	ops.scrollByCoordinate = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) (bridgecdpops.ScrollPosition, error) {
		called = true
		if x != 400 || y != 300 {
			t.Fatalf("wheel coordinates = (%v, %v), want (400, 300)", x, y)
		}
		if deltaX != 0 || deltaY != 120 {
			t.Fatalf("wheel delta = (%d, %d), want (0, 120)", deltaX, deltaY)
		}
		return bridgecdpops.ScrollPosition{}, nil
	}

	result, err := b.Actions[ActionScroll](context.Background(), ActionRequest{})
	if err != nil {
		t.Fatalf("scroll returned error: %v", err)
	}
	if !called {
		t.Fatal("expected viewport-center wheel path to be used")
	}
	if result["deltaX"] != 0 || result["deltaY"] != 120 {
		t.Fatalf("unexpected result payload: %#v", result)
	}
	if result["scrolled"] != false {
		t.Fatalf("a scroller that did not move reported scrolled: %#v", result)
	}
	if result["targetX"] != 400.0 || result["targetY"] != 300.0 {
		t.Fatalf("unexpected scroll target payload: %#v", result)
	}
}

func TestScrollAction_PropagatesViewportCenterError(t *testing.T) {
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops

	ops.scrollViewportCenter = func(context.Context) (float64, float64, error) {
		return 0, 0, context.Canceled
	}
	ops.scrollByCoordinate = func(context.Context, float64, float64, int, int, int) (bridgecdpops.ScrollPosition, error) {
		t.Fatal("wheel dispatch should not be called when viewport center resolution fails")
		return bridgecdpops.ScrollPosition{}, nil
	}

	_, err := b.Actions[ActionScroll](context.Background(), ActionRequest{})
	if err == nil {
		t.Fatal("expected error when viewport center resolution fails")
	}
	if !strings.Contains(err.Error(), "resolve scroll viewport center") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionCheck]; !ok {
		t.Fatal("ActionCheck not registered in action registry")
	}
}

func TestUncheckAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionUncheck]; !ok {
		t.Fatal("ActionUncheck not registered in action registry")
	}
}

func TestCheckAction_RequiresSelectorOrRef(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err := b.Actions[ActionCheck](context.Background(), ActionRequest{})
	if err == nil {
		t.Fatal("expected error when no selector/ref/nodeId provided")
	}
	if !strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected 'need selector' error, got: %v", err)
	}
}

func TestUncheckAction_RequiresSelectorOrRef(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err := b.Actions[ActionUncheck](context.Background(), ActionRequest{})
	if err == nil {
		t.Fatal("expected error when no selector/ref/nodeId provided")
	}
	if !strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected 'need selector' error, got: %v", err)
	}
}

func TestCheckAction_WithNodeID_UsesResolveNode(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.Actions[ActionCheck](ctx, ActionRequest{NodeID: 42})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	// Should NOT be a validation error — it should attempt the CDP path
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected CDP path, got validation error: %v", err)
	}
}

func TestUncheckAction_WithSelector_UsesCSSPath(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.Actions[ActionUncheck](ctx, ActionRequest{Selector: "#my-checkbox"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected CSS path, got validation error: %v", err)
	}
}

func TestFinishFillSubmitIsOptInAndDispatchesEnter(t *testing.T) {
	result := map[string]any{"filled": true}
	got, err := finishFill(context.Background(), result, false)
	if err != nil || got["submitted"] != nil {
		t.Fatalf("non-submit fill = (%v, %v), want unchanged result", got, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := finishFill(ctx, map[string]any{"filled": true}, true); err == nil || !strings.Contains(err.Error(), "submit filled field") {
		t.Fatalf("submit fill error = %v, want Enter dispatch attempt", err)
	}
}

func TestKeyboardTypeAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionKeyboardType]; !ok {
		t.Fatal("ActionKeyboardType not registered in action registry")
	}
}

func TestKeyboardInsertAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionKeyboardInsert]; !ok {
		t.Fatal("ActionKeyboardInsert not registered in action registry")
	}
}

func TestKeyDownAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionKeyDown]; !ok {
		t.Fatal("ActionKeyDown not registered in action registry")
	}
}

func TestKeyUpAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionKeyUp]; !ok {
		t.Fatal("ActionKeyUp not registered in action registry")
	}
}

func TestKeyboardTypeAction_RequiresText(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err := b.Actions[ActionKeyboardType](context.Background(), ActionRequest{})
	if err == nil {
		t.Fatal("expected error when text is empty")
	}
	if !strings.Contains(err.Error(), "text required") {
		t.Fatalf("expected 'text required' error, got: %v", err)
	}
}

func TestKeyboardInsertAction_RequiresText(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err := b.Actions[ActionKeyboardInsert](context.Background(), ActionRequest{})
	if err == nil {
		t.Fatal("expected error when text is empty")
	}
	if !strings.Contains(err.Error(), "text required") {
		t.Fatalf("expected 'text required' error, got: %v", err)
	}
}

func TestKeyDownAction_RequiresKey(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err := b.Actions[ActionKeyDown](context.Background(), ActionRequest{})
	if err == nil {
		t.Fatal("expected error when key is empty")
	}
	if !strings.Contains(err.Error(), "key required") {
		t.Fatalf("expected 'key required' error, got: %v", err)
	}
}

func TestParsePressChord(t *testing.T) {
	tests := []struct {
		input         string
		wantKey       string
		wantModifiers int
		wantChord     bool
		wantError     string
	}{
		{input: "Enter", wantKey: "Enter"},
		{input: "+", wantKey: "+"},
		{input: "Control+A", wantKey: "A", wantModifiers: 2, wantChord: true},
		{input: "Ctrl+Shift+ArrowLeft", wantKey: "ArrowLeft", wantModifiers: 10, wantChord: true},
		{input: "Cmd+C", wantKey: "C", wantModifiers: 4, wantChord: true},
		{input: "Banana+A", wantChord: true, wantError: "invalid press chord modifier"},
		{input: "Ctrl+", wantChord: true, wantError: "invalid press chord"},
		{input: "Ctrl+Control+A", wantChord: true, wantError: "duplicate press chord modifier"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			key, modifiers, chord, err := parsePressChord(tt.input)
			if key != tt.wantKey || modifiers != tt.wantModifiers || chord != tt.wantChord {
				t.Fatalf("parsePressChord(%q) = (%q, %d, %v), want (%q, %d, %v)",
					tt.input, key, modifiers, chord, tt.wantKey, tt.wantModifiers, tt.wantChord)
			}
			if tt.wantError == "" && err != nil {
				t.Fatalf("parsePressChord(%q) unexpected error: %v", tt.input, err)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Fatalf("parsePressChord(%q) error = %v, want %q", tt.input, err, tt.wantError)
			}
		})
	}
}

func TestPressChordRejectsAmbiguousModifierInputsBeforeDispatch(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err := b.Actions[ActionPress](context.Background(), ActionRequest{
		Key: "Control+A", Modifiers: 2,
	})
	if err == nil || !strings.Contains(err.Error(), "also supplied modifiers") {
		t.Fatalf("press ambiguous chord error = %v", err)
	}
}

func TestKeyUpAction_RequiresKey(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err := b.Actions[ActionKeyUp](context.Background(), ActionRequest{})
	if err == nil {
		t.Fatal("expected error when key is empty")
	}
	if !strings.Contains(err.Error(), "key required") {
		t.Fatalf("expected 'key required' error, got: %v", err)
	}
}

func TestKeyboardTypeAction_WithCancelledContext(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.Actions[ActionKeyboardType](ctx, ActionRequest{Text: "hello"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestKeyboardInsertAction_WithCancelledContext(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.Actions[ActionKeyboardInsert](ctx, ActionRequest{Text: "hello"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestKeyDownAction_WithCancelledContext(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.Actions[ActionKeyDown](ctx, ActionRequest{Key: "Control"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestKeyUpAction_WithCancelledContext(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.Actions[ActionKeyUp](ctx, ActionRequest{Key: "Control"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestScrollIntoViewAction_Registered(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if _, ok := b.Actions[ActionScrollIntoView]; !ok {
		t.Fatal("ActionScrollIntoView not registered in action registry")
	}
}

func TestScrollIntoViewAction_RequiresSelectorOrRef(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err := b.Actions[ActionScrollIntoView](context.Background(), ActionRequest{})
	if err == nil {
		t.Fatal("expected error when no selector/ref/nodeId provided")
	}
	if !strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected 'need selector' error, got: %v", err)
	}
}

func TestScrollIntoViewAction_WithNodeID_UsesCDPPath(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.Actions[ActionScrollIntoView](ctx, ActionRequest{NodeID: 42})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected CDP path, got validation error: %v", err)
	}
}

func TestScrollIntoViewAction_WithSelector_UsesCSSPath(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.Actions[ActionScrollIntoView](ctx, ActionRequest{Selector: "#footer"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if strings.Contains(err.Error(), "need selector") {
		t.Fatalf("expected CSS path, got validation error: %v", err)
	}
}

func TestHoverAction_HumanizeRoutesEveryTargetForm(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()

	var nodes []cdp.BackendNodeID
	var points [][2]float64
	ops.hoverElement = func(ctx context.Context, backendNodeID cdp.BackendNodeID) error {
		nodes = append(nodes, backendNodeID)
		return nil
	}
	ops.hoverCoordinate = func(ctx context.Context, x, y float64) error {
		points = append(points, [2]float64{x, y})
		return nil
	}

	tests := []struct {
		name       string
		config     *config.RuntimeConfig
		req        ActionRequest
		wantNodes  []cdp.BackendNodeID
		wantPoints [][2]float64
		wantHuman  bool
	}{
		{
			name:      "nodeId with request opt-in",
			config:    &config.RuntimeConfig{},
			req:       ActionRequest{NodeID: 77, Humanize: new(true)},
			wantNodes: []cdp.BackendNodeID{77},
			wantHuman: true,
		},
		{
			name:       "coordinates with instance default",
			config:     &config.RuntimeConfig{Humanize: true},
			req:        ActionRequest{HasXY: true, X: 12, Y: 34},
			wantPoints: [][2]float64{{12, 34}},
			wantHuman:  true,
		},
		{
			name:      "explicit false opts out of the humanized path",
			config:    &config.RuntimeConfig{Humanize: true},
			req:       ActionRequest{NodeID: 77, Humanize: new(false)},
			wantHuman: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodes, points = nil, nil
			b := New(context.TODO(), nil, tc.config)
			b.pointerOps = ops

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			res, err := b.Actions[ActionHover](ctx, tc.req)

			if tc.wantHuman {
				if err != nil {
					t.Fatalf("humanized hover returned error: %v", err)
				}
				if hovered, _ := res["hovered"].(bool); !hovered {
					t.Fatalf("result = %#v, want hovered=true", res)
				}
				if human, _ := res["human"].(bool); !human {
					t.Fatalf("result = %#v, want human=true", res)
				}
			} else if _, ok := res["human"]; ok {
				t.Fatalf("raw hover result = %#v, want no human key", res)
			}

			if len(nodes) != len(tc.wantNodes) || (len(nodes) == 1 && nodes[0] != tc.wantNodes[0]) {
				t.Fatalf("element hovers = %v, want %v", nodes, tc.wantNodes)
			}
			if len(points) != len(tc.wantPoints) || (len(points) == 1 && points[0] != tc.wantPoints[0]) {
				t.Fatalf("coordinate hovers = %v, want %v", points, tc.wantPoints)
			}
		})
	}
}

func TestHoverAction_HumanizedStillRequiresATarget(t *testing.T) {
	t.Parallel()
	b := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})

	_, err := b.Actions[ActionHover](context.Background(), ActionRequest{})
	if err == nil || !strings.Contains(err.Error(), "need selector") {
		t.Fatalf("targetless humanized hover error = %v, want the target requirement", err)
	}
}

// Browser-backed: the selector form resolves through ResolveCSSToNodeID, which
// has no browserless seam, so this is where selector routing and the real
// trail landing on the target are pinned.
func TestHoverAction_HumanizedSelectorLandsOnTheTarget(t *testing.T) {
	ctx := newActionTab(t, 30*time.Second)

	html := `<style>#target { position: absolute; top: 100px; left: 100px; width: 120px; height: 40px; }</style>
	<div id="target">hover me</div>
	<script>
		window.hovered = false;
		document.getElementById("target").addEventListener("mouseover", () => window.hovered = true);
	</script>`
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx, chromedp.Navigate(dataURL)); err != nil {
		t.Fatal(err)
	}

	b := New(context.Background(), nil, &config.RuntimeConfig{Humanize: true})
	res, err := b.Actions[ActionHover](ctx, ActionRequest{Selector: "#target"})
	if err != nil {
		t.Fatalf("humanized selector hover: %v", err)
	}
	if human, _ := res["human"].(bool); !human {
		t.Fatalf("result = %#v, want human=true", res)
	}

	var hovered bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.hovered`, &hovered)); err != nil {
		t.Fatal(err)
	}
	if !hovered {
		t.Fatal("humanized hover did not fire mouseover on the selector target")
	}
}

func newSelectorActionTab(t *testing.T, html string) context.Context {
	t.Helper()
	ctx := newActionTab(t, 30*time.Second)
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx, chromedp.Navigate(dataURL)); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestSelectorActions_MissingSelectorIsErrSelectorNoMatch(t *testing.T) {
	ctx := newSelectorActionTab(t, `<button id="present">x</button><input type="checkbox" id="box">`)
	b := New(context.Background(), nil, &config.RuntimeConfig{})

	for _, kind := range []string{ActionClick, ActionDoubleClick, ActionHover, ActionScroll, ActionCheck, ActionSelect} {
		req := ActionRequest{Selector: "#missing", Value: "v"}
		if _, err := b.ExecuteAction(ctx, kind, req); !errors.Is(err, ErrSelectorNoMatch) {
			t.Errorf("%s on a missing selector: err = %v, want ErrSelectorNoMatch", kind, err)
		}
	}
}

func TestSelectorActions_ClickReachesAShadowDOMTarget(t *testing.T) {
	ctx := newSelectorActionTab(t, `<div id="host"></div>
	<script>
		window.shadowClicks = 0;
		const root = document.getElementById("host").attachShadow({mode: "open"});
		root.innerHTML = '<button id="inner" style="width:120px;height:40px">inner</button>';
		root.getElementById("inner").addEventListener("click", () => window.shadowClicks++);
	</script>`)
	b := New(context.Background(), nil, &config.RuntimeConfig{})

	if _, err := b.ExecuteAction(ctx, ActionClick, ActionRequest{Selector: "#inner"}); err != nil {
		t.Fatalf("click on a shadow-DOM selector: %v", err)
	}
	var clicks int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.shadowClicks`, &clicks)); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Fatalf("shadow button clicks = %d, want 1", clicks)
	}
}

func TestClickActions_NodeIDWinsOverSelector(t *testing.T) {
	t.Parallel()
	const wantNode = int64(7)

	tests := []struct {
		name string
		kind string
		req  ActionRequest
		fake func(ops *pointerOps, record func(context.Context, int64) error)
	}{
		{
			name: "click",
			kind: ActionClick,
			req:  ActionRequest{Humanize: new(false)},
			fake: func(ops *pointerOps, record func(context.Context, int64) error) { ops.clickByNodeID = record },
		},
		{
			name: "double-click",
			kind: ActionDoubleClick,
			fake: func(ops *pointerOps, record func(context.Context, int64) error) { ops.doubleClickByNodeID = record },
		},
		{
			name: "submit click",
			kind: ActionClick,
			req:  ActionRequest{Submit: true},
			fake: func(ops *pointerOps, record func(context.Context, int64) error) { ops.jsClickByBackendNode = record },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ops := defaultPointerOps()
			ops.clickFloatingFlyoutItem = func(context.Context, int64) (bool, error) { return false, nil }
			var dispatched []int64
			tc.fake(ops, func(_ context.Context, nodeID int64) error {
				dispatched = append(dispatched, nodeID)
				return nil
			})
			b := New(context.TODO(), nil, &config.RuntimeConfig{})
			b.pointerOps = ops

			req := tc.req
			req.Kind = tc.kind
			req.NodeID = wantNode
			req.Selector = "#x"
			if _, err := b.Actions[tc.kind](context.Background(), req); err != nil {
				t.Fatalf("%s with nodeId and selector returned error: %v", tc.name, err)
			}
			if len(dispatched) != 1 || dispatched[0] != wantNode {
				t.Fatalf("%s dispatched at nodes %v, want [%d]", tc.name, dispatched, wantNode)
			}
		})
	}
}

const pressKeyPage = `<input id="name"><input id="agree" type="checkbox">`

func openPressKeyPage(t *testing.T) context.Context {
	t.Helper()
	ctx := testbrowser.Launch(t)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(cancel)
	page := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(pressKeyPage))
	if err := chromedp.Run(ctx, chromedp.Navigate(page)); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	return ctx
}

func TestPressRejectsUnknownKeyNamesWithoutTypingThem(t *testing.T) {
	ctx := openPressKeyPage(t)
	b := &Bridge{}

	for _, key := range []string{"Enterr", "Ctrl+Enterr"} {
		result, err := b.pressKey(ctx, ActionRequest{Selector: "#name", Key: key})
		if !errors.Is(err, ErrInvalidActionRequest) {
			t.Fatalf("press %q = (%v, %v), want an invalid request error", key, result, err)
		}
		for _, want := range []string{`"Enterr"`, "Escape", "Space"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("press %q error = %q, want it to mention %s", key, err, want)
			}
		}
	}
	if _, err := b.keyDown(ctx, ActionRequest{Key: "Enterr"}); !errors.Is(err, ErrInvalidActionRequest) {
		t.Errorf("keydown Enterr error = %v, want an invalid request error", err)
	}
	if got := pageValue(t, ctx, "#name"); got != "" {
		t.Fatalf("field = %q after rejected presses, want nothing typed", got)
	}

	if _, err := b.pressKey(ctx, ActionRequest{Selector: "#name", Key: "a"}); err != nil {
		t.Fatalf("press a: %v", err)
	}
	if got := pageValue(t, ctx, "#name"); got != "a" {
		t.Fatalf("field = %q after press a, want %q", got, "a")
	}

	if _, err := b.pressKey(ctx, ActionRequest{Selector: "#agree", Key: "Space"}); err != nil {
		t.Fatalf("press Space: %v", err)
	}
	var checked bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector("#agree").checked`, &checked)); err != nil {
		t.Fatalf("read checkbox: %v", err)
	}
	if !checked {
		t.Fatal("press Space on a checkbox left it unchecked")
	}
}

func TestKeyUpRejectsUnknownNamesAndEscPressesEscape(t *testing.T) {
	ctx := openPressKeyPage(t)
	b := &Bridge{}

	if _, err := b.keyUp(ctx, ActionRequest{Key: "Enterr"}); !errors.Is(err, ErrInvalidActionRequest) {
		t.Fatalf("keyup Enterr error = %v, want an invalid request error", err)
	}

	listen := `window.seen = []; document.addEventListener("keydown", e => window.seen.push(e.key)); true`
	if err := chromedp.Run(ctx, chromedp.Evaluate(listen, nil)); err != nil {
		t.Fatalf("install listener: %v", err)
	}
	if _, err := b.pressKey(ctx, ActionRequest{Selector: "#name", Key: "Esc"}); err != nil {
		t.Fatalf("press Esc: %v", err)
	}
	var seen []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.seen`, &seen)); err != nil {
		t.Fatalf("read keys: %v", err)
	}
	if len(seen) != 1 || seen[0] != "Escape" {
		t.Fatalf("press Esc dispatched keydown keys %q, want [Escape]", seen)
	}
	if got := pageValue(t, ctx, "#name"); got != "" {
		t.Fatalf("field = %q after press Esc, want nothing typed", got)
	}
}
