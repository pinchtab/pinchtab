package bridge

import (
	"context"
	"testing"
)

func TestClickByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ClickByCoordinate(ctx, 0, 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestHoverByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := HoverByCoordinate(ctx, 10, 20)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMouseMoveByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := MouseMoveByCoordinate(ctx, 10, 20)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMouseDownByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := MouseDownByCoordinate(ctx, 10, 20, "left")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMouseUpByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := MouseUpByCoordinate(ctx, 10, 20, "left")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMouseWheelByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := MouseWheelByCoordinate(ctx, 10, 20, 0, 120)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDoubleClickByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DoubleClickByCoordinate(ctx, 0, 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDoubleClickByCoordinate_RejectNegativeCoordinates(t *testing.T) {
	ctx := context.Background()

	if err := DoubleClickByCoordinate(ctx, -1, 0); err == nil {
		t.Fatal("expected dblclick negative X coordinate to fail")
	}

	if err := DoubleClickByCoordinate(ctx, 0, -1); err == nil {
		t.Fatal("expected dblclick negative Y coordinate to fail")
	}
}

func TestCoordinateActions_RejectNegativeCoordinates(t *testing.T) {
	ctx := context.Background()

	if err := ClickByCoordinate(ctx, -1, 0); err == nil {
		t.Fatal("expected click negative coordinate to fail")
	}
	if err := HoverByCoordinate(ctx, 0, -1); err == nil {
		t.Fatal("expected hover negative coordinate to fail")
	}
	if err := MouseMoveByCoordinate(ctx, -1, 0); err == nil {
		t.Fatal("expected mouse move negative coordinate to fail")
	}
	if err := MouseDownByCoordinate(ctx, 0, -1, "right"); err == nil {
		t.Fatal("expected mouse down negative coordinate to fail")
	}
	if err := MouseUpByCoordinate(ctx, -1, 0, "middle"); err == nil {
		t.Fatal("expected mouse up negative coordinate to fail")
	}
	if err := MouseWheelByCoordinate(ctx, 0, -1, 0, 100); err == nil {
		t.Fatal("expected mouse wheel negative coordinate to fail")
	}
}
