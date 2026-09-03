package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// upstreamEcho records what the upstream actually received.
type upstreamEcho struct {
	mu     sync.Mutex
	method string
	path   string
	query  string
	host   string
	body   string
	header http.Header
}

func newUpstream(t *testing.T) (*httptest.Server, *upstreamEcho) {
	t.Helper()
	seen := &upstreamEcho{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen.mu.Lock()
		seen.method, seen.path, seen.query = r.Method, r.URL.Path, r.URL.RawQuery
		seen.host, seen.body, seen.header = r.Host, string(body), r.Header.Clone()
		seen.mu.Unlock()
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func forwardTo(t *testing.T, srv *httptest.Server, inbound *http.Request, rewrite func(*http.Request)) {
	t.Helper()
	target, err := url.Parse(srv.URL + "/upstream")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	Forward(httptest.NewRecorder(), inbound, target, Options{RewriteRequest: rewrite})
}

// RewriteRequest is handed a whole *http.Request, and the WebSocket branch of
// Forward honours the whole of it. The HTTP branch re-derived the method, the
// target and the body from the ORIGINAL request, so a hook that rewrote any of
// them took effect on one transport and was silently dropped on the other.
func TestRewriteRequestReachesTheUpstreamOnTheHTTPPath(t *testing.T) {
	srv, seen := newUpstream(t)

	inbound := httptest.NewRequest("POST", "/original", strings.NewReader("original-body"))
	forwardTo(t, srv, inbound, func(req *http.Request) {
		req.Method = "PUT"
		req.URL.Path = "/rewritten"
		req.URL.RawQuery = "rewritten=1"
		req.Body = io.NopCloser(strings.NewReader("rewritten-body"))
	})

	seen.mu.Lock()
	defer seen.mu.Unlock()
	if seen.method != "PUT" {
		t.Errorf("upstream saw method %q, want the rewritten PUT", seen.method)
	}
	if seen.path != "/rewritten" {
		t.Errorf("upstream saw path %q, want the rewritten path", seen.path)
	}
	if seen.query != "rewritten=1" {
		t.Errorf("upstream saw query %q, want the rewritten query", seen.query)
	}
	if seen.body != "rewritten-body" {
		t.Errorf("upstream saw body %q, want the rewritten body", seen.body)
	}
}

// A rewriter that sets Host must reach the wire; the Host header is how a shared
// upstream tells virtual hosts apart.
func TestRewriteRequestCanSetTheHostHeader(t *testing.T) {
	srv, seen := newUpstream(t)

	inbound := httptest.NewRequest("GET", "/original", nil)
	forwardTo(t, srv, inbound, func(req *http.Request) {
		req.Host = "rewritten.example"
	})

	seen.mu.Lock()
	defer seen.mu.Unlock()
	if seen.host != "rewritten.example" {
		t.Errorf("upstream saw Host %q, want the rewritten host", seen.host)
	}
}

// Header rewriting is what every hook in the module does today, and it must keep
// arriving unchanged.
func TestRewriteRequestHeadersStillReachTheUpstream(t *testing.T) {
	srv, seen := newUpstream(t)

	inbound := httptest.NewRequest("GET", "/original", nil)
	forwardTo(t, srv, inbound, func(req *http.Request) {
		req.Header.Set("X-Rewritten", "yes")
	})

	seen.mu.Lock()
	defer seen.mu.Unlock()
	if got := seen.header.Get("X-Rewritten"); got != "yes" {
		t.Errorf("upstream saw X-Rewritten %q, want yes", got)
	}
}

// With no rewriter, every field must still be taken from the inbound request and
// the target URL exactly as before — including the Host, which the transport
// derives from the URL rather than from targetURL.Host.
func TestForwardWithoutARewriterIsUnchanged(t *testing.T) {
	srv, seen := newUpstream(t)
	upstreamHost := strings.TrimPrefix(srv.URL, "http://")

	inbound := httptest.NewRequest("POST", "/original?keep=1", strings.NewReader("inbound-body"))
	forwardTo(t, srv, inbound, nil)

	seen.mu.Lock()
	defer seen.mu.Unlock()
	if seen.method != "POST" {
		t.Errorf("method = %q, want POST", seen.method)
	}
	if seen.path != "/upstream" {
		t.Errorf("path = %q, want the target's path", seen.path)
	}
	if seen.body != "inbound-body" {
		t.Errorf("body = %q, want the inbound body", seen.body)
	}
	if seen.host != upstreamHost {
		t.Errorf("Host = %q, want the target host %q the transport derives from the URL", seen.host, upstreamHost)
	}
}

// AllowedURL gates the target before the hook runs, and the hook can move the
// target. Sharing one *url.URL between the two meant the gate approved an object
// the hook then edited — and a gate that asks "is this still the same origin?"
// cannot notice, because an alias always is.
func TestARewriteThatMovesTheTargetIsGatedAgain(t *testing.T) {
	srv, seen := newUpstream(t)
	elsewhere, elsewhereSeen := newUpstream(t)

	approved := 0
	target, err := url.Parse(srv.URL + "/upstream")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	original := target.String()

	w := httptest.NewRecorder()
	Forward(w, httptest.NewRequest("GET", "/original", nil), target, Options{
		AllowedURL: func(u *url.URL) bool {
			approved++
			return u.String() == original
		},
		RewriteRequest: func(req *http.Request) {
			moved, parseErr := url.Parse(elsewhere.URL + "/elsewhere")
			if parseErr != nil {
				t.Errorf("parse moved target: %v", parseErr)
				return
			}
			*req.URL = *moved
		},
	})

	if approved < 2 {
		t.Errorf("AllowedURL was consulted %d time(s); a moved target must be re-gated", approved)
	}
	if w.Code != 400 {
		t.Errorf("status = %d, want 400; the rewritten target was not approved", w.Code)
	}
	elsewhereSeen.mu.Lock()
	reached := elsewhereSeen.path
	elsewhereSeen.mu.Unlock()
	if reached != "" {
		t.Errorf("the request reached %q despite failing the gate", reached)
	}
	if target.String() != original {
		t.Errorf("the caller's target URL was mutated to %q; Forward must not edit what it was handed", target)
	}
	seen.mu.Lock()
	defer seen.mu.Unlock()
	if seen.path != "" {
		t.Errorf("the original upstream was contacted at %q after a refused rewrite", seen.path)
	}
}

// A rewrite that stays on the approved target must not pay for a second gate
// call — the common path, and the one the orchestrator would answer by
// re-scanning its instance table.
func TestARewriteThatKeepsTheTargetIsNotGatedTwice(t *testing.T) {
	srv, _ := newUpstream(t)

	approved := 0
	target, err := url.Parse(srv.URL + "/upstream")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	Forward(httptest.NewRecorder(), httptest.NewRequest("GET", "/original", nil), target, Options{
		AllowedURL:     func(*url.URL) bool { approved++; return true },
		RewriteRequest: func(req *http.Request) { req.Header.Set("X-Rewritten", "yes") },
	})

	if approved != 1 {
		t.Errorf("AllowedURL was consulted %d times for an unmoved target, want 1", approved)
	}
}
