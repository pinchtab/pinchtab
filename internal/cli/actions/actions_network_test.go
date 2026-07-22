package actions

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestNetworkClearPreservesExplicitTab(t *testing.T) {
	m := newMockServer()
	defer m.close()

	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	_ = cmd.Flags().Set("tab", "tab-target")
	NetworkClear(m.server.Client(), m.base(), "", cmd)

	if m.lastPath != "/network/clear" || m.lastQuery != "tabId=tab-target" {
		t.Fatalf("network clear request = %s?%s, want /network/clear?tabId=tab-target", m.lastPath, m.lastQuery)
	}
}
