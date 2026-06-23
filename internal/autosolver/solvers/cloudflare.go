// Package solvers provides built-in solver implementations using the
// autosolver interface system. These solvers depend only on the Page
// and ActionExecutor interfaces — never on chromedp directly.
package solvers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/cfchallenge"
)

// Cloudflare implements autosolver.Solver for Cloudflare Turnstile
// and interstitial challenges. Unlike bridge/cloudflare.go, this
// implementation uses the Page/ActionExecutor abstraction and has
// zero dependency on chromedp.
type Cloudflare struct{}

func (s *Cloudflare) Name() string  { return "cloudflare" }
func (s *Cloudflare) Priority() int { return 10 }

// CanHandle checks for Cloudflare challenge indicators in the page title.
func (s *Cloudflare) CanHandle(_ context.Context, page autosolver.Page) (bool, error) {
	return isCFChallenge(page.Title()), nil
}

// Solve attempts to resolve the Cloudflare challenge by locating the
// Turnstile widget and clicking the checkbox.
func (s *Cloudflare) Solve(ctx context.Context, page autosolver.Page, executor autosolver.ActionExecutor) (*autosolver.Result, error) {
	result := &autosolver.Result{SolverUsed: "cloudflare"}

	if !isCFChallenge(page.Title()) {
		result.Solved = true
		return result, nil
	}

	challengeType, err := detectCFChallengeType(ctx, executor)
	if err != nil {
		return result, fmt.Errorf("detect challenge type: %w", err)
	}

	// Non-interactive challenges resolve automatically.
	if challengeType == "non-interactive" {
		return waitForCFResolve(ctx, page, result, 15*time.Second)
	}

	for attempt := 0; attempt < 3; attempt++ {
		result.Attempts = attempt + 1

		waitForSpinner(ctx, executor, 10*time.Second)

		box, err := findTurnstileBox(ctx, executor)
		if err != nil {
			// Challenge may have resolved while we were looking.
			if !isCFChallenge(page.Title()) {
				result.Solved = true
				result.FinalTitle = page.Title()
				return result, nil
			}
			time.Sleep(1 * time.Second)
			continue
		}

		// Click the checkbox area (left portion of the widget).
		checkboxX := box.x + box.width*0.09
		checkboxY := box.y + box.height*0.40

		if err := executor.Click(ctx, checkboxX, checkboxY); err != nil {
			return result, fmt.Errorf("click turnstile: %w", err)
		}

		resolved := pollResolution(ctx, page, 15*time.Second)
		if resolved {
			result.Solved = true
			result.FinalTitle = page.Title()
			return result, nil
		}
	}

	result.FinalTitle = page.Title()
	result.Solved = !isCFChallenge(page.Title())
	return result, nil
}

type boundingBox struct {
	x, y, width, height float64
}

func isCFChallenge(title string) bool {
	return cfchallenge.IsChallengeTitle(title)
}

func detectCFChallengeType(ctx context.Context, executor autosolver.ActionExecutor) (string, error) {
	var content string
	if err := executor.Evaluate(ctx, `document.documentElement.outerHTML`, &content); err != nil {
		return "", err
	}

	for _, ct := range cfchallenge.CTypeTokens {
		if strings.Contains(content, fmt.Sprintf("cType: '%s'", ct)) {
			return ct, nil
		}
	}

	var hasEmbedded bool
	if err := executor.Evaluate(ctx,
		cfchallenge.EmbeddedTurnstileScriptJS,
		&hasEmbedded); err == nil && hasEmbedded {
		return "embedded", nil
	}

	return "", nil
}

func findTurnstileBox(ctx context.Context, executor autosolver.ActionExecutor) (*boundingBox, error) {
	var rawBox map[string]float64
	err := executor.Evaluate(ctx, cfchallenge.TurnstileBoxJS, &rawBox)
	if err != nil {
		return nil, fmt.Errorf("evaluate turnstile box: %w", err)
	}
	if rawBox == nil {
		return nil, fmt.Errorf("turnstile element not found")
	}

	return &boundingBox{
		x:      rawBox["x"],
		y:      rawBox["y"],
		width:  rawBox["width"],
		height: rawBox["height"],
	}, nil
}

func waitForSpinner(ctx context.Context, executor autosolver.ActionExecutor, timeout time.Duration) {
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
			if err := executor.Evaluate(ctx, `document.body.innerText`, &text); err != nil {
				continue
			}
			if !strings.Contains(text, cfchallenge.SpinnerText) {
				return
			}
		}
	}
}

func waitForCFResolve(ctx context.Context, page autosolver.Page, result *autosolver.Result, timeout time.Duration) (*autosolver.Result, error) {
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
			if !isCFChallenge(page.Title()) {
				result.Solved = true
				result.FinalTitle = page.Title()
				return result, nil
			}
		}
	}
}

func pollResolution(ctx context.Context, page autosolver.Page, timeout time.Duration) bool {
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
			if !isCFChallenge(page.Title()) {
				time.Sleep(1 * time.Second)
				return true
			}
		}
	}
}
