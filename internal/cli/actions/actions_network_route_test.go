package actions

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

const routeRulesPayload = `{"tabId":"t1","rules":[{"pattern":"api/users","action":"fulfill","body":"{\"ok\":true}","contentType":"application/json","status":200},{"pattern":"*.png","action":"abort"}]}`

func networkRulesCommand(tab string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("json", false, "")
	_ = cmd.Flags().Set("tab", tab)
	return cmd
}

func TestNetworkRulesListsEveryRuleWithItsAction(t *testing.T) {
	m := newMockServer()
	defer m.close()
	m.responses["GET /tabs/t1/network/route"] = mockResponse{statusCode: 200, body: routeRulesPayload}

	out := captureStdout(t, func() {
		NetworkRules(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, networkRulesCommand("t1"))
	})
	if m.lastMethod != "GET" || m.lastPath != "/tabs/t1/network/route" {
		t.Fatalf("listed via %s %s, want GET /tabs/t1/network/route", m.lastMethod, m.lastPath)
	}
	for _, want := range []string{"api/users (fulfill)", "*.png (abort)"} {
		if !strings.Contains(out, want) {
			t.Errorf("listing %q does not carry %q; a reader must tell a fulfill from an abort", out, want)
		}
	}
}

func TestNetworkRulesOnATabWithNoRulesAnswersNone(t *testing.T) {
	m := newMockServer()
	defer m.close()
	m.responses["GET /tabs/t1/network/route"] = mockResponse{statusCode: 200, body: `{"tabId":"t1","rules":[]}`}

	out := captureStdout(t, func() {
		NetworkRules(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, networkRulesCommand("t1"))
	})
	if !strings.Contains(out, "no interception rules") {
		t.Fatalf("an empty listing must say none, got %q", out)
	}
}
