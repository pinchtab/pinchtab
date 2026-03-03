package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewDashboard(t *testing.T) {
	d := NewDashboard(nil)
	if d == nil {
		t.Fatal("expected non-nil dashboard")
	}
}

func TestDashboardBroadcastSystemEvent(t *testing.T) {
	d := NewDashboard(nil)

	// Create a test handler and register it
	mux := http.NewServeMux()
	d.RegisterHandlers(mux)

	// In a real scenario, a client would be connected to /api/events
	// For this test, we just verify the broadcast method doesn't panic
	evt := SystemEvent{
		Type: "instance.started",
	}
	d.BroadcastSystemEvent(evt)
}

func TestDashboardSSEHandlerRegistration(t *testing.T) {
	d := NewDashboard(nil)
	mux := http.NewServeMux()
	d.RegisterHandlers(mux)

	// Verify the SSE handler is registered by checking the mux
	// (can't easily test the full SSE flow with httptest due to streaming nature)
	// Just verify handlers are registered without error
}

func TestDashboardShutdown(t *testing.T) {
	d := NewDashboard(nil)
	// Just verify it doesn't panic
	d.Shutdown()
}

func TestDashboardSetInstanceLister(t *testing.T) {
	d := NewDashboard(nil)
	d.SetInstanceLister(nil)
	// Just verify it doesn't panic
}

func TestDashboardCacheHeaders(t *testing.T) {
	d := NewDashboard(nil)

	// Test long cache (assets)
	handler := d.withLongCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/assets/app.js", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl != "public, max-age=31536000, immutable" {
		t.Errorf("expected long cache header, got %q", cacheControl)
	}

	// Test no cache (HTML)
	handler = d.withNoCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req = httptest.NewRequest("GET", "/dashboard", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	cacheControl = w.Header().Get("Cache-Control")
	if cacheControl != "no-store" {
		t.Errorf("expected no-store cache header, got %q", cacheControl)
	}
}

func TestDashboardShutdownTimeout(t *testing.T) {
	d := NewDashboard(&DashboardConfig{
		IdleTimeout:       10 * time.Millisecond,
		DisconnectTimeout: 20 * time.Millisecond,
		ReaperInterval:    5 * time.Millisecond,
		SSEBufferSize:     8,
	})

	d.Shutdown()
	time.Sleep(50 * time.Millisecond) // Verify shutdown completes
}
