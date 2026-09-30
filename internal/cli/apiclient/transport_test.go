package apiclient

import (
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// A connection that drops mid-body used to reach the caller as a fragment
// carrying the response's status and no error, and the CLI parsed or printed the
// fragment as the answer. One rule, shared with the MCP client: a short read must
// never become a successful body.
func TestAShortReadIsAFailedRequestRatherThanAPartialBody(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 4096)
		_, _ = conn.Read(buf)
		// A length the body never reaches, then the connection dies.
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 4096\r\n\r\n"))
		_, _ = conn.Write([]byte(`{"tabs":[`))
		_ = conn.Close()
	}()

	url := fmt.Sprintf("http://%s/tabs", listener.Addr().String())
	code, body, err := doRequest(http.DefaultClient, "", request{method: http.MethodGet, url: url})

	if err == nil {
		t.Fatalf("a truncated response was reported as a success: code %d, %d bytes (%q)", code, len(body), string(body))
	}
	if len(body) != 0 {
		t.Errorf("the failed read still handed back %d bytes for the caller to parse", len(body))
	}
	if !strings.Contains(err.Error(), "/tabs") {
		t.Errorf("the error %q does not name the request that failed", err)
	}
}

// The whole body of a healthy response still comes back, so the guard above is a
// failure rule and not a smaller limit.
func TestACompleteResponseIsStillReturnedWhole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tabs":[]}`)
	}))
	defer srv.Close()

	code, body, err := doRequest(http.DefaultClient, "", request{method: http.MethodGet, url: srv.URL + "/tabs"})
	if err != nil {
		t.Fatalf("a complete response was refused: %v", err)
	}
	if code != http.StatusOK || string(body) != `{"tabs":[]}` {
		t.Errorf("code %d body %q, want 200 and the whole body", code, string(body))
	}
}

func TestAnUnbuildableRequestFailsBeforeTheWire(t *testing.T) {
	var arrived atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived.Add(1)
	}))
	defer srv.Close()

	for name, call := range map[string]func() error{
		"malformed base": func() error {
			_, err := DoRawE(srv.Client(), "http://bad host:9867", "", http.MethodPost, "/action", WithBody(map[string]any{"kind": "click"}))
			return err
		},
		"unencodable body": func() error {
			_, err := DoRawE(srv.Client(), srv.URL, "", http.MethodPost, "/geolocation", WithBody(map[string]any{"latitude": math.NaN(), "longitude": 0.0}))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("the call succeeded; it must refuse a request it could not build")
			}
			if got := arrived.Load(); got != 0 {
				t.Fatalf("%d request(s) reached the server", got)
			}
		})
	}
}

func TestCaptureVocabStoresOnlyASuccessfulResponsesToken(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   string
	}{
		{"success", http.StatusOK, "ep_new"},
		{"refused", http.StatusConflict, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(vocabTabIDHeader, "tab1")
				w.Header().Set(vocabHeader, "ep_new")
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			r := newRequest(http.MethodPost, srv.URL, "/find", WithBody(map[string]any{"query": "x"}), CaptureVocab(true))
			if _, _, err := doRequest(srv.Client(), "", r); err != nil {
				t.Fatal(err)
			}
			if got := VocabTokenFor(srv.URL, "tab1"); got != tc.want {
				t.Errorf("stored token = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTabPath(t *testing.T) {
	tests := []struct {
		tabID, suffix, want string
	}{
		{"a/b", "find", "/tabs/a%2Fb/find"},
		{"", "find", "/find"},
		{"tab1", "emulation/viewport", "/tabs/tab1/emulation/viewport"},
		{"tab1", "/text", "/tabs/tab1/text"},
		{"", "/text", "/text"},
	}
	for _, tt := range tests {
		if got := TabPath(tt.tabID, tt.suffix); got != tt.want {
			t.Errorf("TabPath(%q, %q) = %q, want %q", tt.tabID, tt.suffix, got, tt.want)
		}
	}
}
