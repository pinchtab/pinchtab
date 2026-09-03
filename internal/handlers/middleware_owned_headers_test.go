package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func headersSetByMiddleware(t *testing.T, mw func(http.Handler) http.Handler, r *http.Request) []string {
	t.Helper()

	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})).ServeHTTP(rec, r)

	var names []string
	for name := range rec.Header() {
		names = append(names, name)
	}
	return names
}

// The owned set is what every proxy hop filters out of an upstream response. It is pinned
// against the middlewares themselves rather than a hand-kept list, so the next header the
// outer chain starts setting cannot be the one that keeps arriving twice.
func TestTheOwnedResponseHeaderSetIsExactlyWhatTheOuterChainSets(t *testing.T) {
	plain := httptest.NewRequest("GET", "/tabs", nil)
	secure := httptest.NewRequest("GET", "/tabs", nil)
	secure.TLS = &tls.ConnectionState{}

	cfg := &config.RuntimeConfig{}
	set := map[string]bool{}
	for _, name := range headersSetByMiddleware(t, RequestIDMiddleware, plain) {
		set[name] = true
	}
	for _, r := range []*http.Request{plain, secure} {
		for _, name := range headersSetByMiddleware(t, func(next http.Handler) http.Handler {
			return SecurityHeadersMiddleware(config.NewLive(cfg), next)
		}, r) {
			set[name] = true
		}
	}

	if len(set) < 4 {
		t.Fatalf("the two middlewares set %v; that is fewer headers than this guard was written over, so it would pass vacuously", set)
	}

	var got []string
	for name := range set {
		got = append(got, name)
	}
	want := httpx.OuterChainResponseHeaders()
	sort.Strings(got)
	sort.Strings(want)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the outer chain sets %v but httpx.OuterChainResponseHeaders() declares %v.\nA header the chain sets and the set omits arrives twice on every proxied response, and the second value is one no log holds; a header the set names and the chain no longer sets is filtered out of upstream responses that legitimately own it.", got, want)
	}
}

func TestTheOuterChainSetsItsHeadersSingleValued(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/tabs", nil)
	req.TLS = &tls.ConnectionState{}

	handler := RequestIDMiddleware(SecurityHeadersMiddleware(config.NewLive(&config.RuntimeConfig{}), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})))
	handler.ServeHTTP(rec, req)

	for _, name := range httpx.OuterChainResponseHeaders() {
		if got := rec.Header().Values(name); len(got) > 1 {
			t.Errorf("%s = %v: the outer chain itself is the second writer, so no proxy filter can make it single-valued", name, got)
		}
	}
}

func TestRequestIDMiddlewareStampsTheSameIDOnRequestAndResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/tabs", nil)

	var seen string
	RequestIDMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(httpx.HeaderRequestID)
	})).ServeHTTP(rec, req)

	response := rec.Header().Get(httpx.HeaderRequestID)
	if response == "" {
		t.Fatal("no request id on the response, so a caller has nothing to quote")
	}
	if seen != response {
		t.Errorf("downstream request carries %q while the response carries %q; the id the outer process logs must be the one a downstream hop can propagate", seen, response)
	}
}
