package actions

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newCookiesTestCmd(args ...string) *cobra.Command {
	cmd := &cobra.Command{Use: "cookies"}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("domain", "", "")
	cmd.Flags().String("path", "", "")
	cmd.Flags().String("same-site", "", "")
	cmd.Flags().Bool("secure", false, "")
	cmd.Flags().Bool("http-only", false, "")
	cmd.Flags().Bool("json", false, "")
	if err := cmd.Flags().Parse(args); err != nil {
		panic(err)
	}
	return cmd
}

// cookieJarServer is the pair of routes the two verbs drive, sharing one jar so a
// read observes what the write stored — the round trip is the point, not each call.
func cookieJarServer(t *testing.T, jar map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cookies" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPost:
			var body struct {
				Cookies []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"cookies"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode set body: %v", err)
				return
			}
			for _, c := range body.Cookies {
				jar[c.Name] = c.Value
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"set": len(body.Cookies), "failed": 0, "total": len(body.Cookies)})
		case http.MethodGet:
			cookies := make([]map[string]any, 0, len(jar))
			for name, value := range jar {
				if filter := r.URL.Query().Get("name"); filter != "" && filter != name {
					continue
				}
				cookies = append(cookies, map[string]any{"name": name, "value": value})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"cookies": cookies, "count": len(cookies)})
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
}

// The gap this closes was the CLI wiring only a subset of the routes, so the test is
// a CLI write read back by a CLI read: neither verb can regress to nothing without
// this failing.
func TestCookiesSetThenGetRoundTripsThroughTheCLI(t *testing.T) {
	jar := map[string]string{}
	srv := cookieJarServer(t, jar)
	defer srv.Close()

	CookiesSet(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newCookiesTestCmd("--tab", "tab1"), "session", "abc123")
	if jar["session"] != "abc123" {
		t.Fatalf("jar = %v, want the cookie the CLI set", jar)
	}

	out := captureStdout(t, func() {
		CookiesGet(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newCookiesTestCmd("--tab", "tab1", "--name", "session"))
	})
	if !strings.Contains(out, "abc123") || !strings.Contains(out, "session") {
		t.Fatalf("cookies get printed %q, want the value that was just set", out)
	}
}

// An empty value blanks a cookie, and the CLI must send it rather than treat it as an
// omitted argument.
// The assertion is on the key's PRESENCE in the wire body: a jar that decodes into a
// struct cannot tell an omitted "value" from an empty one, so it would pass either way.
func TestCookiesSetSendsAnEmptyValue(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"set": 1, "failed": 0, "total": 1})
	}))
	defer srv.Close()

	CookiesSet(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newCookiesTestCmd(), "session", "")

	cookies, ok := body["cookies"].([]any)
	if !ok || len(cookies) != 1 {
		t.Fatalf("body = %+v, want one cookie", body)
	}
	cookie, _ := cookies[0].(map[string]any)
	value, present := cookie["value"]
	if !present {
		t.Fatalf("cookie = %+v carries no value key; blanking a cookie is not the same as omitting the value", cookie)
	}
	if value != "" {
		t.Errorf("value = %v, want the empty string the caller passed", value)
	}
}

// The set verb forwards only the attributes the caller gave, so an unset flag does not
// overwrite a server-side default with an empty string.
func TestCookiesSetForwardsOnlyTheFlagsGiven(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"set": 1, "failed": 0, "total": 1})
	}))
	defer srv.Close()

	cmd := newCookiesTestCmd("--domain", "example.com", "--secure", "--tab", "tab1")
	CookiesSet(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, cmd, "session", "abc")

	cookies, ok := body["cookies"].([]any)
	if !ok || len(cookies) != 1 {
		t.Fatalf("body = %+v, want one cookie", body)
	}
	cookie, _ := cookies[0].(map[string]any)
	if cookie["domain"] != "example.com" || cookie["secure"] != true {
		t.Errorf("cookie = %+v, want the domain and secure flag forwarded", cookie)
	}
	for _, absent := range []string{"path", "sameSite", "httpOnly"} {
		if _, present := cookie[absent]; present {
			t.Errorf("cookie carries %q the caller never set: %+v", absent, cookie)
		}
	}
	if _, present := body["url"]; present {
		t.Errorf("body pins a url the caller never gave (%v); the server defaults it to the tab's page", body["url"])
	}
}

// countJSONObjects reports how many top-level JSON objects are concatenated in s.
// A double-print emits two, which is what breaks `jq` / `json.tool` on the pipe.
func countJSONObjects(t *testing.T, s string) int {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	n := 0
	for {
		var v map[string]any
		err := dec.Decode(&v)
		if err == io.EOF {
			return n
		}
		if err != nil {
			t.Fatalf("stdout is not clean JSON (%v): %q", err, s)
		}
		n++
	}
}

// cookieWriteServer answers POST and DELETE /cookies with the confirmation bodies
// the two verbs render, so the test observes exactly what reaches stdout.
func cookieWriteServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = io.Copy(io.Discard, r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{"set": 1, "failed": 0, "total": 1})
		case http.MethodDelete:
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "cleared"})
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
}

// The regression: set/clear printed the response body twice (the auto-rendering
// helper AND printCookiesResult), so --json emitted two objects and terse printed
// the JSON before its status line. Each path must now emit exactly one thing.
func TestCookiesSetAndClearPrintExactlyOnce(t *testing.T) {
	srv := cookieWriteServer(t)
	defer srv.Close()

	t.Run("set --json is one object", func(t *testing.T) {
		out := captureStdout(t, func() {
			CookiesSet(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newCookiesTestCmd("--json"), "z1", "v1")
		})
		if n := countJSONObjects(t, out); n != 1 {
			t.Errorf("set --json emitted %d JSON objects, want exactly 1: %q", n, out)
		}
	})

	t.Run("set terse is only the status line", func(t *testing.T) {
		out := captureStdout(t, func() {
			CookiesSet(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newCookiesTestCmd(), "z2", "v2")
		})
		if strings.Contains(out, "{") {
			t.Errorf("terse set printed a JSON body before its status line: %q", out)
		}
		if strings.TrimSpace(out) != "OK" {
			t.Errorf("terse set printed %q, want only OK", out)
		}
	})

	t.Run("clear --json is one object", func(t *testing.T) {
		out := captureStdout(t, func() {
			CookiesClear(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newCookiesTestCmd("--json"))
		})
		if n := countJSONObjects(t, out); n != 1 {
			t.Errorf("clear --json emitted %d JSON objects, want exactly 1: %q", n, out)
		}
	})

	t.Run("clear terse is only the status line", func(t *testing.T) {
		out := captureStdout(t, func() {
			CookiesClear(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newCookiesTestCmd())
		})
		if strings.Contains(out, "{") {
			t.Errorf("terse clear printed a JSON body before its status line: %q", out)
		}
		if strings.TrimSpace(out) != "OK" {
			t.Errorf("terse clear printed %q, want only OK", out)
		}
	})
}

// The confirmation must fail CLOSED. A response that cannot say how many cookies were
// stored has not confirmed the write, and a check written as "if the field is readable AND
// looks bad" silently retires itself the moment the field is renamed.
func TestCookieWriteConfirmedFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result map[string]any
		want   bool
	}{
		{name: "one stored", result: map[string]any{"set": float64(1), "total": float64(1)}, want: true},
		{name: "nothing stored", result: map[string]any{"set": float64(0), "total": float64(1)}},
		{name: "field absent", result: map[string]any{"status": "ok"}},
		{name: "field renamed", result: map[string]any{"stored": float64(1)}},
		{name: "field not a number", result: map[string]any{"set": "1"}},
		{name: "empty response", result: map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cookieWriteConfirmed(tc.result); got != tc.want {
				t.Errorf("cookieWriteConfirmed(%v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}
