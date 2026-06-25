package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// humanMouseStepTimeout caps each per-step Input.dispatchMouseEvent call in
// the bezier-interpolated mouse moves below. Synthetic mouse events through
// CDP can occasionally stall in --headless=new Chromium (the renderer ack
// chain doesn't always complete for fast bursts of synthesized events). A
// 30s outer ActionTimeout used to absorb the whole hang; this bound lets us
// abandon the bezier early and snap to the target so Press/Release still
// fires.
const humanMouseStepTimeout = 500 * time.Millisecond

// dispatchMouseMovedBounded runs Input.dispatchMouseEvent(mouseMoved, x, y)
// with humanMouseStepTimeout, returning ctx.Err() if the outer context was
// cancelled and a synthetic timeout error if just the step exceeded its bound.
func dispatchMouseMovedBounded(ctx context.Context, x, y float64) error {
	stepCtx, cancel := context.WithTimeout(ctx, humanMouseStepTimeout)
	defer cancel()
	return chromedp.Run(stepCtx,
		chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(c)
		}),
	)
}

var humanRand = rand.New(rand.NewSource(time.Now().UnixNano()))

func SetHumanRandSeed(seed int64) {
	humanRand = rand.New(rand.NewSource(seed))
}

type Config struct {
	Rand *rand.Rand
}

func (c *Config) getRand() *rand.Rand {
	if c != nil && c.Rand != nil {
		return c.Rand
	}
	return humanRand
}

func MouseMove(ctx context.Context, fromX, fromY, toX, toY float64) error {
	distance := math.Sqrt((toX-fromX)*(toX-fromX) + (toY-fromY)*(toY-fromY))
	baseDuration := 100 + (distance/2000)*200
	duration := baseDuration + float64(humanRand.Intn(100))

	steps := int(duration / 20)
	if steps < 5 {
		steps = 5
	}
	if steps > 30 {
		steps = 30
	}

	cp1X := fromX + (toX-fromX)*0.25 + (humanRand.Float64()-0.5)*50
	cp1Y := fromY + (toY-fromY)*0.25 + (humanRand.Float64()-0.5)*50
	cp2X := fromX + (toX-fromX)*0.75 + (humanRand.Float64()-0.5)*50
	cp2Y := fromY + (toY-fromY)*0.75 + (humanRand.Float64()-0.5)*50

	for i := 0; i <= steps; i++ {
		rawT := float64(i) / float64(steps)
		// Ease-in-out velocity curve to avoid perfectly linear timing.
		t := 0.5 - (math.Cos(math.Pi*rawT) / 2.0)

		oneMinusT := 1 - t
		x := oneMinusT*oneMinusT*oneMinusT*fromX +
			3*oneMinusT*oneMinusT*t*cp1X +
			3*oneMinusT*t*t*cp2X +
			t*t*t*toX

		y := oneMinusT*oneMinusT*oneMinusT*fromY +
			3*oneMinusT*oneMinusT*t*cp1Y +
			3*oneMinusT*t*t*cp2Y +
			t*t*t*toY

		x += (humanRand.Float64() - 0.5) * 2
		y += (humanRand.Float64() - 0.5) * 2

		if err := dispatchMouseMovedBounded(ctx, x, y); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Bezier step stalled past the per-step bound. Abandon the
			// rest of the trail and snap to the target so the caller's
			// click can still proceed.
			slog.Debug("humanMove bezier step stalled, snapping to target",
				"step", i, "of", steps, "err", err)
			return dispatchMouseMovedBounded(ctx, toX, toY)
		}

		delay := time.Duration(16+humanRand.Intn(8)) * time.Millisecond
		time.Sleep(delay)
	}

	// End-frame micro-jitter near target to emulate tiny hand stabilization.
	for i := 0; i < 2; i++ {
		jx := toX + (humanRand.Float64()-0.5)*1.2
		jy := toY + (humanRand.Float64()-0.5)*1.2
		if err := dispatchMouseMovedBounded(ctx, jx, jy); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Jitter stalled — outer ctx still good, just give up on
			// micro-jitter; the bezier already landed us at the target.
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

func Click(ctx context.Context, x, y float64) error {
	startOffsetX := (humanRand.Float64()-0.5)*200 + 50
	startOffsetY := (humanRand.Float64()-0.5)*200 + 50
	startX := x + startOffsetX
	startY := y + startOffsetY

	distance := math.Sqrt(startOffsetX*startOffsetX + startOffsetY*startOffsetY)
	if distance > 30 {
		// Best-effort: the initial random-start move and bezier trail are
		// only there for human-trail realism. If they stall, log and
		// proceed — the subsequent MousePressed sets the cursor at (x,y)
		// explicitly, so the click still lands.
		if err := dispatchMouseMovedBounded(ctx, startX, startY); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Debug("humanized click initial-move stalled, skipping bezier", "err", err)
		} else if err := MouseMove(ctx, startX, startY, x, y); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Debug("humanized click bezier trail failed, proceeding to click", "err", err)
		}
	}

	time.Sleep(time.Duration(50+humanRand.Intn(150)) * time.Millisecond)

	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MousePressed, x, y).
				WithButton(input.Left).
				WithClickCount(1).
				Do(ctx)
		}),
	); err != nil {
		return err
	}

	time.Sleep(time.Duration(30+humanRand.Intn(90)) * time.Millisecond)

	releaseX := x + (humanRand.Float64()-0.5)*2
	releaseY := y + (humanRand.Float64()-0.5)*2

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseReleased, releaseX, releaseY).
				WithButton(input.Left).
				WithClickCount(1).
				Do(ctx)
		}),
	)
}

var scrollIntoViewIfNeededAction = func(ctx context.Context, backendNodeID cdp.BackendNodeID) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return dom.ScrollIntoViewIfNeeded().WithBackendNodeID(backendNodeID).Do(ctx)
	}))
}

var boxModelForBackendNodeAction = func(ctx context.Context, backendNodeID cdp.BackendNodeID) (*dom.BoxModel, error) {
	var box *dom.BoxModel
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			box, err = dom.GetBoxModel().WithBackendNodeID(backendNodeID).Do(ctx)
			return err
		}),
	)
	return box, err
}

var clickCoordinateHumanAction = Click

// The ID is the backendDOMNodeId from the accessibility tree, NOT a regular DOM nodeId.
func ClickElement(ctx context.Context, backendNodeID cdp.BackendNodeID) error {
	// Scroll the target into the visual viewport first. The humanized click is
	// coordinate-based (Input.dispatchMouseEvent at the box-model center), so an
	// element below the fold or freshly revealed (lazy-loaded list item,
	// modal-triggered button) would otherwise produce coordinates that land on
	// whatever is currently visible there — the dispatch "succeeds" but the
	// intended click handler never fires. Best-effort: not every node scrolls.
	_ = scrollIntoViewIfNeededAction(ctx, backendNodeID)

	box, err := boxModelForBackendNodeAction(ctx, backendNodeID)
	if err != nil {
		return err
	}

	if len(box.Content) < 8 {
		return fmt.Errorf("invalid box model")
	}

	centerX := (box.Content[0] + box.Content[2]) / 2
	centerY := (box.Content[1] + box.Content[5]) / 2

	offsetX := (humanRand.Float64() - 0.5) * 10
	offsetY := (humanRand.Float64() - 0.5) * 10

	return clickCoordinateHumanAction(ctx, centerX+offsetX, centerY+offsetY)
}

func Type(text string, fast bool) []chromedp.Action {
	return TypeWithConfig(text, fast, nil)
}

func TypeWithConfig(text string, fast bool, cfg *Config) []chromedp.Action {
	rng := cfg.getRand()
	actions := []chromedp.Action{}

	baseDelay := 80
	if fast {
		baseDelay = 40
	}

	chars := []rune(text)
	for i, char := range chars {
		actions = append(actions, chromedp.KeyEvent(string(char)))
		delay := baseDelay + rng.Intn(baseDelay/2)
		if rng.Float64() < 0.05 {
			delay += rng.Intn(500)
		}
		if i > 0 && chars[i-1] == char {
			delay = delay / 2
		}
		actions = append(actions, chromedp.Sleep(time.Duration(delay)*time.Millisecond))

		if rng.Float64() < 0.03 && i < len(chars)-1 {
			wrongChar := rune('a' + rng.Intn(26))
			actions = append(actions,
				chromedp.KeyEvent(string(wrongChar)),
				chromedp.Sleep(time.Duration(50+rng.Intn(100))*time.Millisecond),
				chromedp.KeyEvent("\b"),
				chromedp.Sleep(time.Duration(30+rng.Intn(70))*time.Millisecond),
			)
		}
	}
	return actions
}
