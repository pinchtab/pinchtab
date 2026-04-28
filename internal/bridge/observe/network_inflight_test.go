package observe

import (
	"testing"
	"time"
)

func TestNetworkBuffer_InflightLifecycle(t *testing.T) {
	buf := NewNetworkBuffer(10)

	count, _ := buf.InflightStatus()
	if count != 0 {
		t.Fatalf("fresh buffer: got count=%d, want 0", count)
	}

	buf.MarkRequestStart("req-1")
	buf.MarkRequestStart("req-2")
	count, lastChange := buf.InflightStatus()
	if count != 2 {
		t.Errorf("after 2 starts: got count=%d, want 2", count)
	}
	startTime := lastChange

	// Sleep so we can detect lastChange advancing.
	time.Sleep(2 * time.Millisecond)

	buf.MarkRequestEnd("req-1")
	count, lastChange = buf.InflightStatus()
	if count != 1 {
		t.Errorf("after 1 end: got count=%d, want 1", count)
	}
	if !lastChange.After(startTime) {
		t.Errorf("lastChange did not advance after MarkRequestEnd")
	}

	buf.MarkRequestEnd("req-2")
	count, _ = buf.InflightStatus()
	if count != 0 {
		t.Errorf("after all ends: got count=%d, want 0", count)
	}
}

func TestNetworkBuffer_InflightIdempotent(t *testing.T) {
	buf := NewNetworkBuffer(10)

	buf.MarkRequestStart("req-1")
	buf.MarkRequestStart("req-1") // duplicate start should be no-op
	count, _ := buf.InflightStatus()
	if count != 1 {
		t.Errorf("duplicate start: got count=%d, want 1", count)
	}

	buf.MarkRequestEnd("req-1")
	buf.MarkRequestEnd("req-1") // duplicate end should be no-op
	buf.MarkRequestEnd("never-started")
	count, _ = buf.InflightStatus()
	if count != 0 {
		t.Errorf("duplicate end: got count=%d, want 0", count)
	}
}

func TestNetworkBuffer_InflightSurvivesEviction(t *testing.T) {
	// Ring buffer holds 2 entries, but inflight tracking is independent.
	buf := NewNetworkBuffer(2)

	for i, id := range []string{"r1", "r2", "r3"} {
		buf.MarkRequestStart(id)
		buf.Add(NetworkEntry{RequestID: id, URL: "https://example.com", Method: "GET"})
		_ = i
	}

	// All three are in flight even though the ring has evicted r1.
	count, _ := buf.InflightStatus()
	if count != 3 {
		t.Errorf("after eviction: got count=%d, want 3", count)
	}

	buf.MarkRequestEnd("r1")
	count, _ = buf.InflightStatus()
	if count != 2 {
		t.Errorf("after evicted-end: got count=%d, want 2", count)
	}
}

func TestNetworkBuffer_ClearPreservesInflight(t *testing.T) {
	buf := NewNetworkBuffer(10)
	buf.MarkRequestStart("r1")
	buf.Add(NetworkEntry{RequestID: "r1", URL: "https://example.com"})

	buf.Clear()

	count, _ := buf.InflightStatus()
	if count != 1 {
		t.Errorf("Clear must not reset inflight: got count=%d, want 1", count)
	}
}
