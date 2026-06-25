package bridge

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRouteDispatchPool_RunsAllJobs verifies every submitted job is run.
func TestRouteDispatchPool_RunsAllJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const njobs = 100
	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(njobs)

	pool := newRouteDispatchPool(ctx, maxRouteDispatchWorkers, func(routeDispatchJob) {
		defer wg.Done()
		count.Add(1)
	})

	for i := range njobs {
		if !pool.submit(routeDispatchJob{}) {
			t.Fatalf("submit %d unexpectedly returned full", i)
		}
	}
	wg.Wait()

	if got := count.Load(); got != njobs {
		t.Fatalf("ran %d jobs, want %d", got, njobs)
	}
}

// TestRouteDispatchPool_CapsConcurrency verifies no more than the worker count
// run concurrently.
func TestRouteDispatchPool_CapsConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const workers = 4
	const njobs = 50
	var cur, max atomic.Int32
	gate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(njobs)

	pool := newRouteDispatchPool(ctx, workers, func(routeDispatchJob) {
		defer wg.Done()
		c := cur.Add(1)
		for {
			m := max.Load()
			if c <= m || max.CompareAndSwap(m, c) {
				break
			}
		}
		<-gate
		cur.Add(-1)
	})

	for i := range njobs {
		if !pool.submit(routeDispatchJob{}) {
			t.Fatalf("submit %d unexpectedly returned full", i)
		}
	}

	// Wait until the workers are saturated (all holding a job at the gate).
	deadline := time.Now().Add(2 * time.Second)
	for cur.Load() < workers {
		if time.Now().After(deadline) {
			t.Fatalf("workers never saturated: cur=%d", cur.Load())
		}
		time.Sleep(time.Millisecond)
	}

	close(gate)
	wg.Wait()

	if got := max.Load(); got > workers {
		t.Fatalf("observed max concurrency %d exceeds worker cap %d", got, workers)
	}
}

// TestRouteDispatchPool_SubmitReturnsFalseWhenFull verifies submit never blocks
// and reports saturation so the caller can fall back.
func TestRouteDispatchPool_SubmitReturnsFalseWhenFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const workers = 2
	block := make(chan struct{})
	pool := newRouteDispatchPool(ctx, workers, func(routeDispatchJob) {
		<-block // hold workers busy so the queue cannot drain
	})

	// Capacity is at most queue size + jobs the workers pulled (≤ workers), so
	// exceeding queueSize+workers must produce at least one rejected submit.
	sawFull := false
	for range routeDispatchQueueSize + workers + 10 {
		if !pool.submit(routeDispatchJob{}) {
			sawFull = true
			break
		}
	}
	if !sawFull {
		t.Fatal("submit never returned false despite exceeding queue+worker capacity")
	}

	close(block)
}

// TestRouteDispatchPool_StopsOnContextCancel verifies workers exit on cancel and
// stop draining the queue (no goroutine leak / post-teardown work).
func TestRouteDispatchPool_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var ran atomic.Int32
	pool := newRouteDispatchPool(ctx, 2, func(routeDispatchJob) {
		ran.Add(1)
	})

	// Confirm workers are alive: a submitted job runs.
	pool.submit(routeDispatchJob{})
	deadline := time.Now().Add(time.Second)
	for ran.Load() < 1 {
		if time.Now().After(deadline) {
			t.Fatal("worker never ran the initial job")
		}
		time.Sleep(time.Millisecond)
	}

	// Workers are now blocked in select on an empty queue; cancel makes ctx.Done
	// the only ready case, so they exit deterministically.
	cancel()
	time.Sleep(50 * time.Millisecond)

	snapshot := ran.Load()
	for range routeDispatchQueueSize {
		pool.submit(routeDispatchJob{})
	}
	time.Sleep(50 * time.Millisecond)

	if got := ran.Load(); got != snapshot {
		t.Fatalf("pool ran %d jobs after cancel (was %d) — workers did not stop", got, snapshot)
	}
}
