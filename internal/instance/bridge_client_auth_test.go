package instance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
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

	if _, err := NewBridgeClientWithAuth(nil).FetchTabs(srv.URL); err != nil {
		t.Fatalf("FetchTabs against an open instance: %v", err)
	}
	if *seen != "" {
		t.Errorf("an unauthenticated client sent Authorization %q, want none", *seen)
	}
}

// Every request this client BUILDS must carry the credential, not just the one
// call that happened to be reported. The rule is pinned over the source so a new
// method cannot dispatch without going through bc.authorized.
func TestEveryRequestThisClientBuildsIsAuthorized(t *testing.T) {
	pkg := srccensus.Load(t, ".", 5)
	source := pkg.FileSource(pkg.MustFunc(t, "authorized"))

	dispatches := 0
	for line := range strings.SplitSeq(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || !strings.Contains(trimmed, "bc.client.Do(") {
			continue
		}
		dispatches++
		if !strings.Contains(trimmed, "bc.authorized(") {
			t.Errorf("%s dispatches without bc.authorized(); a request this client builds must carry the instance credential", trimmed)
		}
	}
	if dispatches == 0 {
		t.Fatal("found no dispatch site; the scan matched nothing and would pass vacuously")
	}
}
