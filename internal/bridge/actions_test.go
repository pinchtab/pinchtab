package bridge

import (
	"context"
	"strings"
	"testing"

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
	scrollByCoordinateAction = func(ctx context.Context, x, y float64, deltaX, deltaY int) error {
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
		HasXY:       true,
		X:           50,
		Y:           75,
		WheelDeltaX: 123,
		WheelDeltaY: -456,
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

func TestScrollAction_UsesCoordinateWheelPath(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})

	origScrollByCoordinate := scrollByCoordinateAction
	origScrollViewportCenter := scrollViewportCenter
	t.Cleanup(func() {
		scrollByCoordinateAction = origScrollByCoordinate
		scrollViewportCenter = origScrollViewportCenter
	})

	called := false
	scrollByCoordinateAction = func(ctx context.Context, x, y float64, deltaX, deltaY int) error {
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
	scrollByCoordinateAction = func(ctx context.Context, x, y float64, deltaX, deltaY int) error {
		called = true
		if x != 400 || y != 300 {
			t.Fatalf("wheel coordinates = (%v, %v), want (400, 300)", x, y)
		}
		if deltaX != 0 || deltaY != 800 {
			t.Fatalf("wheel delta = (%d, %d), want (0, 800)", deltaX, deltaY)
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
	if result["x"] != 0 || result["y"] != 800 {
		t.Fatalf("unexpected result payload: %#v", result)
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
	scrollByCoordinateAction = func(context.Context, float64, float64, int, int) error {
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

// ── Keyboard action tests ──────────────────────────────────────────────

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

// ── ScrollIntoView action tests ────────────────────────────────────────

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
