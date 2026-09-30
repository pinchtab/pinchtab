package actions

import (
	"net/http"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func TestNetworkClearPreservesExplicitTab(t *testing.T) {
	m := newMockServer()
	defer m.close()

	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	_ = cmd.Flags().Set("tab", "tab-target")
	NetworkClear(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, cmd)

	if m.lastPath != "/network/clear" || m.lastQuery != "tabId=tab-target" {
		t.Fatalf("network clear request = %s?%s, want /network/clear?tabId=tab-target", m.lastPath, m.lastQuery)
	}
}

func TestNetworkStreamSendsSessionScheme(t *testing.T) {
	m := newMockServer()
	defer m.close()

	NetworkStream(apiclient.Conn{Client: m.server.Client(), Base: m.base(), Token: "ses_x"}, &cobra.Command{})

	if got := m.lastHeaders.Get("Authorization"); got != "Session ses_x" {
		t.Fatalf("Authorization = %q, want %q", got, "Session ses_x")
	}
}

type agentHeaderRoundTripper struct{ next http.RoundTripper }

func (rt agentHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Agent-Id", "agent-7")
	return rt.next.RoundTrip(req)
}

func TestNetworkStreamKeepsTheCLITransport(t *testing.T) {
	m := newMockServer()
	defer m.close()

	client := &http.Client{Transport: agentHeaderRoundTripper{next: m.server.Client().Transport}}
	NetworkStream(apiclient.Conn{Client: client, Base: m.base()}, &cobra.Command{})

	if got := m.lastHeaders.Get("X-Agent-Id"); got != "agent-7" {
		t.Fatalf("X-Agent-Id = %q, want %q", got, "agent-7")
	}
	if m.lastPath != "/network/stream" {
		t.Fatalf("path = %q, want /network/stream", m.lastPath)
	}
}
