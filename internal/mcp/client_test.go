package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:9867", "tok123")
	if c.BaseURL != "http://localhost:9867" {
		t.Fatalf("BaseURL = %q, want %q", c.BaseURL, "http://localhost:9867")
	}
	if c.Token != "tok123" {
		t.Fatalf("Token = %q, want %q", c.Token, "tok123")
	}
	if c.HTTPClient == nil {
		t.Fatal("HTTPClient is nil")
	}
}

func TestClientGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer testtoken" {
			t.Errorf("no auth header")
		}
		if r.URL.Query().Get("tabId") != "t1" {
			t.Errorf("tabId = %q, want t1", r.URL.Query().Get("tabId"))
		}
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "testtoken")
	body, code, err := c.Get(context.Background(), "/health", url.Values{"tabId": {"t1"}})
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 {
		t.Fatalf("code = %d, want 200", code)
	}
	if !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("body = %q", body)
	}
}

func TestClientGetNoQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	_, code, err := c.Get(context.Background(), "/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
}

func TestClientPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"url"`) {
			t.Errorf("body missing url field: %s", body)
		}
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"navigated":true}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	body, code, err := c.Post(context.Background(), "/navigate", map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(string(body), "navigated") {
		t.Fatalf("body = %q", body)
	}
}

func TestClientPostNilPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Errorf("expected empty body, got %s", body)
		}
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	_, code, err := c.Post(context.Background(), "/shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
}

func TestClientPostStreamReturnsReadableBody(t *testing.T) {
	payload := strings.Repeat("X", 64*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	body, code, err := c.PostStream(context.Background(), "/record/stop", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	data, _ := io.ReadAll(body)
	if len(data) != len(payload) {
		t.Fatalf("read %d bytes, want %d", len(data), len(payload))
	}
}

func TestClientAuthHeaderAbsentWhenNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Authorization"); h != "" {
			t.Errorf("unexpected Authorization header: %s", h)
		}
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	_, _, err := c.Get(context.Background(), "/health", nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientProfileInstancePath(t *testing.T) {
	c := NewClient("http://localhost:9867", "")
	got := c.profileInstancePath("work profile")
	want := "/profiles/work%20profile/instance"
	if got != want {
		t.Fatalf("profileInstancePath = %q, want %q", got, want)
	}
}

func TestClientDashboardProfilesURL(t *testing.T) {
	c := NewClient("http://localhost:9867/", "")
	got := c.dashboardProfilesURL()
	want := "http://localhost:9867/dashboard/profiles"
	if got != want {
		t.Fatalf("dashboardProfilesURL = %q, want %q", got, want)
	}
}
