package orchestrator

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/session"
)

type blockingCloses struct {
	release  chan struct{}
	inFlight atomic.Int32
	peak     atomic.Int32
	arrived  atomic.Int32
	first    chan struct{}
	once     sync.Once
}

func newBlockingCloses() *blockingCloses {
	return &blockingCloses{release: make(chan struct{}), first: make(chan struct{})}
}

func (b *blockingCloses) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b.arrived.Add(1)
		now := b.inFlight.Add(1)
		defer b.inFlight.Add(-1)
		for {
			peak := b.peak.Load()
			if now <= peak || b.peak.CompareAndSwap(peak, now) {
				break
			}
		}
		b.once.Do(func() { close(b.first) })
		select {
		case <-b.release:
			_, _ = w.Write([]byte(`{"closed":[],"kept":[]}`))
		case <-r.Context().Done():
		}
	}
}

func TestABurstOfEndedSessionsKeepsTheCloseFanOutBounded(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	closes := newBlockingCloses()
	addInstanceOver(t, o, "inst_a", closes.handler(), "")
	addInstanceOver(t, o, "inst_b", closes.handler(), "")
	hook := o.SessionLifecycleHook()

	for i := 0; i < 20; i++ {
		hook(session.LifecycleEvent{SessionID: fmt.Sprintf("ses_%02d", i), Reason: session.LifecycleReasonPruned})
	}
	bound := int32(sessionCloseSlots * 2)
	deadline := time.Now().Add(2 * time.Second)
	for closes.inFlight.Load() < bound && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	peak := closes.peak.Load()
	close(closes.release)
	o.sessionCloses.Wait()

	if peak > bound {
		t.Fatalf("%d close requests were in flight at once, want at most %d (%d session slots x 2 instances)", peak, bound, sessionCloseSlots)
	}
	if got := closes.arrived.Load(); got != 40 {
		t.Fatalf("%d close requests arrived after the release, want all 40", got)
	}
}

func TestShutdownCancelsAnInFlightCloseBeforeStoppingTheInstances(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.timings.registeredBridgeStop = 100 * time.Millisecond
	closes := newBlockingCloses()
	var stopAsked, cancelledBeforeStop, closeCancelled atomic.Bool
	blockingClose := closes.handler()
	addInstanceOver(t, o, "inst_a", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case handlers.SessionTabsClosePath:
			blockingClose(w, r)
			closeCancelled.Store(r.Context().Err() != nil)
		case "/shutdown":
			o.sessionCloseMu.Lock()
			cancelledBeforeStop.Store(o.sessionCloseCtx != nil && o.sessionCloseCtx.Err() != nil)
			o.sessionCloseMu.Unlock()
			stopAsked.Store(true)
		}
	}), "")
	o.mu.Lock()
	o.instances["inst_a"].AttachType = "bridge"
	o.mu.Unlock()

	o.SessionLifecycleHook()(session.LifecycleEvent{SessionID: "ses_dead", Reason: session.LifecycleReasonRevoked})
	select {
	case <-closes.first:
	case <-time.After(2 * time.Second):
		t.Fatal("the close request never reached the instance")
	}

	started := time.Now()
	o.Shutdown()
	elapsed := time.Since(started)

	if !stopAsked.Load() {
		t.Fatal("Shutdown never asked the instance to stop")
	}
	if !cancelledBeforeStop.Load() {
		t.Fatal("the instance was asked to stop while the ended session's close was still live")
	}
	if elapsed > sessionCloseShutdownGrace {
		t.Fatalf("Shutdown took %v, want within the %v grace", elapsed, sessionCloseShutdownGrace)
	}
	deadline := time.Now().Add(time.Second)
	for !closeCancelled.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !closeCancelled.Load() {
		t.Fatal("the blocked close request was never cancelled")
	}
}

func TestASessionEndingAfterShutdownClearsItsBindingAndSendsNoClose(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.Shutdown()
	rec := &closeRecorder{}
	addInstanceOver(t, o, "inst_late", rec.handler(false), "")
	o.bindings.BindSession("ses_late", "inst_late")

	o.SessionLifecycleHook()(session.LifecycleEvent{SessionID: "ses_late", Reason: session.LifecycleReasonRevoked})
	o.sessionCloses.Wait()
	time.Sleep(50 * time.Millisecond)

	if _, ok := o.bindings.ResolveSession("ses_late"); ok {
		t.Fatal("the binding must be cleared even after shutdown")
	}
	if calls := rec.snapshot(); len(calls) != 0 {
		t.Fatalf("a close was sent after shutdown: %+v", calls)
	}
	if _, _, admitted := o.beginSessionClose(); admitted {
		o.sessionCloses.Done()
		t.Fatal("a session close was admitted to sessionCloses after Shutdown had joined it")
	}
}
