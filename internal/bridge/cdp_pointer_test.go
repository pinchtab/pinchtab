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
}
