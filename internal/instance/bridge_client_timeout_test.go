package instance

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

func TestBridgeClientAllowsTheFullNavigationBudget(t *testing.T) {
	got := NewBridgeClient().client.Timeout
	if got != httpx.MaxNavigationHTTPDuration {
		t.Fatalf("timeout = %v, want %v", got, httpx.MaxNavigationHTTPDuration)
	}
}

// The orchestrator's own tab listing goes through FetchTabs, so a wedged /tabs
// must cost it the read budget and name the instance address, while the
// client's Timeout stays the navigation ceiling above.
func TestFetchTabsIsBoundByTheReadBudget(t *testing.T) {
	prevRead := httpx.ReadHTTPDuration
	httpx.ReadHTTPDuration = 100 * time.Millisecond
	t.Cleanup(func() { httpx.ReadHTTPDuration = prevRead })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	started := time.Now()
	_, err := NewBridgeClient().FetchTabs(srv.URL)
	if err == nil {
		t.Fatal("a wedged /tabs answered")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("FetchTabs waited %v; the read budget did not bound it", time.Since(started))
	}
	host := strings.TrimPrefix(srv.URL, "http://")
	if !strings.Contains(err.Error(), host) || !strings.Contains(err.Error(), "did not answer within its 100ms budget") {
		t.Fatalf("error does not name the instance and the budget: %v", err)
	}
}
