package observe

import (
	"fmt"
	"testing"
	"time"
)

func TestNetworkBuffer_RingEvictionOrder(t *testing.T) {
	buf := NewNetworkBuffer(3)
	for i := 0; i < 5; i++ {
		buf.Add(NetworkEntry{RequestID: fmt.Sprintf("r%d", i), URL: "https://example.com", Method: "GET"})
	}

	if buf.Len() != 3 {
		t.Fatalf("Len = %d, want 3", buf.Len())
	}

	// Newest 3 in oldest→newest order: r2, r3, r4 (head has wrapped past slot 0).
	list := buf.List(NetworkFilter{})
	want := []string{"r2", "r3", "r4"}
	if len(list) != len(want) {
		t.Fatalf("List len = %d, want %d", len(list), len(want))
	}
	for i, id := range want {
		if list[i].RequestID != id {
			t.Fatalf("list[%d].RequestID = %q, want %q", i, list[i].RequestID, id)
		}
	}

	// Evicted entries are gone; kept entries retrievable.
	for _, id := range []string{"r0", "r1"} {
		if _, ok := buf.Get(id); ok {
			t.Fatalf("evicted %q still present", id)
		}
	}
	for _, id := range want {
		if _, ok := buf.Get(id); !ok {
			t.Fatalf("kept %q missing", id)
		}
	}

	// In-place update of an existing entry must not evict or change size.
	buf.Add(NetworkEntry{RequestID: "r4", URL: "https://example.com/updated", Method: "POST"})
	if buf.Len() != 3 {
		t.Fatalf("Len after in-place update = %d, want 3", buf.Len())
	}
	if got, ok := buf.Get("r4"); !ok || got.Method != "POST" {
		t.Fatalf("in-place update lost: ok=%v method=%q", ok, got.Method)
	}
}

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

func TestNetworkBuffer_ClearResetsRetainedBytes(t *testing.T) {
	buf := NewNetworkBuffer(10)
	buf.Add(NetworkEntry{RequestID: "r1", URL: "https://example.com"})
	buf.Update("r1", func(entry *NetworkEntry) {
		entry.ResponseBody = "retained"
		entry.BodyRetained = true
	})
	if got := buf.RetainedBytes(); got != int64(len("retained")) {
		t.Fatalf("setup retained bytes = %d, want %d", got, len("retained"))
	}

	buf.Clear()

	if got := buf.RetainedBytes(); got != 0 {
		t.Fatalf("retained bytes after clear = %d, want 0", got)
	}
}
