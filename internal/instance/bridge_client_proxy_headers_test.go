package instance

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

func bridgeStub(t *testing.T) (*BridgeClient, string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range httpx.OuterChainResponseHeaders() {
			w.Header().Set(name, "instance-"+name)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return NewBridgeClient(), parsed.Port()
}

func assertOuterChainHeadersSurviveAlone(t *testing.T, header http.Header) {
	t.Helper()

	for _, name := range httpx.OuterChainResponseHeaders() {
		got := header.Values(name)
		if len(got) != 1 {
			t.Errorf("%s = %v, want exactly one value — the instance's own copy is a value the outer process never minted or logged", name, got)
			continue
		}
		if got[0] != "outer-"+name {
			t.Errorf("%s = %q, want the outer chain's value", name, got[0])
		}
	}
	if got := header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want the instance's own response header copied through", got)
	}
}

func recorderWithOuterChainHeaders() *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	for _, name := range httpx.OuterChainResponseHeaders() {
		rec.Header().Set(name, "outer-"+name)
	}
	return rec
}

func TestProxyWithTabIDDoesNotDoubleTheOuterChainsResponseHeaders(t *testing.T) {
	client, port := bridgeStub(t)

	rec := recorderWithOuterChainHeaders()
	req := httptest.NewRequest("POST", "/find", strings.NewReader(`{"text":"Buy"}`))

	client.ProxyWithTabID(rec, req, port, "tab-1", "/find")

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	assertOuterChainHeadersSurviveAlone(t, rec.Header())
}

func TestProxyToTabDoesNotDoubleTheOuterChainsResponseHeaders(t *testing.T) {
	client, port := bridgeStub(t)

	rec := recorderWithOuterChainHeaders()
	req := httptest.NewRequest("GET", "/tabs/tab-1/text", nil)

	client.ProxyToTab(rec, req, port, "tab-1", "/text")

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	assertOuterChainHeadersSurviveAlone(t, rec.Header())
}

func TestProxyToTabForwardsRequestHeadersButNotHopByHopOnes(t *testing.T) {
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/tabs/tab-1/text", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Te", "trailers")

	NewBridgeClient().ProxyToTab(httptest.NewRecorder(), req, parsed.Port(), "tab-1", "/text")

	if got := seen.Get("Authorization"); got != "Bearer token" {
		t.Errorf("Authorization = %q, want it forwarded to the instance", got)
	}
	if got := seen.Get("Te"); got != "" {
		t.Errorf("Te = %q, want the hop-by-hop header dropped on the proxy hop", got)
	}
}

// Both bridge hops must tell the instance the outer chain's request id, or a caller
// quoting it can only be found in the outer log. They arrive at that from opposite
// directions and so are asserted together: ProxyToTab copies the caller's headers and
// carries the id along with them, while ProxyWithTabID re-encodes the body and builds a
// FRESH request, so it tells the instance nothing unless the id is forwarded explicitly.
// That asymmetry is why one hop having it says nothing about the other.
func TestBothBridgeHopsTellTheInstanceTheOuterChainsRequestID(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(client *BridgeClient, rec *httptest.ResponseRecorder, req *http.Request, port string)
	}{
		{
			name: "ProxyWithTabID re-encodes the body and builds a fresh request",
			call: func(client *BridgeClient, rec *httptest.ResponseRecorder, req *http.Request, port string) {
				client.ProxyWithTabID(rec, req, port, "tab-1", "/find")
			},
		},
		{
			name: "ProxyToTab copies the caller's headers",
			call: func(client *BridgeClient, rec *httptest.ResponseRecorder, req *http.Request, port string) {
				client.ProxyToTab(rec, req, port, "tab-1", "/text")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen http.Header
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = r.Header.Clone()
				w.WriteHeader(200)
			}))
			defer srv.Close()

			parsed, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest("POST", "/find", strings.NewReader(`{"text":"Buy"}`))
			req.Header.Set(httpx.RequestIDHeader, "outer-trace-key")

			tc.call(NewBridgeClient(), httptest.NewRecorder(), req, parsed.Port())

			if seen == nil {
				t.Fatal("the instance received no request; this test is measuring nothing")
			}
			if got := seen.Get(httpx.RequestIDHeader); got != "outer-trace-key" {
				t.Errorf("instance was told %q, want the outer chain's id — without it the instance mints its own and one request needs two ids to trace", got)
			}
		})
	}
}

// The narrowness guard for the fresh-request hop: it must forward the request id and
// nothing else. Re-encoding the body is not a licence to hand over the caller's cookie.
func TestProxyWithTabIDForwardsOnlyTheRequestID(t *testing.T) {
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/find", strings.NewReader(`{"text":"Buy"}`))
	req.Header.Set(httpx.RequestIDHeader, "outer-trace-key")
	req.Header.Set("Cookie", "pinchtab_auth_token=session-secret")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("Authorization", "Bearer user-token")

	NewBridgeClient().ProxyWithTabID(httptest.NewRecorder(), req, parsed.Port(), "tab-1", "/find")

	if got := seen.Get(httpx.RequestIDHeader); got != "outer-trace-key" {
		t.Fatalf("request id = %q, want it forwarded — the rest of this test cannot tell a narrow forward from no forward without it", got)
	}
	for _, name := range []string{"Cookie", "X-Forwarded-For", "Authorization"} {
		if got := seen.Get(name); got != "" {
			t.Errorf("%s = %q reached the instance; this hop builds a fresh request precisely so the caller's headers do not travel, and forwarding the id widened it", name, got)
		}
	}
}
