package orchestrator

import (
	"net/http"
	"testing"
)

func TestIsInstanceHealthyStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{http.StatusOK, true},
		{http.StatusNotFound, true},
		{http.StatusBadRequest, true},
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
		{0, false},
	}

	for _, tt := range tests {
		if got := isInstanceHealthyStatus(tt.code); got != tt.want {
			t.Errorf("isInstanceHealthyStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestInstanceBaseURLs(t *testing.T) {
	port := "1234"
	urls := instanceBaseURLs("", port)

	expected := []string{
		"http://127.0.0.1:1234",
		"http://[::1]:1234",
		"http://localhost:1234",
	}

	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d", len(expected), len(urls))
	}

	for i, url := range urls {
		if url != expected[i] {
			t.Errorf("url[%d] = %q, want %q", i, url, expected[i])
		}
	}
}

func TestInstanceBaseURLs_UsesCanonicalURLWhenPresent(t *testing.T) {
	urls := instanceBaseURLs("https://bridge.example.com:9868/", "1234")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
	if urls[0] != "https://bridge.example.com:9868" {
		t.Fatalf("url = %q, want %q", urls[0], "https://bridge.example.com:9868")
	}
}
