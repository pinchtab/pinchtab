package instance

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// recordingBridge answers /tabs only when a credential is presented, which is
// what a spawned instance does: it runs the same auth middleware as the process
// that launched it.
func recordingBridge(t *testing.T, wantToken string) (*httptest.Server, *string) {
	t.Helper()
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		if wantToken != "" && seen != "Bearer "+wantToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tabs":[{"id":"tab-1"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

// FetchTabs is the only production entry point into this client, and it feeds the
// locator's tab→instance discovery. It sent no credential at all, so against a
// token-protected instance it was answered 401 — and both of its callers swallow
// that at debug level, so the discovery found nothing and said so nowhere.
func TestFetchTabsPresentsTheInstanceCredential(t *testing.T) {
	srv, seen := recordingBridge(t, "instance-token")

	client := NewBridgeClientWithAuth(func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer instance-token")
	})

	tabs, err := client.FetchTabs(srv.URL)
	if err != nil {
		t.Fatalf("FetchTabs against a token-protected instance: %v", err)
	}
	if len(tabs) != 1 || tabs[0].ID != "tab-1" {
		t.Errorf("tabs = %+v, want the one the instance reported", tabs)
	}
	if *seen != "Bearer instance-token" {
		t.Errorf("instance saw Authorization %q, want the credential the authorizer set", *seen)
	}
}

// The authorizer is optional: a bridge that wants no credential must still be
// reachable, which is what every stub in this package relies on.
func TestFetchTabsWithoutAnAuthorizerStillWorks(t *testing.T) {
	srv, seen := recordingBridge(t, "")

	if _, err := NewBridgeClient().FetchTabs(srv.URL); err != nil {
		t.Fatalf("FetchTabs against an open instance: %v", err)
	}
	if *seen != "" {
		t.Errorf("an unauthenticated client sent Authorization %q, want none", *seen)
	}
}

// Every request this client BUILDS must carry the credential, not just the one
// call that happened to be reported. A unit test cannot say that about a method
// nobody calls yet, so the rule is pinned over the source: each bc.client.Do goes
// through bc.authorized.
//
// The two proxy methods are excluded with their reason rather than silently: they
// forward a caller's request instead of building one, and each already has its own
// header rule — ProxyToTab copies the caller's Authorization, ProxyWithTabID
// deliberately forwards the request id and nothing else.
func TestEveryRequestThisClientBuildsIsAuthorized(t *testing.T) {
	exempt := map[string]string{
		"proxyReq": "forwards a caller's request, which carries its own header rule",
	}

	source, err := os.ReadFile("bridge_client.go")
	if err != nil {
		t.Fatalf("read the source this rule is about: %v", err)
	}

	dispatches, authorized := 0, 0
	for _, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || !strings.Contains(trimmed, "bc.client.Do(") {
			continue
		}
		dispatches++
		switch {
		case strings.Contains(trimmed, "bc.authorized("):
			authorized++
		case exemptedDispatch(trimmed, exempt):
		default:
			t.Errorf("%s dispatches without bc.authorized(); a request this client builds must carry the instance credential", trimmed)
		}
	}

	if dispatches < len(exempt)+1 {
		t.Fatalf("found %d dispatch site(s) against %d exemption(s); the scan matched almost nothing and would pass vacuously", dispatches, len(exempt))
	}
	if authorized == 0 {
		t.Error("no dispatch site is authorized, so this rule is guarding nothing")
	}
	for name, reason := range exempt {
		if !strings.Contains(string(source), "bc.client.Do("+name+")") {
			t.Errorf("%s is exempt (%s) but no longer dispatches, so the exemption is stale", name, reason)
		}
	}
}

func exemptedDispatch(line string, exempt map[string]string) bool {
	for name := range exempt {
		if strings.Contains(line, "bc.client.Do("+name+")") {
			return true
		}
	}
	return false
}
