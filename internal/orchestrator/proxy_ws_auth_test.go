package orchestrator

import (
	"net/http"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// The WebSocket header filter in internal/proxy promotes
// X-Pinchtab-Proxy-Authorization into Authorization on the orchestrator →
// instance connection. It is an internal channel, so a value arriving from the
// client must never survive — including when the orchestrator has no token of
// its own to overwrite it with. The /screencast path clears it unconditionally
// before setting; applyInstanceAuth must do the same.
func TestApplyInstanceAuthDropsClientSuppliedWSAuthorization(t *testing.T) {
	const header = "X-Pinchtab-Proxy-Authorization"

	tests := []struct {
		name          string
		instanceToken string
		childToken    string
		want          string
	}{
		{name: "no tokens configured", want: ""},
		{name: "instance token wins", instanceToken: "real-token", want: "Bearer real-token"},
		{name: "child token used", childToken: "child-token", want: "Bearer child-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := orchWithConfig(&config.RuntimeConfig{Token: tt.childToken})
			inst := &InstanceInternal{authToken: tt.instanceToken}

			req, err := http.NewRequest(http.MethodGet, "http://instance/screencast", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(header, "Bearer attacker-supplied")

			o.applyInstanceAuth(req, inst)

			if got := req.Header.Get(header); got != tt.want {
				t.Errorf("%s = %q, want %q", header, got, tt.want)
			}
		})
	}
}
