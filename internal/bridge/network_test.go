package bridge

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

const (
	testMaxNetworkURLBytes         = 8 * 1024
	testMaxNetworkPostDataBytes    = 64 * 1024
	testMaxNetworkHeaderTotalBytes = 32 * 1024
)

func TestNetworkBuffer_AddAndGet(t *testing.T) {
	buf := NewNetworkBuffer(3)

	buf.Add(NetworkEntry{RequestID: "r1", URL: "https://example.com/a", Method: "GET"})
	buf.Add(NetworkEntry{RequestID: "r2", URL: "https://example.com/b", Method: "POST"})

	if buf.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", buf.Len())
	}

	e, ok := buf.Get("r1")
	if !ok {
		t.Fatal("expected to find r1")
	}
	if e.URL != "https://example.com/a" {
		t.Errorf("expected URL https://example.com/a, got %s", e.URL)
	}
}

func TestNetworkBuffer_Eviction(t *testing.T) {
	buf := NewNetworkBuffer(2)

	buf.Add(NetworkEntry{RequestID: "r1", URL: "https://example.com/1"})
	buf.Add(NetworkEntry{RequestID: "r2", URL: "https://example.com/2"})
	buf.Add(NetworkEntry{RequestID: "r3", URL: "https://example.com/3"})

	if buf.Len() != 2 {
		t.Fatalf("expected 2 entries after eviction, got %d", buf.Len())
	}

	if _, ok := buf.Get("r1"); ok {
		t.Error("r1 should have been evicted")
	}
	if _, ok := buf.Get("r2"); !ok {
		t.Error("r2 should still exist")
	}
	if _, ok := buf.Get("r3"); !ok {
		t.Error("r3 should exist")
	}
}

func TestNetworkBuffer_Update(t *testing.T) {
	buf := NewNetworkBuffer(10)
	buf.Add(NetworkEntry{RequestID: "r1", URL: "https://example.com", Method: "GET"})

	buf.Update("r1", func(e *NetworkEntry) {
		e.Status = 200
		e.Finished = true
	})

	e, ok := buf.Get("r1")
	if !ok {
		t.Fatal("expected to find r1")
	}
	if e.Status != 200 {
		t.Errorf("expected status 200, got %d", e.Status)
	}
	if !e.Finished {
		t.Error("expected finished to be true")
	}
}

func TestNetworkBuffer_TruncatesOversizedFields(t *testing.T) {
	buf := NewNetworkBuffer(10)
	buf.Add(NetworkEntry{
		RequestID: "r1",
		URL:       "https://example.com/" + strings.Repeat("a", testMaxNetworkURLBytes),
		PostData:  strings.Repeat("b", testMaxNetworkPostDataBytes+1024),
		RequestHeaders: map[string]string{
			"X-Test": strings.Repeat("c", testMaxNetworkHeaderTotalBytes),
		},
	})

	entry, ok := buf.Get("r1")
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if len(entry.URL) > testMaxNetworkURLBytes {
		t.Fatalf("URL length = %d, want <= %d", len(entry.URL), testMaxNetworkURLBytes)
	}
	if len(entry.PostData) > testMaxNetworkPostDataBytes {
		t.Fatalf("PostData length = %d, want <= %d", len(entry.PostData), testMaxNetworkPostDataBytes)
	}
	totalHeaderBytes := 0
	for key, value := range entry.RequestHeaders {
		totalHeaderBytes += len(key) + len(value)
	}
	if totalHeaderBytes > testMaxNetworkHeaderTotalBytes {
		t.Fatalf("header bytes = %d, want <= %d", totalHeaderBytes, testMaxNetworkHeaderTotalBytes)
	}
}

func TestNetworkBuffer_Clear(t *testing.T) {
	buf := NewNetworkBuffer(10)
	buf.Add(NetworkEntry{RequestID: "r1"})
	buf.Add(NetworkEntry{RequestID: "r2"})
	buf.Clear()

	if buf.Len() != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", buf.Len())
	}
}

func TestNetworkFilter_Match(t *testing.T) {
	entry := NetworkEntry{
		RequestID:    "r1",
		URL:          "https://api.example.com/users",
		Method:       "POST",
		Status:       404,
		ResourceType: "XHR",
	}

	tests := []struct {
		name   string
		filter NetworkFilter
		want   bool
	}{
		{"empty filter matches all", NetworkFilter{}, true},
		{"url match", NetworkFilter{URLPattern: "api.example"}, true},
		{"url no match", NetworkFilter{URLPattern: "other.com"}, false},
		{"method match", NetworkFilter{Method: "POST"}, true},
		{"method no match", NetworkFilter{Method: "GET"}, false},
		{"method case insensitive", NetworkFilter{Method: "post"}, true},
		{"status exact match", NetworkFilter{StatusRange: "404"}, true},
		{"status exact no match", NetworkFilter{StatusRange: "200"}, false},
		{"status range match", NetworkFilter{StatusRange: "4xx"}, true},
		{"status range no match", NetworkFilter{StatusRange: "2xx"}, false},
		{"type match", NetworkFilter{ResourceType: "xhr"}, true},
		{"type no match", NetworkFilter{ResourceType: "document"}, false},
		{"combined match", NetworkFilter{Method: "POST", StatusRange: "4xx"}, true},
		{"combined partial no match", NetworkFilter{Method: "GET", StatusRange: "4xx"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Match(entry)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkBuffer_List_WithFilter(t *testing.T) {
	buf := NewNetworkBuffer(10)
	buf.Add(NetworkEntry{RequestID: "r1", URL: "https://api.com/a", Method: "GET", Status: 200, ResourceType: "XHR"})
	buf.Add(NetworkEntry{RequestID: "r2", URL: "https://api.com/b", Method: "POST", Status: 404, ResourceType: "XHR"})
	buf.Add(NetworkEntry{RequestID: "r3", URL: "https://cdn.com/style.css", Method: "GET", Status: 200, ResourceType: "Stylesheet"})

	// Filter by method
	entries := buf.List(NetworkFilter{Method: "POST"})
	if len(entries) != 1 || entries[0].RequestID != "r2" {
		t.Errorf("expected 1 POST entry, got %d", len(entries))
	}

	// Filter by status range
	entries = buf.List(NetworkFilter{StatusRange: "4xx"})
	if len(entries) != 1 || entries[0].RequestID != "r2" {
		t.Errorf("expected 1 4xx entry, got %d", len(entries))
	}

	// Filter by type
	entries = buf.List(NetworkFilter{ResourceType: "xhr"})
	if len(entries) != 2 {
		t.Errorf("expected 2 XHR entries, got %d", len(entries))
	}

	// No filter
	entries = buf.List(NetworkFilter{})
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestNetworkMonitor_GetBuffer(t *testing.T) {
	nm := NewNetworkMonitor(50)

	// No buffer yet
	if buf := nm.GetBuffer("tab1"); buf != nil {
		t.Error("expected nil buffer for unknown tab")
	}

	// Create buffer via getOrCreateBuffer
	buf := nm.GetOrCreateBufferForTest("tab1")
	if buf == nil {
		t.Fatal("expected non-nil buffer")
	}

	// Should return same buffer
	buf2 := nm.GetBuffer("tab1")
	if buf2 != buf {
		t.Error("expected same buffer instance")
	}
}

func TestNetworkMonitor_ClearTab(t *testing.T) {
	nm := NewNetworkMonitor(50)
	buf := nm.GetOrCreateBufferForTest("tab1")
	buf.Add(NetworkEntry{RequestID: "r1"})

	nm.ClearTab("tab1")
	if buf.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", buf.Len())
	}
}

func TestNetworkMonitor_ClearAll(t *testing.T) {
	nm := NewNetworkMonitor(50)
	buf1 := nm.GetOrCreateBufferForTest("tab1")
	buf2 := nm.GetOrCreateBufferForTest("tab2")
	buf1.Add(NetworkEntry{RequestID: "r1"})
	buf2.Add(NetworkEntry{RequestID: "r2"})

	nm.ClearAll()
	if buf1.Len() != 0 || buf2.Len() != 0 {
		t.Error("expected all buffers cleared")
	}
}

func TestMatchStatusRange(t *testing.T) {
	tests := []struct {
		status  int
		pattern string
		want    bool
	}{
		{200, "200", true},
		{200, "201", false},
		{200, "2xx", true},
		{404, "4xx", true},
		{500, "5xx", true},
		{301, "3xx", true},
		{200, "4xx", false},
		{0, "", true},
	}
	for _, tt := range tests {
		got := matchStatusRange(tt.status, tt.pattern)
		if got != tt.want {
			t.Errorf("matchStatusRange(%d, %q) = %v, want %v", tt.status, tt.pattern, got, tt.want)
		}
	}
}

func TestNetworkBuffer_Subscribe(t *testing.T) {
	buf := NewNetworkBuffer(10)
	subID, ch := buf.Subscribe()
	defer buf.Unsubscribe(subID)

	go func() {
		buf.Add(NetworkEntry{RequestID: "r1", URL: "https://example.com", Method: "GET"})
	}()

	select {
	case entry := <-ch:
		if entry.RequestID != "r1" {
			t.Errorf("expected r1, got %s", entry.RequestID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscriber notification")
	}
}

func TestNetworkBuffer_SubscribeMultiple(t *testing.T) {
	buf := NewNetworkBuffer(10)
	id1, ch1 := buf.Subscribe()
	id2, ch2 := buf.Subscribe()
	defer buf.Unsubscribe(id1)
	defer buf.Unsubscribe(id2)

	buf.Add(NetworkEntry{RequestID: "r1", Method: "GET"})

	for _, ch := range []<-chan NetworkEntry{ch1, ch2} {
		select {
		case entry := <-ch:
			if entry.RequestID != "r1" {
				t.Errorf("expected r1, got %s", entry.RequestID)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for subscriber")
		}
	}
}

func TestNetworkBuffer_SubscribeNoNotifyOnUpdate(t *testing.T) {
	buf := NewNetworkBuffer(10)
	buf.Add(NetworkEntry{RequestID: "r1", Method: "GET"})

	subID, ch := buf.Subscribe()
	defer buf.Unsubscribe(subID)

	// Re-adding same requestId is an update, not a new entry — no notification
	buf.Add(NetworkEntry{RequestID: "r1", Method: "POST"})

	select {
	case <-ch:
		t.Fatal("should not receive notification for update")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestNetworkBuffer_Unsubscribe(t *testing.T) {
	buf := NewNetworkBuffer(10)
	subID, ch := buf.Subscribe()
	buf.Unsubscribe(subID)

	buf.Add(NetworkEntry{RequestID: "r1"})

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	case <-time.After(50 * time.Millisecond):
		// also acceptable — closed channel returns immediately
	}
}

func TestNetworkMonitor_GetOrCreateBufferWithSize(t *testing.T) {
	nm := NewNetworkMonitor(50)

	// Custom size
	buf := nm.GetOrCreateBufferWithSizeForTest("tab1", 200)
	if buf == nil {
		t.Fatal("expected non-nil buffer")
	}
	if buf.MaxSizeForTest() != 200 {
		t.Errorf("expected maxSize 200, got %d", buf.MaxSizeForTest())
	}

	// Same tab returns existing buffer (doesn't resize)
	buf2 := nm.GetOrCreateBufferWithSizeForTest("tab1", 500)
	if buf2 != buf {
		t.Error("expected same buffer instance")
	}

	// Zero size uses default
	buf3 := nm.GetOrCreateBufferWithSizeForTest("tab2", 0)
	if buf3.MaxSizeForTest() != 50 {
		t.Errorf("expected default maxSize 50, got %d", buf3.MaxSizeForTest())
	}
}

func TestNewNetworkBuffer_CustomSize(t *testing.T) {
	buf := NewNetworkBuffer(500)
	if buf.MaxSizeForTest() != 500 {
		t.Errorf("expected maxSize 500, got %d", buf.MaxSizeForTest())
	}

	// Add more than default (100) entries to verify custom size works
	for i := 0; i < 200; i++ {
		buf.Add(NetworkEntry{RequestID: fmt.Sprintf("r%d", i)})
	}
	if buf.Len() != 200 {
		t.Errorf("expected 200 entries with buffer size 500, got %d", buf.Len())
	}
}

func TestNewNetworkBuffer_ZeroDefaultsTo100(t *testing.T) {
	buf := NewNetworkBuffer(0)
	if buf.MaxSizeForTest() != DefaultNetworkBufferSize {
		t.Errorf("expected maxSize %d, got %d", DefaultNetworkBufferSize, buf.MaxSizeForTest())
	}
}

func TestNewNetworkBuffer_ClampsOversizedBuffer(t *testing.T) {
	buf := NewNetworkBuffer(config.MaxNetworkBufferSize + 1)
	if buf.MaxSizeForTest() != config.MaxNetworkBufferSize {
		t.Errorf("expected maxSize %d, got %d", config.MaxNetworkBufferSize, buf.MaxSizeForTest())
	}
}

func TestNewNetworkMonitor_ClampsOversizedBuffer(t *testing.T) {
	nm := NewNetworkMonitor(config.MaxNetworkBufferSize + 1)
	if nm.BufferSizeForTest() != config.MaxNetworkBufferSize {
		t.Errorf("expected bufSize %d, got %d", config.MaxNetworkBufferSize, nm.BufferSizeForTest())
	}
}
