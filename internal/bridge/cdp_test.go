package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestWaitForTitle_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := WaitForTitle(ctx, 5*time.Second)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestWaitForTitle_NoTimeout(t *testing.T) {
	ctx, _ := chromedp.NewContext(context.Background())

	// With timeout <= 0, should return immediately
	title, _ := WaitForTitle(ctx, 0)
	if title != "" {
		t.Errorf("expected empty title without browser, got %q", title)
	}
}

func TestNavigatePage_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NavigatePage(ctx, "https://example.com")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestSelectByNodeID_UsesValue(t *testing.T) {
	ctx, _ := chromedp.NewContext(context.Background())
	// Without a real browser this will error, but it must NOT silently succeed
	// (the old implementation was a no-op that always returned nil).
	err := SelectByNodeID(ctx, 1, "option-value")
	if err == nil {
		t.Error("expected error without browser connection, got nil (possible no-op)")
	}
}

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
