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
		return h.waitForNavigationNetworkIdle(ctx)
	default:
		return fmt.Errorf("unsupported waitFor %q (use: none|dom|selector|networkidle)", waitMode)
	}
}

// waitForNavigationNetworkIdle approximates "network idle" without the per-tab
// network monitor: it requires a fully loaded readyState and a stable URL across
// two consecutive checks, capped at 12 polls. The inter-poll wait is cancellable
// via ctx (the previous raw time.Sleep was not).
func (h *Handlers) waitForNavigationNetworkIdle(ctx context.Context) error {
	var lastURL string
	idleChecks := 0
	iterations := 0
	return pollUntil(ctx, 250*time.Millisecond, func() (bool, error) {
		var ready string
		if err := h.Bridge.Evaluate(ctx, `document.readyState`, &ready, bridge.EvalOpts{}); err != nil {
			return false, err
		}
		curURL, err := h.Bridge.CurrentURL(ctx)
		if err != nil {
			return false, err
		}
		if ready == "complete" && curURL == lastURL {
			idleChecks++
			if idleChecks >= 2 {
				return true, nil
			}
		} else {
			idleChecks = 0
		}
		lastURL = curURL
		iterations++
		if iterations >= 12 {
			return false, fmt.Errorf("networkidle wait timed out")
		}
		return false, nil
	})
}
