package actions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNetworkRulesListsDifferentActionsAndHonorsTab(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/tabs/tab%2Fone/network/route" {
			t.Fatalf("request = %s %s", r.Method, r.URL.EscapedPath())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rules": []map[string]any{
			{"pattern": "api/users", "action": "fulfill"},
			{"pattern": "*.png", "action": "abort"},
		}})
	}))
	defer srv.Close()

	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "tab/one", "")
	out := captureStdout(t, func() { NetworkRules(http.DefaultClient, srv.URL, "", cmd) })
	for _, want := range []string{"api/users", "fulfill", "*.png", "abort"} {
		if !strings.Contains(out, want) {
			t.Errorf("listing %q does not contain %q", out, want)
		}
	}
}

func TestNetworkRulesEmptyListingIsSuccessfulAndExplicit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rules":[]}`))
	}))
	defer srv.Close()

	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	out := captureStdout(t, func() { NetworkRules(http.DefaultClient, srv.URL, "", cmd) })
	if !strings.Contains(out, `"rules": []`) {
		t.Fatalf("empty listing = %q, want explicit empty rules", out)
	}
}
