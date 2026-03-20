package activity

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type captureRecorder struct {
	events []Event
}

func (c *captureRecorder) Enabled() bool { return true }

func (c *captureRecorder) Record(evt Event) error {
	c.events = append(c.events, evt)
	return nil
}

func (c *captureRecorder) Query(Filter) ([]Event, error) {
	return append([]Event(nil), c.events...), nil
}

func TestSanitizeActivityURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "drops query fragment and credentials",
			raw:  "https://user:pass@App.EXAMPLE.com:8443/callback?code=secret#done",
			want: "https://app.example.com:8443/callback",
		},
		{
			name: "normalizes bare hostname",
			raw:  "pinchtab.com/reset?token=secret",
			want: "https://pinchtab.com/reset",
		},
		{
			name: "keeps non-network scheme without fragment",
			raw:  "about:blank#frag",
			want: "about:blank",
		},
		{
			name: "rejects malformed",
			raw:  "://bad-url",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeActivityURL(tt.raw); got != tt.want {
				t.Fatalf("sanitizeActivityURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestMiddlewareSanitizesInitialURL(t *testing.T) {
	rec := &captureRecorder{}
	handler := Middleware(rec, "server", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/navigate?url="+url.QueryEscape("pinchtab.com/reset?token=secret#done"), nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if len(rec.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(rec.events))
	}
	if got := rec.events[0].URL; got != "https://pinchtab.com/reset" {
		t.Fatalf("event.URL = %q, want sanitized URL", got)
	}
}

func TestEnrichRequestSanitizesURL(t *testing.T) {
	rec := &captureRecorder{}
	handler := Middleware(rec, "server", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		EnrichRequest(r, Update{URL: "https://user:pass@example.com/callback?code=secret#frag"})
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/tabs/tab-1/text", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if len(rec.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(rec.events))
	}
	if got := rec.events[0].URL; got != "https://example.com/callback" {
		t.Fatalf("event.URL = %q, want sanitized URL", got)
	}
}
