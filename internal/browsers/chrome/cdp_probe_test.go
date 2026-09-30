package chrome

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeCDPVersionWithRetryReturnsPromptlyOnCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	time.AfterFunc(150*time.Millisecond, cancel)

	start := time.Now()
	_, err := probeCDPVersionWithRetry(ctx, srv.URL+"/json/version")
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if limit := 150*time.Millisecond + cdpProbeInterval + 100*time.Millisecond; elapsed > limit {
		t.Fatalf("returned after %v, want within one interval of cancel (%v)", elapsed, limit)
	}
}

func TestProbeCDPVersionWithRetryReturnsWebSocketURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://127.0.0.1:1/devtools/browser/x"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := probeCDPVersionWithRetry(ctx, srv.URL)
	if err != nil || got != "ws://127.0.0.1:1/devtools/browser/x" {
		t.Fatalf("got %q, %v", got, err)
	}
}
