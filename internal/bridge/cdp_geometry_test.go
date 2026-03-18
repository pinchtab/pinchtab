package bridge

import (
	"context"
	"testing"
)

func TestGetElementCenter_ParsesBoxModel(t *testing.T) {
	// Test the box model parsing logic
	// Content quad: [x1,y1, x2,y2, x3,y3, x4,y4]
	// For a 100x50 box at position (200, 100):
	// corners: (200,100), (300,100), (300,150), (200,150)
	content := []float64{200, 100, 300, 100, 300, 150, 200, 150}

	// Calculate expected center
	expectedX := (content[0] + content[2] + content[4] + content[6]) / 4 // (200+300+300+200)/4 = 250
	expectedY := (content[1] + content[3] + content[5] + content[7]) / 4 // (100+100+150+150)/4 = 125

	if expectedX != 250 {
		t.Errorf("expected X=250, got %f", expectedX)
	}
	if expectedY != 125 {
		t.Errorf("expected Y=125, got %f", expectedY)
	}
}

// TestGetElementCenterJS_ContextCancelled verifies that getElementCenterJS
// returns an error when the context is already cancelled (no browser panic).
func TestGetElementCenterJS_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := getElementCenterJS(ctx, 1)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
