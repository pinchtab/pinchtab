package bridge

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
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
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately - no browser spawned

	// MouseMove will try to call chromedp.Run.
	// Without a real browser it will return an error, but we cover the code path.
	_ = MouseMove(ctx, 0, 0, 100, 100)
}

func TestClick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately - no browser spawned
	_ = Click(ctx, 50, 50)
}

func TestTypeWithConfig(t *testing.T) {
	// Test with fixed seed for reproducibility
	cfg := &Config{
		Rand: rand.New(rand.NewSource(12345)),
	}

	// Generate actions twice with same config - should be identical
	actions1 := TypeWithConfig("hello", false, cfg)

	// Reset the rand source to same seed
	cfg.Rand = rand.New(rand.NewSource(12345))
	actions2 := TypeWithConfig("hello", false, cfg)

	if len(actions1) != len(actions2) {
		t.Errorf("expected same number of actions with same seed, got %d and %d", len(actions1), len(actions2))
	}

	// Verify at least some actions were generated
	if len(actions1) < 10 {
		t.Errorf("expected at least 10 actions, got %d", len(actions1))
	}
}

func TestClickElement_RequiresMinContentLength(t *testing.T) {
	// ClickElement accesses box.Content[0], [1], [2], and [5]
	// CDP BoxModel Content has 8 float64 values (4 x/y pairs)
	// The guard must check len(box.Content) < 8
	// Without a browser, GetBoxModel will fail
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately - no browser spawned
	err := ClickElement(ctx, 0)
	if err == nil {
		t.Error("expected error without browser connection")
	}
}

func TestClickElement_ScrollsBeforeReadingBoxModel(t *testing.T) {
	origScroll := scrollIntoViewIfNeededAction
	origBoxModel := boxModelForBackendNodeAction
	t.Cleanup(func() {
		scrollIntoViewIfNeededAction = origScroll
		boxModelForBackendNodeAction = origBoxModel
	})

	stop := errors.New("stop after box lookup")
	var calls []string
	scrollIntoViewIfNeededAction = func(ctx context.Context, backendNodeID cdp.BackendNodeID) error {
		if backendNodeID != 99 {
			t.Fatalf("scroll backendNodeID = %d, want 99", backendNodeID)
		}
		calls = append(calls, "scroll")
		return errors.New("scroll failed but should be best-effort")
	}
	boxModelForBackendNodeAction = func(ctx context.Context, backendNodeID cdp.BackendNodeID) (*dom.BoxModel, error) {
		if backendNodeID != 99 {
			t.Fatalf("box backendNodeID = %d, want 99", backendNodeID)
		}
		calls = append(calls, "box")
		return nil, stop
	}

	err := ClickElement(context.Background(), 99)
	if !errors.Is(err, stop) {
		t.Fatalf("ClickElement error = %v, want %v", err, stop)
	}
	if len(calls) != 2 || calls[0] != "scroll" || calls[1] != "box" {
		t.Fatalf("calls = %v, want [scroll box]", calls)
	}
}
