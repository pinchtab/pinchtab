package observe

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestReserveCaptureListenerIdempotent(t *testing.T) {
	nm := NewNetworkMonitor(0)
	ctx := context.Background()

	lctx1, cancel1, already1 := nm.reserveCaptureListener("tab1", ctx)
	if already1 {
		t.Fatal("first reserve should not be alreadyActive")
	}
	if lctx1 == nil || cancel1 == nil {
		t.Fatal("first reserve should return a listener ctx + cancel")
	}
	if _, ok := nm.listeners["tab1"]; !ok {
		t.Fatal("reserve should store the cancel in listeners")
	}

	// A second reserve for the same tab must not stack another listener.
	lctx2, cancel2, already2 := nm.reserveCaptureListener("tab1", ctx)
	if !already2 {
		t.Fatal("second reserve for the same tab should be alreadyActive")
	}
	if lctx2 != nil || cancel2 != nil {
		t.Fatal("second reserve should return nil ctx/cancel")
	}
	if len(nm.listeners) != 1 {
		t.Fatalf("listeners should still have 1 entry, got %d", len(nm.listeners))
	}

	if _, _, already3 := nm.reserveCaptureListener("tab2", ctx); already3 {
		t.Fatal("a distinct tab should not be alreadyActive")
	}
	if len(nm.listeners) != 2 {
		t.Fatalf("listeners should have 2 entries, got %d", len(nm.listeners))
	}
}

func TestReleaseCaptureListenerCancelsAndRemoves(t *testing.T) {
	nm := NewNetworkMonitor(0)
	lctx, _, _ := nm.reserveCaptureListener("tab1", context.Background())

	nm.releaseCaptureListener("tab1")

	if _, ok := nm.listeners["tab1"]; ok {
		t.Fatal("release should remove the listeners entry")
	}
	select {
	case <-lctx.Done():
	default:
		t.Fatal("release should cancel the listener context")
	}

	// Releasing again must be a safe no-op.
	nm.releaseCaptureListener("tab1")
}

func TestStopCaptureRemovesBufferAndListener(t *testing.T) {
	nm := NewNetworkMonitor(0)
	lctx, _, _ := nm.reserveCaptureListener("tab1", context.Background())
	nm.getOrCreateBuffer("tab1")

	if nm.GetBuffer("tab1") == nil {
		t.Fatal("setup: expected a buffer")
	}

	nm.StopCapture("tab1")

	if nm.GetBuffer("tab1") != nil {
		t.Fatal("StopCapture should remove the buffer")
	}
	if _, ok := nm.listeners["tab1"]; ok {
		t.Fatal("StopCapture should remove the listeners entry")
	}
	select {
	case <-lctx.Done():
	default:
		t.Fatal("StopCapture should cancel the listener context")
	}
}

func TestReserveCaptureListenerParentCancelPropagates(t *testing.T) {
	nm := NewNetworkMonitor(0)
	parent, cancelParent := context.WithCancel(context.Background())
	lctx, _, _ := nm.reserveCaptureListener("tab1", parent)

	cancelParent()
	select {
	case <-lctx.Done():
	default:
		t.Fatal("cancelling the parent ctx should cancel the listener ctx")
	}
}

func TestSubscribeCompletionsNotifiesOnce(t *testing.T) {
	nb := NewNetworkBuffer(0)
	id, completions := nb.SubscribeCompletions()
	defer nb.UnsubscribeCompletions(id)

	nb.MarkRequestStart("r1")
	nb.MarkRequestEnd("r1")

	select {
	case got := <-completions:
		if got != "r1" {
			t.Fatalf("completion = %q, want r1", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected a completion notification for r1")
	}

	// A second MarkRequestEnd for an already-ended request must not re-notify.
	nb.MarkRequestEnd("r1")
	select {
	case got := <-completions:
		t.Fatalf("unexpected second completion: %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEntrySubscriberDoesNotReceiveCompletions(t *testing.T) {
	nb := NewNetworkBuffer(0)
	subID, ch := nb.Subscribe()
	defer nb.Unsubscribe(subID)

	nb.MarkRequestStart("r1")
	nb.MarkRequestEnd("r1")

	select {
	case e := <-ch:
		t.Fatalf("entry subscriber should not receive completion events, got %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSignalBodyChangeClosesAndRenews(t *testing.T) {
	nb := NewNetworkBuffer(0)
	ch := nb.BodyChangeChan()

	select {
	case <-ch:
		t.Fatal("body-change channel should be open before any signal")
	default:
	}

	nb.SignalBodyChange()

	select {
	case <-ch:
		// closed -> a waiter on this channel would wake. Good.
	default:
		t.Fatal("SignalBodyChange should close the previously captured channel")
	}

	next := nb.BodyChangeChan()
	if next == ch {
		t.Fatal("a fresh channel should be installed after a signal")
	}
	select {
	case <-next:
		t.Fatal("the post-signal channel should be open")
	default:
	}
}

func TestPublishCompletionDropsWhenSubscriberFull(t *testing.T) {
	nb := NewNetworkBuffer(0)
	id, _ := nb.SubscribeCompletions() // never drained
	defer nb.UnsubscribeCompletions(id)

	done := make(chan struct{})
	go func() {
		// Far more than the 64-slot channel cap; excess must be dropped, not block.
		for i := 0; i < 200; i++ {
			rid := fmt.Sprintf("r%d", i)
			nb.MarkRequestStart(rid)
			nb.MarkRequestEnd(rid)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publishCompletion blocked on a full subscriber channel")
	}
}
