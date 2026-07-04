package apiclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsRouteNotFound(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"mux plain-text 404", 404, "404 page not found\n", true},
		{"application JSON 404", 404, `{"error":"tab not found"}`, false},
		{"JSON body without error field", 404, `{"status":"missing"}`, false},
		{"plain-text 500", 500, "boom", false},
		{"ok", 200, "404 page not found", false},
	}
	for _, tc := range cases {
		if got := isRouteNotFound(tc.status, []byte(tc.body)); got != tc.want {
			t.Errorf("%s: isRouteNotFound = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// An older instance without the /audit route answers with the mux's
// plain-text 404; the rendered error must blame the instance, not the
// audited target site.
func TestRenderAPIErrorForFaked404Instance(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	r := request{method: "POST", url: srv.URL + "/audit", body: map[string]any{"urls": []string{"https://example.com"}}}
	resp, err := http.Post(r.url, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}

	msg := renderAPIError(r, resp.StatusCode, body)
	host := strings.TrimPrefix(srv.URL, "http://")
	for _, want := range []string{host, "POST /audit", "older version", "Restart it with the current binary"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "example.com") {
		t.Errorf("message must not implicate the target site:\n%s", msg)
	}
	if strings.Contains(msg, "404 page not found") {
		t.Errorf("message should replace the bare mux text:\n%s", msg)
	}
}

func TestRenderAPIErrorKeepsApplicationErrors(t *testing.T) {
	r := request{method: "GET", url: "http://127.0.0.1:9867/console"}
	msg := renderAPIError(r, 404, []byte(`{"error":"tab not found: nonexistent_xyz"}`))
	if !strings.Contains(msg, "Error 404: tab not found: nonexistent_xyz") {
		t.Errorf("application 404 rendering changed:\n%s", msg)
	}
	if strings.Contains(msg, "older version") {
		t.Errorf("application 404 must not be treated as a missing route:\n%s", msg)
	}

	msg = renderAPIError(r, 403, []byte(`{"error":"blocked","details":{"hint":"allow the domain"}}`))
	if !strings.Contains(msg, "Error 403: blocked") || !strings.Contains(msg, "allow the domain") {
		t.Errorf("hint rendering changed:\n%s", msg)
	}
}
