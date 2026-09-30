// Package noinstance registers the "no-instance" strategy: no local browser
// instances are launched, and the server only accepts remote bridges via
// /instances/attach-bridge.
package noinstance

import (
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/strategy"
	"github.com/pinchtab/pinchtab/internal/strategy/explicit"
)

func init() {
	strategy.MustRegister("no-instance", func() strategy.Strategy {
		return explicit.NewDirect("no-instance", (*orchestrator.Orchestrator).RegisterHandlersNoLaunch,
			"no remote instances connected — attach a bridge first")
	})
}
