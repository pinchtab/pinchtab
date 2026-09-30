package bridge

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
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

func TestHover_TrailIsBestEffort(t *testing.T) {
	origSettle := settleHoverAction
	t.Cleanup(func() { settleHoverAction = origSettle })

	SetHumanRandSeed(7)

	var settled [][2]float64
	settleHoverAction = func(ctx context.Context, x, y float64) error {
		settled = append(settled, [2]float64{x, y})
		return nil
	}

	// No browser attached, so every trail dispatch fails; the hover must still land.
	if err := Hover(context.Background(), 40, 60); err != nil {
		t.Fatalf("Hover with a failing trail returned %v, want nil", err)
	}
	if len(settled) != 1 || settled[0] != [2]float64{40, 60} {
		t.Fatalf("settled hovers = %v, want one at (40,60)", settled)
	}
}

func TestHover_CancelledContextStopsBeforeSettling(t *testing.T) {
	origSettle := settleHoverAction
	t.Cleanup(func() { settleHoverAction = origSettle })

	SetHumanRandSeed(7)

	settleHoverAction = func(ctx context.Context, x, y float64) error {
		t.Fatal("cancelled hover should not reach the settle dispatch")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Hover(ctx, 40, 60); !errors.Is(err, context.Canceled) {
		t.Fatalf("Hover error = %v, want context.Canceled", err)
	}
}

func TestHoverElement_ReusesTheClickTargetPoint(t *testing.T) {
	origScroll := scrollIntoViewIfNeededAction
	origBoxModel := boxModelForBackendNodeAction
	origHover := hoverCoordinateHumanAction
	origClick := clickCoordinateHumanAction
	t.Cleanup(func() {
		scrollIntoViewIfNeededAction = origScroll
		boxModelForBackendNodeAction = origBoxModel
		hoverCoordinateHumanAction = origHover
		clickCoordinateHumanAction = origClick
	})

	var calls []string
	scrollIntoViewIfNeededAction = func(ctx context.Context, backendNodeID cdp.BackendNodeID) error {
		calls = append(calls, "scroll")
		return errors.New("scroll failed but should be best-effort")
	}
	boxModelForBackendNodeAction = func(ctx context.Context, backendNodeID cdp.BackendNodeID) (*dom.BoxModel, error) {
		if backendNodeID != 99 {
			t.Fatalf("box backendNodeID = %d, want 99", backendNodeID)
		}
		calls = append(calls, "box")
		return &dom.BoxModel{Content: []float64{100, 200, 300, 200, 300, 260, 100, 260}}, nil
	}

	var hoverX, hoverY float64
	hoverCoordinateHumanAction = func(ctx context.Context, x, y float64) error {
		calls = append(calls, "hover")
		hoverX, hoverY = x, y
		return nil
	}
	var clickX, clickY float64
	clickCoordinateHumanAction = func(ctx context.Context, x, y float64) error {
		clickX, clickY = x, y
		return nil
	}

	SetHumanRandSeed(11)
	if err := HoverElement(context.Background(), 99); err != nil {
		t.Fatalf("HoverElement returned %v", err)
	}
	hoverCalls := append([]string(nil), calls...)

	SetHumanRandSeed(11)
	if err := ClickElement(context.Background(), 99); err != nil {
		t.Fatalf("ClickElement returned %v", err)
	}

	if hoverX != clickX || hoverY != clickY {
		t.Fatalf("hover point (%v,%v) != click point (%v,%v); the two humanized paths must aim at the same box-model point", hoverX, hoverY, clickX, clickY)
	}
	if hoverX < 100 || hoverX > 300 || hoverY < 200 || hoverY > 260 {
		t.Fatalf("hover point (%v,%v) is outside the box model", hoverX, hoverY)
	}
	if len(hoverCalls) != 3 || hoverCalls[0] != "scroll" || hoverCalls[1] != "box" || hoverCalls[2] != "hover" {
		t.Fatalf("calls = %v, want [scroll box hover]", hoverCalls)
	}
}

func TestHoverElement_PropagatesBoxModelFailure(t *testing.T) {
	origBoxModel := boxModelForBackendNodeAction
	origHover := hoverCoordinateHumanAction
	t.Cleanup(func() {
		boxModelForBackendNodeAction = origBoxModel
		hoverCoordinateHumanAction = origHover
	})

	boxModelForBackendNodeAction = func(ctx context.Context, backendNodeID cdp.BackendNodeID) (*dom.BoxModel, error) {
		return &dom.BoxModel{Content: []float64{1, 2}}, nil
	}
	hoverCoordinateHumanAction = func(ctx context.Context, x, y float64) error {
		t.Fatal("hover should not run without a usable box model")
		return nil
	}

	if err := HoverElement(context.Background(), 99); err == nil {
		t.Fatal("expected an error for a short box model")
	}
}

func TestHumanRand_ParallelMouseMoveAndTypeAreRaceFree(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			SetHumanRandSeed(int64(i))
			_ = MouseMove(context.Background(), 0, 0, float64(100+i), 100)
			if actions := Type("parallel", true); len(actions) < len("parallel") {
				t.Errorf("Type returned %d actions, want at least %d", len(actions), len("parallel"))
			}
		}(i)
	}
	wg.Wait()
}

type recordedButton struct {
	kind    input.MouseType
	ctxLive bool
}

func stubClickSeams(t *testing.T, onDispatch func(kind input.MouseType)) *[]recordedButton {
	t.Helper()
	origApproach, origDispatch := approachTargetAction, dispatchMouseButtonAction
	t.Cleanup(func() {
		approachTargetAction = origApproach
		dispatchMouseButtonAction = origDispatch
	})
	approachTargetAction = func(context.Context, float64, float64) error { return nil }
	var mu sync.Mutex
	var sent []recordedButton
	dispatchMouseButtonAction = func(ctx context.Context, kind input.MouseType, _, _ float64) error {
		mu.Lock()
		sent = append(sent, recordedButton{kind: kind, ctxLive: ctx.Err() == nil})
		mu.Unlock()
		if onDispatch != nil {
			onDispatch(kind)
		}
		return nil
	}
	return &sent
}

func TestClick_CancelBeforePressReturnsPromptlyWithoutPressing(t *testing.T) {
	sent := stubClickSeams(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(5*time.Millisecond, cancel)

	start := time.Now()
	err := Click(ctx, 10, 10)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Click error = %v, want context.Canceled", err)
	}
	if elapsed > 25*time.Millisecond {
		t.Fatalf("Click returned %s after cancel at 5ms, want within ~20ms", elapsed)
	}
	if len(*sent) != 0 {
		t.Fatalf("dispatched %v after cancellation before the press, want nothing", *sent)
	}
}

func TestClick_ReleasesAfterPressEvenWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := stubClickSeams(t, func(kind input.MouseType) {
		if kind == input.MousePressed {
			cancel()
		}
	})

	err := Click(ctx, 10, 10)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Click error = %v, want context.Canceled from the interrupted hold", err)
	}
	want := []recordedButton{{kind: input.MousePressed, ctxLive: true}, {kind: input.MouseReleased, ctxLive: true}}
	if len(*sent) != len(want) || (*sent)[0] != want[0] || (*sent)[1] != want[1] {
		t.Fatalf("dispatched %+v, want press then release on a live context %+v", *sent, want)
	}
}
