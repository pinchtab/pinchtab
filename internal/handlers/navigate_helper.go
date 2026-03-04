package handlers

import (
	"context"
	"fmt"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// WaitMode constants for readiness after navigation.
const (
	WaitDOM      = "dom"         // document.readyState == interactive (fast, a11y/text ready)
	WaitComplete = "networkidle" // network idle (visual rendering done, closest to document.readyState complete)
)

// ensureNavigated navigates to the given URL within the provided chromedp
// context if url is non-empty. Uses the appropriate wait strategy for the
// operation type. If url is empty, this is a no-op (use current page).
//
// defaultWait sets the readiness signal:
//   - WaitDOM ("dom")           — good for snapshot/text/find/eval
//   - WaitComplete ("complete") — needed for screenshot/pdf
//
// If waitFor is provided by the caller (e.g. query param), it overrides defaultWait.
func (h *Handlers) ensureNavigated(ctx context.Context, url, waitFor, defaultWait string) error {
	if url == "" {
		return nil
	}

	if err := bridge.NavigatePage(ctx, url); err != nil {
		return fmt.Errorf("navigate to %s: %w", url, err)
	}

	wait := defaultWait
	if waitFor != "" {
		wait = waitFor
	}

	return h.waitForNavigationState(ctx, wait, "")
}
