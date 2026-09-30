package bench

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPostJSONReportsStatusBeforeDecodingNonJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>Bad Gateway</body></html>"))
	}))
	defer server.Close()

	cfg := DefaultRetryConfig()
	cfg.Sleep = func(time.Duration) {}

	_, err := postJSON(context.Background(), server.Client(), cfg, server.URL, nil, map[string]string{"k": "v"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "API error 502") {
		t.Errorf("err = %v; want it to contain %q", err, "API error 502")
	}
}

func TestAnthropicSendSurfacesUpstreamStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>upstream down</html>"))
	}))
	defer server.Close()

	r := NewAnthropicRunner("key", "model", 16, 0, false)
	r.retryCfg.Sleep = func(time.Duration) {}
	r.client = &http.Client{Transport: &testTransport{server.URL}}

	_, err := r.Send("system", r.InitialConversation("hi"))
	if err == nil || !strings.Contains(err.Error(), "anthropic API error 502") {
		t.Errorf("err = %v; want anthropic API error 502", err)
	}
}

func TestPinchtabClientSendsBenchmarkToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer server.Close()

	c := newBenchPinchtabClient(time.Second)
	c.baseURL = server.URL
	if err := c.health(); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer "+benchPinchtabToken {
		t.Errorf("Authorization = %q; want bearer benchmark token", gotAuth)
	}
}
