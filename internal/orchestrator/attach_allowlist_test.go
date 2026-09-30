package orchestrator

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge/runtime"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestAttachHostAllowlistAgreesAcrossBridgeAndCDPAttach(t *testing.T) {
	tests := []struct {
		name  string
		host  string
		entry string
		want  bool
	}{
		{name: "exact remote", host: "10.0.0.8", entry: "10.0.0.8", want: true},
		{name: "LOCALHOST entry", host: "localhost", entry: "LOCALHOST", want: true},
		{name: "padded entry", host: "localhost", entry: " localhost ", want: true},
		{name: "bracketed ipv6 entry", host: "[::1]", entry: "[::1]", want: true},
		{name: "unbracketed ipv6 entry", host: "[::1]", entry: "::1", want: true},
		{name: "wildcard", host: "10.0.0.8", entry: "*", want: true},
		{name: "unlisted remote", host: "10.0.0.8", entry: "10.0.0.9", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowHosts := []string{tt.entry}
			rawURL := "ws://" + tt.host + ":9222/devtools/browser/x"

			o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
			o.ApplyRuntimeConfig(&config.RuntimeConfig{
				AttachEnabled:      true,
				AttachAllowHosts:   allowHosts,
				AttachAllowSchemes: []string{"ws"},
			})
			bridgeErr := o.validateAttachURL(rawURL)
			_, cdpErr := runtime.NormalizeCDPURLWithAllowlist(rawURL, allowHosts, []string{"ws"})

			if got := bridgeErr == nil; got != tt.want {
				t.Errorf("bridge attach allowed = %v, want %v (err: %v)", got, tt.want, bridgeErr)
			}
			if got := cdpErr == nil; got != tt.want {
				t.Errorf("CDP attach allowed = %v, want %v (err: %v)", got, tt.want, cdpErr)
			}
		})
	}
}
