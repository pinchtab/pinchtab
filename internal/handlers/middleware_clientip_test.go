package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/config"
)

func fillRateBucket(host string) {
	for i := 0; i < rateLimitMaxReq; i++ {
		requestLimiter.RecordFailure(host)
	}
}

func clientIPChain(trustProxy bool, seen *string) http.Handler {
	return clientIPChainWithHops(trustProxy, 1, seen)
}

func clientIPChainWithHops(trustProxy bool, hops int, seen *string) http.Handler {
	live := config.NewLive(&config.RuntimeConfig{TrustProxyHeaders: trustProxy, TrustedProxyHops: hops})
	return ClientIPMiddleware(live, RateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = authn.ClientIP(r)
		}
		w.WriteHeader(http.StatusOK)
	})))
}

func requestThrough(handler http.Handler, path, peer, forwardedFor string) int {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = peer
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w.Code
}

func TestUntrustedProxyBucketsRequestsByPeerWhateverForwardingHeadersArrive(t *testing.T) {
	resetRateLimitStateForTests()
	t.Cleanup(resetRateLimitStateForTests)

	fillRateBucket("198.51.100.10")
	handler := clientIPChain(false, nil)

	for _, forwarded := range []string{"203.0.113.1", "203.0.113.2"} {
		if code := requestThrough(handler, "/test", "198.51.100.10:41000", forwarded); code != http.StatusTooManyRequests {
			t.Fatalf("X-Forwarded-For %s got %d, want %d; an untrusted forwarding header must not mint a bucket", forwarded, code, http.StatusTooManyRequests)
		}
	}
}

func TestUntrustedProxyBucketsStreamsByPeerWhateverForwardingHeadersArrive(t *testing.T) {
	resetRateLimitStateForTests()
	t.Cleanup(resetRateLimitStateForTests)

	streamMu.Lock()
	streamConnections["198.51.100.10"] = maxConcurrentStreamRequestsPerHost
	streamMu.Unlock()
	handler := clientIPChain(false, nil)

	for _, forwarded := range []string{"203.0.113.1", "203.0.113.2"} {
		if code := requestThrough(handler, "/api/events", "198.51.100.10:41000", forwarded); code != http.StatusTooManyRequests {
			t.Fatalf("X-Forwarded-For %s got %d, want %d; an untrusted forwarding header must not mint a stream slot", forwarded, code, http.StatusTooManyRequests)
		}
	}
}

// An appending proxy writes the address it saw AFTER whatever the client sent, so
// the identity is the last element and the header the client controls is only a
// prefix. This test used to assert the client-most element, which is the value a
// caller chooses for itself.
func TestTrustedProxyBucketsRequestsByTheAddressTheProxyAppended(t *testing.T) {
	resetRateLimitStateForTests()
	t.Cleanup(resetRateLimitStateForTests)

	fillRateBucket("10.0.0.1")
	var seen string
	handler := clientIPChain(true, &seen)

	if code := requestThrough(handler, "/test", "198.51.100.10:41000", "203.0.113.1, 10.0.0.1"); code != http.StatusTooManyRequests {
		t.Fatalf("the exhausted client got %d, want %d", code, http.StatusTooManyRequests)
	}
	if code := requestThrough(handler, "/test", "198.51.100.10:41001", "203.0.113.2, 10.0.0.2"); code != http.StatusOK {
		t.Fatalf("a second client behind the same proxy got %d, want %d; the two share the proxy's peer address and must not share its bucket", code, http.StatusOK)
	}
	if seen != "10.0.0.2" {
		t.Fatalf("ClientIP() inside the chain = %q, want %q; audit lines record whatever this answers", seen, "10.0.0.2")
	}
}

// The bypass, pinned where it bites. Under an appending proxy the leftmost element
// is attacker-controlled, so a caller that rotates it once per request used to mint
// a fresh limiter bucket every time — and the same identity keys the dashboard
// login and elevation limiters, i.e. the brute-force protection on the server
// token. Every one of these requests must land in the ONE bucket the proxy's
// appended address names.
func TestRotatingTheForgedElementCannotEscapeTheRateLimitBucket(t *testing.T) {
	resetRateLimitStateForTests()
	t.Cleanup(resetRateLimitStateForTests)

	fillRateBucket("10.0.0.1")
	handler := clientIPChain(true, nil)

	for attempt := 0; attempt < 25; attempt++ {
		forged := fmt.Sprintf("203.0.113.%d, 10.0.0.1", attempt+1)
		if code := requestThrough(handler, "/test", "198.51.100.10:41000", forged); code != http.StatusTooManyRequests {
			t.Fatalf("attempt %d with X-Forwarded-For %q got %d, want %d; rotating the client-most element minted a fresh bucket", attempt, forged, code, http.StatusTooManyRequests)
		}
	}
}

// Under-configured hops must fail closed to the peer: a chain shorter than the
// count is what a forging client produces, and accepting it would hand the caller
// the bucket again.
func TestAChainShorterThanTheConfiguredHopsBucketsByThePeer(t *testing.T) {
	resetRateLimitStateForTests()
	t.Cleanup(resetRateLimitStateForTests)

	fillRateBucket("198.51.100.10")
	handler := clientIPChainWithHops(true, 2, nil)

	for _, forged := range []string{"203.0.113.1", "203.0.113.2"} {
		if code := requestThrough(handler, "/test", "198.51.100.10:41000", forged); code != http.StatusTooManyRequests {
			t.Fatalf("X-Forwarded-For %q got %d, want %d; a short chain must fall back to the transport peer", forged, code, http.StatusTooManyRequests)
		}
	}
}

func TestTrustedProxyBucketsStreamsByTheClientMostForwardedAddress(t *testing.T) {
	resetRateLimitStateForTests()
	t.Cleanup(resetRateLimitStateForTests)

	streamMu.Lock()
	streamConnections["203.0.113.1"] = maxConcurrentStreamRequestsPerHost
	streamMu.Unlock()
	handler := clientIPChain(true, nil)

	if code := requestThrough(handler, "/api/events", "198.51.100.10:41000", "203.0.113.1"); code != http.StatusTooManyRequests {
		t.Fatalf("the exhausted client got %d, want %d", code, http.StatusTooManyRequests)
	}
	if code := requestThrough(handler, "/api/events", "198.51.100.10:41001", "203.0.113.2"); code != http.StatusOK {
		t.Fatalf("a second client behind the same proxy got %d, want %d; eight streams belong to a client, not to the deployment", code, http.StatusOK)
	}
}
