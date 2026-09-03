package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/config"
)

func fillRateBucket(host string) {
	now := time.Now()
	hits := make([]time.Time, rateLimitMaxReq)
	for i := range hits {
		hits[i] = now
	}
	rateMu.Lock()
	rateBuckets[host] = hits
	rateMu.Unlock()
}

func clientIPChain(trustProxy bool, seen *string) http.Handler {
	live := config.NewLive(&config.RuntimeConfig{TrustProxyHeaders: trustProxy})
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

func TestTrustedProxyBucketsRequestsByTheClientMostForwardedAddress(t *testing.T) {
	resetRateLimitStateForTests()
	t.Cleanup(resetRateLimitStateForTests)

	fillRateBucket("203.0.113.1")
	var seen string
	handler := clientIPChain(true, &seen)

	if code := requestThrough(handler, "/test", "198.51.100.10:41000", "203.0.113.1, 10.0.0.1"); code != http.StatusTooManyRequests {
		t.Fatalf("the exhausted client got %d, want %d", code, http.StatusTooManyRequests)
	}
	if code := requestThrough(handler, "/test", "198.51.100.10:41001", "203.0.113.2, 10.0.0.1"); code != http.StatusOK {
		t.Fatalf("a second client behind the same proxy got %d, want %d; the two share the proxy's peer address and must not share its bucket", code, http.StatusOK)
	}
	if seen != "203.0.113.2" {
		t.Fatalf("ClientIP() inside the chain = %q, want %q; audit lines record whatever this answers", seen, "203.0.113.2")
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
