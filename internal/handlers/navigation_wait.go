package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func (h *Handlers) waitForNavigationState(ctx context.Context, waitFor, waitSelector string) error {
	waitMode := strings.ToLower(strings.TrimSpace(waitFor))
	switch waitMode {
	case "", "none":
		return nil
	case "dom":
		var ready string
		return chromedp.Run(ctx, chromedp.Evaluate(`document.readyState`, &ready))
	case "selector":
		if waitSelector == "" {
			return fmt.Errorf("waitSelector required when waitFor=selector")
		}
		return chromedp.Run(ctx, chromedp.WaitVisible(waitSelector, chromedp.ByQuery))
	case "networkidle":
		// Approximation for "network idle": require fully loaded readyState and no URL changes.
		var lastURL string
		idleChecks := 0
		for i := 0; i < 12; i++ {
			var ready, curURL string
			if err := chromedp.Run(ctx,
				chromedp.Evaluate(`document.readyState`, &ready),
				chromedp.Location(&curURL),
			); err != nil {
				return err
			}
			if ready == "complete" && curURL == lastURL {
				idleChecks++
				if idleChecks >= 2 {
					return nil
				}
			} else {
				idleChecks = 0
			}
			lastURL = curURL
			time.Sleep(250 * time.Millisecond)
		}
		return fmt.Errorf("networkidle wait timed out")
	default:
		return fmt.Errorf("unsupported waitFor %q (use: none|dom|selector|networkidle)", waitMode)
	}
}
