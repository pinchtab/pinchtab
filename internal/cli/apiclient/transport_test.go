package apiclient

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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
