package bridge

import (
	"context"
	"testing"
)

func TestSelectByNodeID_UsesValue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately - no browser spawned
	// Without a real browser this will error, but it must NOT silently succeed
	// (the old implementation was a no-op that always returned nil).
	err := SelectByNodeID(ctx, 1, "option-value")
	if err == nil {
		t.Error("expected error without browser connection, got nil (possible no-op)")
	}
}
