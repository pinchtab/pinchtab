package bridge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/cfchallenge"
	"github.com/pinchtab/pinchtab/internal/solver"
)

func init() {
	solver.MustRegister("cloudflare", &CloudflareSolver{})
}

// CloudflareSolver detects and solves Cloudflare Turnstile/Interstitial challenges.
// It locates the Turnstile iframe, clicks the checkbox with human-like input,
// and polls for resolution.
type CloudflareSolver struct{}

func (s *CloudflareSolver) Name() string { return "cloudflare" }

// CanHandle checks the page title for known Cloudflare challenge indicators.
func (s *CloudflareSolver) CanHandle(ctx context.Context) (bool, error) {
	var title string
	if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
		return false, fmt.Errorf("get title: %w", err)
	}
	return isCFChallenge(title), nil
}

// Solve attempts to resolve the Cloudflare challenge on the current page.
func (s *CloudflareSolver) Solve(ctx context.Context, opts solver.Options) (*solver.Result, error) {
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	result := &solver.Result{Solver: "cloudflare"}

	var title string
	if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
		return result, fmt.Errorf("get title: %w", err)
	}
	result.Title = title

	if !isCFChallenge(title) {
		result.Solved = true
		return result, nil
	}

	challengeType := detectCFType(ctx)
	result.ChallengeType = challengeType

	if challengeType == "" {
		time.Sleep(2 * time.Second)
		challengeType = detectCFType(ctx)
		result.ChallengeType = challengeType
	}

	if challengeType == "non-interactive" {
		return waitForCFResolve(ctx, result, 15*time.Second)
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		result.Attempts = attempt + 1

		waitForCFSpinner(ctx, 10*time.Second)

		box, err := findTurnstileBox(ctx)
		if err != nil {
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err == nil && !isCFChallenge(title) {
				result.Solved = true
				result.Title = title
				return result, nil
			}
			time.Sleep(1 * time.Second)
			continue
		}

		// Click the checkbox area using relative positioning within the
		// Turnstile widget. The checkbox sits in the left portion (~9% from
		// the left edge, ~40% from the top). Wider random offsets (+-4px)
		// make the click pattern harder to fingerprint.
		checkboxX := box.X + box.Width*0.09
		checkboxY := box.Y + box.Height*0.40
		clickX := checkboxX + (humanRand.Float64()-0.5)*8
		clickY := checkboxY + (humanRand.Float64()-0.5)*8

		if err := Click(ctx, clickX, clickY); err != nil {
			return result, fmt.Errorf("click turnstile: %w", err)
		}

		resolved := pollCFResolution(ctx, 15*time.Second)
		if resolved {
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err == nil {
				result.Title = title
			}
			result.Solved = true
			return result, nil
		}
	}

	if err := chromedp.Run(ctx, chromedp.Title(&title)); err == nil {
		result.Title = title
		result.Solved = !isCFChallenge(title)
	}

	return result, nil
}

type cfBoundingBox struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

func isCFChallenge(title string) bool {
	return cfchallenge.IsChallengeTitle(title)
}

func detectCFType(ctx context.Context) string {
	var content string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.outerHTML`, &content)); err != nil {
		return ""
	}

	for _, ct := range cfchallenge.CTypeTokens {
		if strings.Contains(content, fmt.Sprintf("cType: '%s'", ct)) {
			return ct
		}
	}

	var hasEmbedded bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		cfchallenge.EmbeddedTurnstileScriptJS,
		&hasEmbedded,
	)); err == nil && hasEmbedded {
		return "embedded"
	}

	return ""
}

func findTurnstileBox(ctx context.Context) (*cfBoundingBox, error) {
	var rawBox map[string]float64
	err := chromedp.Run(ctx, chromedp.Evaluate(cfchallenge.TurnstileBoxJS, &rawBox))
	if err != nil {
		return nil, fmt.Errorf("evaluate turnstile box: %w", err)
	}
	if rawBox == nil {
		return nil, fmt.Errorf("turnstile element not found")
	}

	return &cfBoundingBox{
		X:      rawBox["x"],
		Y:      rawBox["y"],
		Width:  rawBox["width"],
		Height: rawBox["height"],
	}, nil
}

func waitForCFSpinner(ctx context.Context, timeout time.Duration) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			return
		case <-ticker.C:
			var text string
			if err := chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText`, &text)); err != nil {
				continue
			}
			if !strings.Contains(text, cfchallenge.SpinnerText) {
				return
			}
		}
	}
}

func waitForCFResolve(ctx context.Context, result *solver.Result, timeout time.Duration) (*solver.Result, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-deadline:
			return result, nil
		case <-ticker.C:
			var title string
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
				continue
			}
			if !isCFChallenge(title) {
				result.Solved = true
				result.Title = title
				return result, nil
			}
		}
	}
}

func pollCFResolution(ctx context.Context, timeout time.Duration) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline:
			return false
		case <-ticker.C:
			var title string
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
				continue
			}
			if !isCFChallenge(title) {
				time.Sleep(1 * time.Second)
				return true
			}
		}
	}
}
