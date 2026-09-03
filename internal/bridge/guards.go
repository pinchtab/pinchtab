package bridge

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/chromedp/chromedp"
)

var (
	// ErrElementStale indicates the targeted DOM/backend node is no longer valid.
	ErrElementStale         = errors.New("element reference is stale")
	ErrInvalidActionRequest = errors.New("invalid action request")
)

func NewInvalidActionRequestError(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalidActionRequest, msg)
}

// URLReader is used by guards to read the current tab URL from an action context.
type URLReader func(ctx context.Context) (string, error)

func defaultActionURLReader(ctx context.Context) (string, error) {
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

// navigationChanged reports whether an action moved the page. It used to return
// an error, and ExecuteAction returned that error INSTEAD of the successful
// result the action had already produced — so clicking an ordinary link failed
// after the click had worked. A navigation is an outcome to report, not a
// failure: the caller is told where it landed and that its refs are dead, and
// the result it earned is delivered.
//
// A fragment-only or case-of-host change is not a navigation: the document is
// the same and the refs still resolve.
func navigationChanged(before, after string) bool {
	before = strings.TrimSpace(before)
	after = strings.TrimSpace(after)
	if before == "" || after == "" || before == after {
		return false
	}
	if normalizedBefore, ok := normalizeGuardURL(before); ok {
		if normalizedAfter, ok := normalizeGuardURL(after); ok && normalizedBefore == normalizedAfter {
			return false
		}
	}
	return true
}

func normalizeGuardURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return "", false
	}
	parsed.Fragment = ""
	if parsed.Host != "" {
		parsed.Host = strings.ToLower(parsed.Host)
	}
	return parsed.String(), true
}
