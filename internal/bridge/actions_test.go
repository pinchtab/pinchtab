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
