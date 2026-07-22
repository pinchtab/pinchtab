package actions

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCaptureThreadsExplicitTabIntoPairedRequest(t *testing.T) {
	m := newMockServer()
	m.response = `{"status":"ok","tabId":"tab-target","image":{"format":"png","base64":"aW1n"},"snapshot":{"nodeCount":1,"nodes":[{"ref":"e1","role":"button","name":"Save"}]},"pairing":{"navigated":false}}`
	defer m.close()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("tab", "", "")
	_ = cmd.Flags().Set("json", "true")
	_ = cmd.Flags().Set("tab", "tab-target")

	out := captureStdout(t, func() {
		Capture(m.server.Client(), m.base(), "", cmd)
	})
	if m.lastPath != "/capture" || !strings.Contains(m.lastQuery, "tabId=tab-target") {
		t.Fatalf("paired capture request = %s?%s, want /capture?tabId=tab-target", m.lastPath, m.lastQuery)
	}
	for _, want := range []string{`"tabId": "tab-target"`, `"base64": "aW1n"`, `"name": "Save"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("capture JSON missing %s: %s", want, out)
		}
	}
}
