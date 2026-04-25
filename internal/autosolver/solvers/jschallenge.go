package solvers

import (
	"context"
	"fmt"
	"time"

	"github.com/pinchtab/pinchtab/internal/autosolver"
)

// JSChallenge is a built-in fallback solver for generic JavaScript-based
// anti-bot blocks and interstitial challenge pages.
type JSChallenge struct{}

func (s *JSChallenge) Name() string  { return "jschallenge" }
func (s *JSChallenge) Priority() int { return 40 }

func (s *JSChallenge) CanHandle(_ context.Context, page autosolver.Page) (bool, error) {
	html, err := page.HTML()
	if err != nil {
		return false, nil
	}
	intent := autosolver.DetectChallengeIntent(page.Title(), page.URL(), html)
	if intent == nil {
		return false, nil
	}
	return intent.ChallengeType == "custom-js" || intent.Type == autosolver.IntentBlocked, nil
}

func (s *JSChallenge) Solve(ctx context.Context, page autosolver.Page, executor autosolver.ActionExecutor) (*autosolver.Result, error) {
	result := &autosolver.Result{SolverUsed: s.Name()}

	// Step 1: wait briefly for JS anti-bot scripts to execute naturally.
	if err := executor.WaitFor(ctx, "body", 2*time.Second); err != nil {
		result.Error = fmt.Sprintf("wait for body: %v", err)
		return result, err
	}

	// Step 2: click common continue/verify buttons if present.
	for _, selector := range []string{
		"button[type='submit']",
		"button[name='verify']",
		"button[id*='verify']",
		"button[class*='verify']",
		"input[type='submit']",
		"#challenge-stage button",
		"form button",
	} {
		// Non-fatal; continue probing other controls.
		_ = clickIfExists(ctx, executor, selector)
	}

	// Step 3: poll for resolution by challenge intent re-check.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		html, err := page.HTML()
		if err == nil {
			intent := autosolver.DetectChallengeIntent(page.Title(), page.URL(), html)
			if intent == nil || intent.Type == autosolver.IntentNormal {
				result.Solved = true
				result.FinalTitle = page.Title()
				result.FinalURL = page.URL()
				return result, nil
			}
		}

		select {
		case <-ctx.Done():
			result.Error = ctx.Err().Error()
			return result, ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}

	// Final state snapshot.
	result.FinalTitle = page.Title()
	result.FinalURL = page.URL()
	html, err := page.HTML()
	if err != nil {
		result.Error = fmt.Sprintf("get final html: %v", err)
		return result, nil
	}
	intent := autosolver.DetectChallengeIntent(page.Title(), page.URL(), html)
	if intent == nil || intent.Type == autosolver.IntentNormal {
		result.Solved = true
		return result, nil
	}
	result.Error = fmt.Sprintf("challenge still present (%s)", intent.ChallengeType)
	return result, nil
}

func clickIfExists(ctx context.Context, executor autosolver.ActionExecutor, selector string) error {
	var coords struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	expr := fmt.Sprintf(`(() => {
		const el = document.querySelector(%q);
		if (!el) return null;
		const r = el.getBoundingClientRect();
		if (!r || r.width <= 0 || r.height <= 0) return null;
		return {x: r.x + r.width/2, y: r.y + r.height/2};
	})()`, selector)
	if err := executor.Evaluate(ctx, expr, &coords); err != nil {
		return err
	}
	if coords.X == 0 && coords.Y == 0 {
		return nil
	}
	return executor.Click(ctx, coords.X, coords.Y)
}
