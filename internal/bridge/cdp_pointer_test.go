package bridge

import (
	"context"
	"testing"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

func TestClickByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bridgecdpops.ClickByCoordinate(ctx, 0, 0, 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestHoverByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bridgecdpops.HoverByCoordinate(ctx, 10, 20)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMouseMoveByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bridgecdpops.MouseMoveByCoordinate(ctx, 10, 20)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMouseDownByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bridgecdpops.MouseDownByCoordinate(ctx, 10, 20, "left", 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMouseUpByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bridgecdpops.MouseUpByCoordinate(ctx, 10, 20, "left", 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMouseWheelByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bridgecdpops.MouseWheelByCoordinate(ctx, 10, 20, 0, 120, 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDoubleClickByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bridgecdpops.DoubleClickByCoordinate(ctx, 0, 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDoubleClickByCoordinate_RejectNegativeCoordinates(t *testing.T) {
	ctx := context.Background()

	if err := bridgecdpops.DoubleClickByCoordinate(ctx, -1, 0); err == nil {
		t.Fatal("expected dblclick negative X coordinate to fail")
	}

	if err := bridgecdpops.DoubleClickByCoordinate(ctx, 0, -1); err == nil {
		t.Fatal("expected dblclick negative Y coordinate to fail")
	}
}

func TestCoordinateActions_RejectNegativeCoordinates(t *testing.T) {
	ctx := context.Background()

	if err := bridgecdpops.ClickByCoordinate(ctx, -1, 0, 0); err == nil {
		t.Fatal("expected click negative coordinate to fail")
	}
	if err := bridgecdpops.HoverByCoordinate(ctx, 0, -1); err == nil {
		t.Fatal("expected hover negative coordinate to fail")
	}
	if err := bridgecdpops.MouseMoveByCoordinate(ctx, -1, 0); err == nil {
		t.Fatal("expected mouse move negative coordinate to fail")
	}
	if err := bridgecdpops.MouseDownByCoordinate(ctx, 0, -1, "right", 0); err == nil {
		t.Fatal("expected mouse down negative coordinate to fail")
	}
	if err := bridgecdpops.MouseUpByCoordinate(ctx, -1, 0, "middle", 0); err == nil {
		t.Fatal("expected mouse up negative coordinate to fail")
	}
	if err := bridgecdpops.MouseWheelByCoordinate(ctx, 0, -1, 0, 100, 0); err == nil {
		t.Fatal("expected mouse wheel negative coordinate to fail")
	}
}
