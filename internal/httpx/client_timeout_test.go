package httpx

import (
	"context"
	"testing"
	"time"
)

func TestWithClientTimeoutEndsWhenClientDisconnects(t *testing.T) {
	client, disconnect := context.WithCancel(context.Background())
	ctx, cancel := WithClientTimeout(client, context.Background(), time.Minute)
	defer cancel()

	disconnect()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context survived client disconnect")
	}
}

func TestWithClientTimeoutEndsAtDeadline(t *testing.T) {
	ctx, cancel := WithClientTimeout(context.Background(), context.Background(), 10*time.Millisecond)
	defer cancel()

	<-ctx.Done()
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("err = %v, want DeadlineExceeded", ctx.Err())
	}
}

func TestWithClientTimeoutCancelEndsOnlyTheChild(t *testing.T) {
	parent := t.Context()
	ctx, cancel := WithClientTimeout(t.Context(), parent, time.Minute)
	cancel()

	if ctx.Err() != context.Canceled {
		t.Fatalf("err = %v, want Canceled", ctx.Err())
	}
	if parent.Err() != nil {
		t.Fatal("cancel must not reach the parent")
	}
}
