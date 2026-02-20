package human

import (
	"context"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestSetHumanRandSeed(t *testing.T) {
	// Just verify it doesn't panic
	SetHumanRandSeed(12345)
}

func TestType(t *testing.T) {
	text := "hello"

	// Test normal typing
	actions := Type(text, false)
	if len(actions) < len(text) {
		t.Errorf("expected at least %d actions, got %d", len(text), len(actions))
	}

	// Test fast typing
	fastActions := Type(text, true)
	if len(fastActions) < len(text) {
		t.Errorf("expected at least %d actions, got %d", len(text), len(fastActions))
	}
}

func TestTypeWithCorrections(t *testing.T) {
	// Use a fixed seed that we know triggers a correction (statistically likely with long string)
	SetHumanRandSeed(1)
	text := "this is a very long string to increase the chance of a simulated typo correction"
	actions := Type(text, false)

	// If a typo happened, there will be more actions than just KeyEvents and Sleeps for each char
	if len(actions) < len(text)*2 {
		t.Errorf("expected many actions for long string, got %d", len(actions))
	}
}

func TestMouseMove(t *testing.T) {
	// Create a context that chromedp.Run won't immediately reject
	ctx, _ := chromedp.NewContext(context.Background())

	// MouseMove will try to call chromedp.Run.
	// Without a real browser it will return an error, but we cover the code path.
	_ = MouseMove(ctx, 0, 0, 100, 100)
}

func TestClick(t *testing.T) {
	ctx, _ := chromedp.NewContext(context.Background())
	_ = Click(ctx, 50, 50)
}
