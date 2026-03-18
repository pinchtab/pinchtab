package bridge

import (
	"context"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestSelectByNodeID_UsesValue(t *testing.T) {
	ctx, _ := chromedp.NewContext(context.Background())
	// Without a real browser this will error, but it must NOT silently succeed
	// (the old implementation was a no-op that always returned nil).
	err := SelectByNodeID(ctx, 1, "option-value")
	if err == nil {
		t.Error("expected error without browser connection, got nil (possible no-op)")
	}
}
