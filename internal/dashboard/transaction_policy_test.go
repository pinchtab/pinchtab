package dashboard

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestRestartReasonsIncludesTransactionPolicy(t *testing.T) {
	api := &ConfigAPI{boot: config.DefaultFileConfig()}
	next := config.DefaultFileConfig()
	next.Security.TransactionPolicy = config.TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example"}}
	for _, reason := range api.restartReasonsFor(next) {
		if reason == "Transaction policy" {
			return
		}
	}
	t.Fatal("transaction policy change did not require restart")
}
