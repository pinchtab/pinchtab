package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestClickAction_UsesCoordinatePathIncludingZeroZero(t *testing.T) {
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
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	origScrollByCoordinate := scrollByCoordinateAction
	origScrollViewportCenter := scrollViewportCenter
	t.Cleanup(func() {
		scrollByCoordinateAction = origScrollByCoordinate
		scrollViewportCenter = origScrollViewportCenter
	})

	called := false
	scrollByCoordinateAction = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) error {
		called = true
		if x != 50 || y != 75 {
			t.Fatalf("wheel coordinates = (%v, %v), want (50, 75)", x, y)
		}
		if deltaX != 123 || deltaY != -456 {
			t.Fatalf("wheel delta = (%d, %d), want (123, -456)", deltaX, deltaY)
		}
		return nil
	}
	scrollViewportCenter = func(context.Context) (float64, float64, error) {
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
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	origClick := clickByCoordinateAction
	t.Cleanup(func() { clickByCoordinateAction = origClick })

	var gotModifiers int
	called := false
	clickByCoordinateAction = func(ctx context.Context, x, y float64, modifiers int) error {
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
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	origScroll := scrollByCoordinateAction
	t.Cleanup(func() { scrollByCoordinateAction = origScroll })

	var gotModifiers int
	scrollByCoordinateAction = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) error {
		gotModifiers = modifiers
		return nil
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
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	origMove := mouseMoveByCoordinateAction
	origUp := mouseUpByCoordinateAction
	t.Cleanup(func() {
		mouseMoveByCoordinateAction = origMove
		mouseUpByCoordinateAction = origUp
	})

	moveCalled := false
	upCalled := false
	mouseMoveByCoordinateAction = func(ctx context.Context, x, y float64) error {
		moveCalled = true
		if x != 15 || y != 25 {
			t.Fatalf("move coordinates = (%v, %v), want (15, 25)", x, y)
		}
		return nil
	}
	mouseUpByCoordinateAction = func(ctx context.Context, x, y float64, button string, modifiers int) error {
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
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.rememberPointerPosition("tab-current", 33, 44)

	origDown := mouseDownByCoordinateAction
	t.Cleanup(func() {
		mouseDownByCoordinateAction = origDown
	})

	mouseDownByCoordinateAction = func(ctx context.Context, x, y float64, button string, modifiers int) error {
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
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	origScrollByCoordinate := scrollByCoordinateAction
	origScrollViewportCenter := scrollViewportCenter
	t.Cleanup(func() {
		scrollByCoordinateAction = origScrollByCoordinate
		scrollViewportCenter = origScrollViewportCenter
	})

	scrollViewportCenter = func(context.Context) (float64, float64, error) {
		return 300, 200, nil
	}
	called := false
	scrollByCoordinateAction = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) error {
		called = true
		if x != 300 || y != 200 {
			t.Fatalf("wheel coordinates = (%v, %v), want (300, 200)", x, y)
		}
		if deltaX != 0 || deltaY != 120 {
			t.Fatalf("wheel delta = (%d, %d), want (0, 120)", deltaX, deltaY)
		}
		return nil
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

func boolPtr(v bool) *bool {
	return &v
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
			req:        ActionRequest{Humanize: boolPtr(true)},
			want:       true,
			justifying: "per-request override can opt in",
		},
		{
			name:       "request false overrides config true",
			config:     &config.RuntimeConfig{Humanize: true},
			req:        ActionRequest{Humanize: boolPtr(false)},
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
			if !strings.Contains(err.Error(), "unknown action") {
				t.Fatalf("expected unknown action error for %s, got: %v", kind, err)
			}
		})
	}
}

func TestExecuteAction_ClickRejectsModeAndEffectiveHumanizeTogether(t *testing.T) {
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
				Humanize: boolPtr(true),
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
	origJSClick := jsClickByBackendNodeAction
	t.Cleanup(func() {
		jsClickByBackendNodeAction = origJSClick
	})

	called := false
	jsClickByBackendNodeAction = func(ctx context.Context, nodeID int64) error {
		called = true
		if nodeID != 42 {
			t.Fatalf("nodeID = %d, want 42", nodeID)
		}
		return nil
	}

	b := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})
	res, err := b.ExecuteAction(context.Background(), ActionClick, ActionRequest{
		Kind:     ActionClick,
		NodeID:   42,
		Mode:     "dom",
		Humanize: boolPtr(false),
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
	raw := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := raw.Actions[ActionClick](ctx, ActionRequest{
		Kind:     ActionClick,
		Humanize: boolPtr(false),
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

	humanized := New(context.TODO(), nil, &config.RuntimeConfig{})
	_, err = humanized.Actions[ActionClick](context.Background(), ActionRequest{
		Kind:     ActionClick,
		Humanize: boolPtr(true),
		HasXY:    true,
		X:        10,
		Y:        20,
	})
	if err == nil {
		t.Fatal("expected humanized coordinate-only click to fail")
	}
	if !strings.Contains(err.Error(), "need selector") {
		t.Fatalf("humanized click should require selector/ref/nodeId, got: %v", err)
	}
}

func TestClickAction_HumanizedDialogActionArmsAutoHandler(t *testing.T) {
	origClickElement := clickElementAction
	t.Cleanup(func() {
		clickElementAction = origClickElement
	})

	b := New(context.TODO(), nil, &config.RuntimeConfig{Humanize: true})
	dm := b.GetDialogManager()

	clickElementAction = func(ctx context.Context, backendNodeID cdp.BackendNodeID) error {
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
	origTrusted := clickByNodeIDAction
	origFallback := jsClickByBackendNodeAction
	t.Cleanup(func() {
		clickByNodeIDAction = origTrusted
		jsClickByBackendNodeAction = origFallback
	})

	var trustedCalled bool
	var fallbackCalled bool
	clickByNodeIDAction = func(ctx context.Context, nodeID int64) error {
		trustedCalled = true
		if nodeID != 42 {
			t.Fatalf("nodeID = %d, want 42", nodeID)
		}
		return nil
	}
	jsClickByBackendNodeAction = func(context.Context, int64) error {
		fallbackCalled = true
		return nil
	}

	if err := clickByNodeIDWithJSFallback(context.Background(), 42); err != nil {
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
	origTrusted := clickByNodeIDAction
	origFallback := jsClickByBackendNodeAction
	t.Cleanup(func() {
		clickByNodeIDAction = origTrusted
		jsClickByBackendNodeAction = origFallback
	})

	var fallbackCalled bool
	clickByNodeIDAction = func(context.Context, int64) error {
		return context.DeadlineExceeded
	}
	jsClickByBackendNodeAction = func(context.Context, int64) error {
		fallbackCalled = true
		return nil
	}

	if err := clickByNodeIDWithJSFallback(context.Background(), 42); err != nil {
		t.Fatalf("clickByNodeIDWithJSFallback error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("JS fallback should run after trusted click timeout")
	}
}

func TestClickByNodeIDWithJSFallback_DoesNotFallbackOnOtherErrors(t *testing.T) {
	origTrusted := clickByNodeIDAction
	origFallback := jsClickByBackendNodeAction
	t.Cleanup(func() {
		clickByNodeIDAction = origTrusted
		jsClickByBackendNodeAction = origFallback
	})

	boom := errors.New("boom")
	var fallbackCalled bool
	clickByNodeIDAction = func(context.Context, int64) error {
		return boom
	}
	jsClickByBackendNodeAction = func(context.Context, int64) error {
		fallbackCalled = true
		return nil
	}

	err := clickByNodeIDWithJSFallback(context.Background(), 42)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want boom", err)
	}
	if fallbackCalled {
		t.Fatal("JS fallback should not run for non-timeout trusted click errors")
	}
}

func TestClickByNodeIDWithJSFallback_SkipsFallbackOnCancelledCtx(t *testing.T) {
	origTrusted := clickByNodeIDAction
	origFallback := jsClickByBackendNodeAction
	t.Cleanup(func() {
		clickByNodeIDAction = origTrusted
		jsClickByBackendNodeAction = origFallback
	})

	ctx, cancel := context.WithCancel(context.Background())
	var fallbackCalled bool
	clickByNodeIDAction = func(context.Context, int64) error {
		cancel() // simulate dialog-detection cancelling the context
		return context.DeadlineExceeded
	}
	jsClickByBackendNodeAction = func(context.Context, int64) error {
		fallbackCalled = true
		return nil
	}

	err := clickByNodeIDWithJSFallback(ctx, 42)
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
	if fallbackCalled {
		t.Fatal("JS fallback should not run when parent context is cancelled")
	}
}

func TestClickSubmitUsesOneJSTransactionWithoutFallback(t *testing.T) {
	origJS := jsClickByBackendNodeAction
	origTrusted := clickByNodeIDAction
	origFlyout := clickFloatingFlyoutItemAction
	t.Cleanup(func() {
		jsClickByBackendNodeAction = origJS
		clickByNodeIDAction = origTrusted
		clickFloatingFlyoutItemAction = origFlyout
	})

	jsCalls := 0
	trustedCalls := 0
	flyoutCalls := 0
	jsClickByBackendNodeAction = func(_ context.Context, nodeID int64) error {
		jsCalls++
		if nodeID != 42 {
			t.Fatalf("nodeID = %d, want 42", nodeID)
		}
		return context.DeadlineExceeded
	}
	clickByNodeIDAction = func(context.Context, int64) error {
		trustedCalls++
		return nil
	}
	clickFloatingFlyoutItemAction = func(context.Context, int64) (bool, error) {
		flyoutCalls++
		return false, nil
	}

	b := New(context.Background(), nil, &config.RuntimeConfig{Humanize: true})
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
		Humanize: boolPtr(false),
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
		Humanize: boolPtr(true),
	})
	if err == nil {
		t.Fatal("expected humanized targetless type to fail")
	}
	if !strings.Contains(err.Error(), "need selector, ref, or nodeId") {
		t.Fatalf("humanized type should require selector/ref/nodeId, got: %v", err)
	}
}

func TestScrollAction_UsesCoordinateWheelPath(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	origScrollByCoordinate := scrollByCoordinateAction
	origScrollViewportCenter := scrollViewportCenter
	t.Cleanup(func() {
		scrollByCoordinateAction = origScrollByCoordinate
		scrollViewportCenter = origScrollViewportCenter
	})

	called := false
	scrollByCoordinateAction = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) error {
		called = true
		if x != 12.5 || y != 34.5 {
			t.Fatalf("wheel coordinates = (%v, %v), want (12.5, 34.5)", x, y)
		}
		if deltaX != 0 || deltaY != 50 {
			t.Fatalf("wheel delta = (%d, %d), want (0, 50)", deltaX, deltaY)
		}
		return nil
	}
	scrollViewportCenter = func(context.Context) (float64, float64, error) {
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
	if result["x"] != 0 || result["y"] != 50 {
		t.Fatalf("unexpected result payload: %#v", result)
	}
}

func TestScrollAction_UsesViewportCenterWhenCoordinatesMissing(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	origScrollByCoordinate := scrollByCoordinateAction
	origScrollViewportCenter := scrollViewportCenter
	t.Cleanup(func() {
		scrollByCoordinateAction = origScrollByCoordinate
		scrollViewportCenter = origScrollViewportCenter
	})

	scrollViewportCenter = func(context.Context) (float64, float64, error) {
		return 400, 300, nil
	}

	called := false
	scrollByCoordinateAction = func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) error {
		called = true
		if x != 400 || y != 300 {
			t.Fatalf("wheel coordinates = (%v, %v), want (400, 300)", x, y)
		}
		if deltaX != 0 || deltaY != 120 {
			t.Fatalf("wheel delta = (%d, %d), want (0, 120)", deltaX, deltaY)
		}
		return nil
	}

	result, err := b.Actions[ActionScroll](context.Background(), ActionRequest{})
	if err != nil {
		t.Fatalf("scroll returned error: %v", err)
	}
	if !called {
		t.Fatal("expected viewport-center wheel path to be used")
	}
	if result["x"] != 0 || result["y"] != 120 {
		t.Fatalf("unexpected result payload: %#v", result)
	}
	if result["targetX"] != 400.0 || result["targetY"] != 300.0 {
		t.Fatalf("unexpected scroll target payload: %#v", result)
	}
}

func TestScrollAction_PropagatesViewportCenterError(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	origScrollByCoordinate := scrollByCoordinateAction
	origScrollViewportCenter := scrollViewportCenter
	t.Cleanup(func() {
		scrollByCoordinateAction = origScrollByCoordinate
		scrollViewportCenter = origScrollViewportCenter
	})

	scrollViewportCenter = func(context.Context) (float64, float64, error) {
		return 0, 0, context.Canceled
	}
	scrollByCoordinateAction = func(context.Context, float64, float64, int, int, int) error {
		t.Fatal("wheel dispatch should not be called when viewport center resolution fails")
		return nil
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
