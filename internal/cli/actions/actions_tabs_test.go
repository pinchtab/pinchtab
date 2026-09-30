package actions

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func TestTabCloseUsesCloseEndpoint(t *testing.T) {
	m := newMockServer()
	defer m.close()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")

	TabClose(apiclient.Conn{Client: http.DefaultClient, Base: m.base()}, "tab_123", cmd)

	if m.lastPath != "/close" {
		t.Fatalf("expected /close, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastBody, `"tabId":"tab_123"`) {
		t.Fatalf("expected tabId in body, got %s", m.lastBody)
	}
	if strings.Contains(m.lastBody, "action") {
		t.Fatalf("close endpoint body should not include legacy action field, got %s", m.lastBody)
	}
}

func TestTabFocusUsesResolvedTabID(t *testing.T) {
	m := newMockServer()
	m.response = `{"focused":true,"tabId":"tab_123"}`
	defer m.close()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")

	got := TabFocus(apiclient.Conn{Client: http.DefaultClient, Base: m.base()}, "tab_123", cmd)

	if got != "tab_123" {
		t.Fatalf("focused tab = %q, want tab_123", got)
	}
	if m.lastPath != "/tab" {
		t.Fatalf("expected /tab, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastBody, `"tabId":"tab_123"`) {
		t.Fatalf("expected tabId in body, got %s", m.lastBody)
	}
}

func TestTabFocusResolvesOneBasedIndex(t *testing.T) {
	m := newMockServer()
	m.response = `{"tabs":[{"id":"tab_1"},{"id":"tab_2"}]}`
	defer m.close()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")

	got := TabFocus(apiclient.Conn{Client: http.DefaultClient, Base: m.base()}, "2", cmd)

	if got != "tab_2" {
		t.Fatalf("focused tab = %q, want tab_2", got)
	}
	if len(m.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(m.requests))
	}
	if m.requests[0].Path != "/tabs" {
		t.Fatalf("first request path = %q, want /tabs", m.requests[0].Path)
	}
	if m.requests[1].Path != "/tab" {
		t.Fatalf("second request path = %q, want /tab", m.requests[1].Path)
	}
	if !strings.Contains(m.requests[1].Body, `"tabId":"tab_2"`) {
		t.Fatalf("expected resolved tabId in body, got %s", m.requests[1].Body)
	}
}
