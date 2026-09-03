package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
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

// A body cut at exactly the cap is indistinguishable from a whole one, and the
// consumer here is a model: a truncated 10 MB base64 image arrived as a
// successful text block that nothing on the path could see was incomplete.
func TestAnOversizeResponseIsRefusedRatherThanTruncated(t *testing.T) {
	const sent = MaxResponseBytes + (1 << 20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"format":"png","base64":"`)
		_, _ = w.Write(bytes.Repeat([]byte("A"), sent))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	body, code, err := c.Get(context.Background(), "/screenshot", nil)

	if err == nil {
		t.Fatalf("an oversize response was accepted: %d bytes, code %d", len(body), code)
	}
	if len(body) != 0 {
		t.Errorf("a refused read still returned %d bytes; a partial body must never reach a caller", len(body))
	}
	for _, want := range []string{"/screenshot", "10485760", "beyondViewport=false"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not carry %q — an agent that cannot act on it retries the same call", err, want)
		}
	}
}

// A body exactly at the limit is whole and must still be served: reading limit+1
// is a detection scheme, not a smaller limit.
func TestAResponseExactlyAtTheLimitIsServed(t *testing.T) {
	payload := bytes.Repeat([]byte("B"), MaxResponseBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	body, _, err := NewClient(srv.URL, "").Get(context.Background(), "/text", nil)
	if err != nil {
		t.Fatalf("a body at exactly the limit was refused: %v", err)
	}
	if len(body) != MaxResponseBytes {
		t.Errorf("read %d bytes, want the whole %d", len(body), MaxResponseBytes)
	}
}

// The whole path, as the agent sees it: the tool must answer with an error
// result, and must not spend the model's context on megabytes of truncated
// base64 dressed as text.
func TestTheScreenshotToolReportsAnOversizeResponseAsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"format":"png","base64":"`)
		_, _ = w.Write(bytes.Repeat([]byte("A"), MaxResponseBytes+(1<<20)))
	}))
	defer srv.Close()

	result, err := handleScreenshot(NewClient(srv.URL, ""))(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned a transport error instead of a tool result: %v", err)
	}
	if !result.IsError {
		t.Fatal("the oversize response reached the agent as a success")
	}
	encoded, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > maxEchoedScreenshotBytes {
		t.Errorf("the result carries %d bytes of content; a refusal must not echo the payload it refused", len(encoded))
	}
}

// The fallback exists for a small error envelope or an unmodelled field, and
// still serves that case — the refusal above must not have swallowed it.
func TestTheScreenshotFallbackStillSurfacesASmallUnparseableBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"error":"screenshot failed: tab crashed"}`)
	}))
	defer srv.Close()

	result, err := handleScreenshot(NewClient(srv.URL, ""))(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "tab crashed") {
		t.Errorf("the small-envelope fallback stopped surfacing the server's reason: %s", encoded)
	}
}

// Below the client's limit and above an envelope: a damaged image payload that
// does not parse. The old fallback echoed it verbatim as text, which is the same
// defect one layer down from the truncation this card removes.
func TestTheScreenshotFallbackRefusesABodyTooLargeToBeAnEnvelope(t *testing.T) {
	damaged := append([]byte(`{"format":"png","base64":"`), bytes.Repeat([]byte("A"), 100<<10)...)

	result, err := screenshotResult(damaged, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("a damaged multi-kilobyte payload was reported as a success")
	}
	encoded, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > maxEchoedScreenshotBytes {
		t.Errorf("the refusal echoed %d bytes of the payload it refused", len(encoded))
	}
	if !strings.Contains(string(encoded), "beyondViewport=false") {
		t.Errorf("the refusal does not tell the agent how to ask for less: %s", encoded)
	}
}
