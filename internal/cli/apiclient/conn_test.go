package apiclient_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

type recordedRequest struct {
	method string
	path   string
	query  url.Values
	auth   string
	body   map[string]any
}

func recordingConn(t *testing.T, reply string) (apiclient.Conn, *recordedRequest) {
	t.Helper()
	got := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.query = r.URL.Query()
		got.auth = r.Header.Get("Authorization")
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &got.body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return apiclient.Conn{Client: srv.Client(), Base: srv.URL, Token: "secret"}, got
}

func TestConnGetSendsQueryAndToken(t *testing.T) {
	conn, got := recordingConn(t, `{"ok":true}`)

	result := conn.Get("/tabs", url.Values{"limit": {"3"}}, apiclient.Quiet())

	if got.method != http.MethodGet || got.path != "/tabs" || got.query.Get("limit") != "3" {
		t.Fatalf("request = %s %s?%s, want GET /tabs?limit=3", got.method, got.path, got.query.Encode())
	}
	if !strings.Contains(got.auth, "secret") {
		t.Fatalf("Authorization = %q, want it to carry the conn token", got.auth)
	}
	if result["ok"] != true {
		t.Fatalf("result = %v, want decoded body", result)
	}
}

func TestConnPostSendsJSONBody(t *testing.T) {
	conn, got := recordingConn(t, `{"clicked":true}`)

	result := conn.Post("/action", map[string]any{"kind": "click"}, apiclient.Quiet())

	if got.method != http.MethodPost || got.path != "/action" {
		t.Fatalf("request = %s %s, want POST /action", got.method, got.path)
	}
	if got.body["kind"] != "click" {
		t.Fatalf("body = %v, want kind=click", got.body)
	}
	if result["clicked"] != true {
		t.Fatalf("result = %v, want decoded body", result)
	}
}

func TestConnRawReturnsBodyForMethod(t *testing.T) {
	conn, got := recordingConn(t, `plain`)

	body, err := conn.Raw(http.MethodDelete, "/tabs/t1")

	if err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodDelete || got.path != "/tabs/t1" || string(body) != "plain" {
		t.Fatalf("request = %s %s -> %q, want DELETE /tabs/t1 -> plain", got.method, got.path, body)
	}
}

func TestConnTabPath(t *testing.T) {
	var conn apiclient.Conn
	cases := []struct{ tab, suffix, want string }{
		{"", "snapshot", "/snapshot"},
		{"t1", "/snapshot", "/tabs/t1/snapshot"},
		{"a/b", "text", "/tabs/a%2Fb/text"},
	}
	for _, c := range cases {
		if got := conn.TabPath(c.tab, c.suffix); got != c.want {
			t.Errorf("TabPath(%q, %q) = %q, want %q", c.tab, c.suffix, got, c.want)
		}
	}
}
