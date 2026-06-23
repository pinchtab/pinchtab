package adapters

import (
	"context"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// stalledEndpoint listens on a TCP port, accepts connections, and then never
// replies. Pointing a chromedp remote allocator at it makes every CDP command
// (including the HTML fetch) block until the caller's context fires — the exact
// "stalled page" condition HTMLWithin's derived deadline must defend against.
func stalledEndpoint(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold the connection open but never respond, so chromedp blocks.
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	return "ws://" + ln.Addr().String()
}

// TestHTMLWithin_HonorsDeadlineOnStalledPage pins the goroutine-leak fix: when
// the underlying CDP HTML fetch stalls, HTMLWithin must return when its derived
// deadline fires instead of blocking indefinitely. This drives the real
// PinchtabPage.HTMLWithin against a chromedp tab context whose remote endpoint
// never answers, so the timeout/cancellation path — not a fast no-executor error
// — is what unblocks the call.
func TestHTMLWithin_HonorsDeadlineOnStalledPage(t *testing.T) {
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), stalledEndpoint(t))
	defer allocCancel()
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	page := NewPinchtabPage(tabCtx, "tab1", nil)

	const timeout = 300 * time.Millisecond
	start := time.Now()
	done := make(chan error, 1)
	go func() {
		_, err := page.HTMLWithin(timeout)
		done <- err
	}()

	select {
	case err := <-done:
		elapsed := time.Since(start)
		// Returned promptly via the derived deadline (not instantly, which would
		// mean the fetch never actually blocked, and not far past the timeout).
		if elapsed < timeout/2 {
			t.Fatalf("HTMLWithin returned in %v — fetch did not actually block, "+
				"so the deadline path was not exercised", elapsed)
		}
		if elapsed > 3*time.Second {
			t.Fatalf("HTMLWithin returned in %v — deadline not honored promptly", elapsed)
		}
		// A stalled fetch must surface an error, never a phantom success.
		if err == nil {
			t.Fatal("HTMLWithin returned nil error on a stalled page; expected a timeout error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HTMLWithin hung past the derived deadline — stalled fetch leaked/blocked indefinitely")
	}
}

// TestHTMLWithin_StalledFetchDoesNotLeakGoroutine asserts that, once a stalled
// HTMLWithin returns via its deadline, it has not left a worker goroutine
// running — the leak the fix exists to prevent.
func TestHTMLWithin_StalledFetchDoesNotLeakGoroutine(t *testing.T) {
	wsURL := stalledEndpoint(t)

	runStalledFetch := func() {
		allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
		defer allocCancel()
		tabCtx, tabCancel := chromedp.NewContext(allocCtx)
		defer tabCancel()
		_, _ = NewPinchtabPage(tabCtx, "tab1", nil).HTMLWithin(200 * time.Millisecond)
	}

	// Warm up to ignore one-time runtime goroutines (DNS, etc.).
	runStalledFetch()

	settle := func() { runtime.GC(); time.Sleep(200 * time.Millisecond) }
	settle()
	before := runtime.NumGoroutine()

	const iterations = 5
	for i := 0; i < iterations; i++ {
		runStalledFetch()
	}
	settle()
	after := runtime.NumGoroutine()

	// Each leak would compound per iteration; allow modest slack for runtime
	// bookkeeping but fail if the count grew on the order of the iteration count.
	if grew := after - before; grew >= iterations {
		t.Fatalf("goroutine count grew by %d across %d stalled fetches (before=%d after=%d); "+
			"a stalled HTML fetch appears to leak a worker", grew, iterations, before, after)
	}
}
