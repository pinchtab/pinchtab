package orchestrator

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestLaunchRejectsRequestExtensionPathsWhenTransactionPolicyEnabled(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{TransactionPolicy: config.TransactionPolicyConfig{Enabled: true}})
	_, err := o.Launch("profile", "9868", true, []string{"/request-extension"})
	if err == nil || !strings.Contains(err.Error(), "request-supplied extensionPaths") {
		t.Fatalf("Launch error = %v", err)
	}
	if _, err := o.Attach("external", "ws://127.0.0.1:9222/devtools/browser/x"); err == nil || !strings.Contains(err.Error(), "managed browser launch") {
		t.Fatalf("Attach error = %v", err)
	}
}
