package actions

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newNavigateCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("new-tab", false, "")
	cmd.Flags().Bool("block-images", false, "")
	cmd.Flags().Bool("block-ads", false, "")
	cmd.Flags().Bool("dismiss-banners", false, "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("print-tab-id", false, "")
	return cmd
}

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("snap", false, "")
	cmd.Flags().Bool("snap-diff", false, "")
	cmd.Flags().Bool("text", false, "")
	cmd.Flags().Bool("dismiss-banners", false, "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func TestNavigate(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	if m.lastMethod != "POST" {
		t.Errorf("expected POST, got %s", m.lastMethod)
	}
	if m.lastPath != "/navigate" {
		t.Errorf("expected /navigate, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["url"] != "https://pinchtab.com" {
		t.Errorf("expected url=https://pinchtab.com, got %v", body["url"])
	}
}

func TestNavigateReusesImplicitTabWhenItExists(t *testing.T) {
	m := newMockServer()
	m.response = `{"tabId":"ABC123","status":"ok"}`
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	cmd.Flags().Lookup("tab").DefValue = "ABC123"
	_ = cmd.Flags().Set("tab", "ABC123")
	cmd.Flags().Lookup("tab").Changed = false

	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)

	if len(m.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(m.requests))
	}
	if m.requests[0].Path != "/tabs/ABC123/navigate" {
		t.Fatalf("navigate path = %q, want /tabs/ABC123/navigate", m.requests[0].Path)
	}
}

func TestNavigateFallsBackToNewTabForStaleImplicitTab(t *testing.T) {
	m := newMockServer()
	m.setResponse(http.MethodPost, "/tabs/STALE123/navigate", http.StatusNotFound, `{"error":"tab not found"}`)
	m.setResponse(http.MethodPost, "/navigate", http.StatusOK, `{"tabId":"NEW123","status":"ok"}`)
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	cmd.Flags().Lookup("tab").DefValue = "STALE123"
	_ = cmd.Flags().Set("tab", "STALE123")
	cmd.Flags().Lookup("tab").Changed = false

	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)

	if len(m.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(m.requests))
	}
	if m.requests[0].Path != "/tabs/STALE123/navigate" {
		t.Fatalf("first request path = %q, want /tabs/STALE123/navigate", m.requests[0].Path)
	}
	if m.requests[1].Path != "/navigate" {
		t.Fatalf("navigate path = %q, want /navigate", m.requests[1].Path)
	}
}

func TestBuildNavigateRequestDoesNotFallbackForExplicitTab(t *testing.T) {
	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("tab", "EXPLICIT123")

	req := buildNavigateRequest("https://pinchtab.com", cmd)

	if req.path != "/tabs/EXPLICIT123/navigate" {
		t.Fatalf("path = %q, want /tabs/EXPLICIT123/navigate", req.path)
	}
	if req.fallbackOnNotFound {
		t.Fatal("explicit --tab should not fallback on 404")
	}
}

func TestNavigateWithAllFlags(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("new-tab", "true")
	_ = cmd.Flags().Set("block-images", "true")
	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["newTab"] != true {
		t.Error("expected newTab=true")
	}
	if body["blockImages"] != true {
		t.Error("expected blockImages=true")
	}
}

func TestNavigateWithBlockAds(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("block-ads", "true")
	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["blockAds"] != true {
		t.Error("expected blockAds=true")
	}
}

func TestNavigateDismissBanners(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["dismissBanners"] != true {
		t.Errorf("expected dismissBanners=true in body, got %v", body["dismissBanners"])
	}
}

func TestReloadDismissBannersAppendsQuery(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHistoryCmd()
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Reload(client, m.base(), "", cmd)
	if m.lastPath != "/reload" {
		t.Errorf("expected /reload path, got %q", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "dismissBanners=true") {
		t.Errorf("expected dismissBanners=true in query, got %q", m.lastQuery)
	}
}

func TestBackDismissBannersAppendsQuery(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHistoryCmd()
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Back(client, m.base(), "", cmd)
	if m.lastPath != "/back" {
		t.Errorf("expected /back path, got %q", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "dismissBanners=true") {
		t.Errorf("expected dismissBanners=true in query, got %q", m.lastQuery)
	}
}

func TestForwardDismissBannersAppendsQueryWithTab(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHistoryCmd()
	_ = cmd.Flags().Set("tab", "TAB1")
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Forward(client, m.base(), "", cmd)
	if m.lastPath != "/tabs/TAB1/forward" {
		t.Errorf("expected /tabs/TAB1/forward, got %q", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "dismissBanners=true") {
		t.Errorf("expected dismissBanners=true in query, got %q", m.lastQuery)
	}
}

func TestReloadWithoutDismissBannersOmitsQuery(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHistoryCmd()
	Reload(client, m.base(), "", cmd)
	if m.lastQuery != "" {
		t.Errorf("expected empty query, got %q", m.lastQuery)
	}
}

// TestNavigatePrintTabID verifies that --print-tab-id makes `nav` emit only
// the tab ID on stdout so agents can capture it via `$(pinchtab nav URL)`.
func TestNavigatePrintTabID(t *testing.T) {
	m := newMockServer()
	m.response = `{"tabId":"ABC123","status":"ok"}`
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("print-tab-id", "true")

	out := captureStdout(t, func() {
		Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	})
	got := strings.TrimSpace(out)
	if got != "ABC123" {
		t.Errorf("expected stdout to be exactly 'ABC123', got %q", got)
	}
}
