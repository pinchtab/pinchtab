package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func (h *Handlers) waitForNavigationState(ctx context.Context, waitFor, waitSelector string) error {
	waitMode := strings.ToLower(strings.TrimSpace(waitFor))
	switch waitMode {
	case "", "none":
		return nil
	case "dom":
		var ready string
		return h.Bridge.Evaluate(ctx, `document.readyState`, &ready, bridge.EvalOpts{})
	case "selector":
		if waitSelector == "" {
			return fmt.Errorf("waitSelector required when waitFor=selector")
		}
		return h.Bridge.WaitVisible(ctx, waitSelector)
	case "networkidle":
		// Approximation for "network idle": require fully loaded readyState and no URL changes.
		var lastURL string
		idleChecks := 0
		for i := 0; i < 12; i++ {
			var ready string
			if err := h.Bridge.Evaluate(ctx, `document.readyState`, &ready, bridge.EvalOpts{}); err != nil {
				return err
			}
			curURL, err := h.Bridge.CurrentURL(ctx)
			if err != nil {
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
