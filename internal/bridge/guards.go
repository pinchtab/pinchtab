package bridge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"
)

var (
	// ErrUnexpectedNavigation indicates an action was expected to keep context stable
	// but the page URL changed.
	ErrUnexpectedNavigation = errors.New("unexpected page navigation")
	// ErrElementStale indicates the targeted DOM/backend node is no longer valid.
	ErrElementStale = errors.New("element reference is stale")
)

// URLReader is used by guards to read the current tab URL from an action context.
type URLReader func(ctx context.Context) (string, error)

var readActionURL URLReader = func(ctx context.Context) (string, error) {
	if chromedp.FromContext(ctx) == nil {
		return "", nil
	}
	var current string
	if err := chromedp.Run(ctx, chromedp.Location(&current)); err != nil {
		return "", err
	}
	return strings.TrimSpace(current), nil
}

func classifyActionError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrElementStale) {
		return err
	}
	e := strings.ToLower(err.Error())
	if strings.Contains(e, "could not find node") ||
		strings.Contains(e, "node with given id") ||
		strings.Contains(e, "no node") ||
		strings.Contains(e, "node with given identifier does not exist") {
		return fmt.Errorf("%w: %v", ErrElementStale, err)
	}
	return err
}

func shouldCheckUnexpectedNavigation(kind string, req ActionRequest) bool {
	if req.WaitNav {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case ActionClick, ActionDoubleClick, ActionHumanClick, ActionPress:
		return false
	default:
		return true
	}
}

func checkUnexpectedNavigation(before, after string) error {
	before = strings.TrimSpace(before)
	after = strings.TrimSpace(after)
	if before == "" || after == "" || before == after {
		return nil
	}
	return fmt.Errorf("%w: %s -> %s", ErrUnexpectedNavigation, before, after)
}
