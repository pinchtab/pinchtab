package main

import (
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
)

// agentStatus is the rendered projection of the local config plus an optional
// health snapshot. It is computed without any I/O so the renderer stays a pure
// function of (config, snapshot, state).
type agentStatus struct {
	state          healthSnapshotState
	running        bool
	guardsDown     bool
	listenAddr     string
	sensitive      []string
	allowedDomains []string
	idpiEnabled    bool
	nextSteps      []cli.CommandHint
	nextStepsWidth int
}

// projectAgentStatus derives the banner fields from the runtime config and the
// (possibly nil) health snapshot. snap is non-nil only when state is
// healthSnapshotRunning.
func projectAgentStatus(cfg *config.RuntimeConfig, snap *healthSnapshot, state healthSnapshotState) agentStatus {
	st := agentStatus{
		state:          state,
		running:        state == healthSnapshotRunning,
		listenAddr:     cfg.ListenAddr(),
		allowedDomains: cfg.AllowedDomains,
		idpiEnabled:    cfg.IDPI.Enabled,
	}

	if st.running && snap != nil && snap.Security != nil {
		st.guardsDown = snap.Security.GuardsDown
		st.sensitive = snap.Security.EnabledSensitiveEndpoints
		st.allowedDomains = snap.Security.AllowedDomains
		st.idpiEnabled = snap.Security.IDPIEnabled
	}

	st.nextSteps, st.nextStepsWidth = nextStepsForState(state)
	return st
}

func nextStepsForState(state healthSnapshotState) ([]cli.CommandHint, int) {
	switch state {
	case healthSnapshotRunning:
		return cli.NextStepsRunningHints, 64
	case healthSnapshotProtected:
		return []cli.CommandHint{
			{Command: "pinchtab config token", Comment: "# copy configured API token"},
			{Command: "pinchtab health --json", Comment: "# retry health with the current token"},
			{Command: "pinchtab config show", Comment: "# inspect configured port and token"},
		}, 44
	case healthSnapshotInvalid, healthSnapshotUnhealthy:
		return []cli.CommandHint{
			{Command: "pinchtab health --json", Comment: "# inspect the current listener"},
			{Command: "pinchtab config show", Comment: "# verify configured port/token"},
			{Command: "pinchtab server", Comment: "# start after freeing the port"},
		}, 44
	default:
		return []cli.CommandHint{
			{Command: "pinchtab server", Comment: "# start the server (foreground)"},
			{Command: "pinchtab server -y", Comment: "# start with guards down (this run only)"},
			{Command: "pinchtab daemon install", Comment: "# install background service"},
		}, 44
	}
}
