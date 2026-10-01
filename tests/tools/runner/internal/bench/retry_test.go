package bench

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetrySuccessOnFirstAttempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	cfg := DefaultRetryConfig()
	cfg.Sleep = func(d time.Duration) {}

	var callCount int32
	resp, err := DoWithRetry(context.Background(), cfg, func() (*http.Response, error) {
		atomic.AddInt32(&callCount, 1)
		return http.Get(server.URL)
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if callCount != 1 {
		t.Errorf("callCount = %d; want 1", callCount)
	}
}

func TestRetryOn429(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	cfg := DefaultRetryConfig()
	var totalSleep time.Duration
	cfg.Sleep = func(d time.Duration) { totalSleep += d }

	resp, err := DoWithRetry(context.Background(), cfg, func() (*http.Response, error) {
		return http.Get(server.URL)
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if callCount != 3 {
		t.Errorf("callCount = %d; want 3", callCount)
	}
	if totalSleep < 2*time.Second {
		t.Errorf("totalSleep = %v; want >= 2s", totalSleep)
	}
}

func TestRetryExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()

	cfg := DefaultRetryConfig()
	cfg.Sleep = func(d time.Duration) {}

	var callCount int32
	resp, err := DoWithRetry(context.Background(), cfg, func() (*http.Response, error) {
		atomic.AddInt32(&callCount, 1)
		return http.Get(server.URL)
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 429 {
		t.Errorf("status = %d; want final 429 handed to caller", resp.StatusCode)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d; want 3", callCount)
	}
}

func TestNoRetryOn400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
	}))
	defer server.Close()

	cfg := DefaultRetryConfig()
	cfg.Sleep = func(d time.Duration) {}

	var callCount int32
	resp, err := DoWithRetry(context.Background(), cfg, func() (*http.Response, error) {
		atomic.AddInt32(&callCount, 1)
		return http.Get(server.URL)
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if callCount != 1 {
		t.Errorf("callCount = %d; want 1 (no retry on 400)", callCount)
	}
	if resp.StatusCode != 400 {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
}

func TestParseRetryAfterSeconds(t *testing.T) {
	now := time.Now()
	d := parseRetryAfter("5", now)
	if d != 5*time.Second {
		t.Errorf("got %v; want 5s", d)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	future := now.Add(30 * time.Second)
	d := parseRetryAfter(future.Format(http.TimeFormat), now)
	if d < 29*time.Second || d > 31*time.Second {
		t.Errorf("got %v; want ~30s", d)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := DefaultRetryConfig()
	cfg.Sleep = func(d time.Duration) {}

	_, err := DoWithRetry(ctx, cfg, func() (*http.Response, error) {
		return http.Get(server.URL)
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v; want context.Canceled", err)
	}
}
