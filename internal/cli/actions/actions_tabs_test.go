package actions

import (
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTabCloseUsesCloseEndpoint(t *testing.T) {
	m := newMockServer()
	defer m.close()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")

	TabClose(http.DefaultClient, m.base(), "", "tab_123", cmd)

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
