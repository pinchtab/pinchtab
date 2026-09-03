package dashboard

import (
	"path/filepath"

	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/session"
)

const dashboardSessionStateFile = "dashboard-auth-sessions.json"

const agentSessionStateFile = "sessions.json"

func BrowserSessionConfig(runtime *config.RuntimeConfig) browsersession.Config {
	if runtime == nil {
		return browsersession.Config{}
	}
	return browsersession.Config{
		IdleTimeout:                   runtime.Sessions.Dashboard.IdleTimeout,
		MaxLifetime:                   runtime.Sessions.Dashboard.MaxLifetime,
		ElevationWindow:               runtime.Sessions.Dashboard.ElevationWindow,
		Persist:                       runtime.Sessions.Dashboard.Persist,
		PersistPath:                   filepath.Join(runtime.StateDir, dashboardSessionStateFile),
		PersistElevationAcrossRestart: runtime.Sessions.Dashboard.PersistElevationAcrossRestart,
	}
}

// AgentSessionConfig maps the runtime config onto the agent session store's
// settings. persistPath is passed in rather than rebuilt from StateDir: the
// store's config is replaced wholesale, so recomputing it would relocate live
// sessions mid-run if server.stateDir were edited.
func AgentSessionConfig(runtime *config.RuntimeConfig, persistPath string) session.Config {
	if runtime == nil {
		return session.Config{PersistPath: persistPath}
	}
	return session.Config{
		Enabled:     runtime.Sessions.Agent.Enabled,
		Mode:        runtime.Sessions.Agent.Mode,
		IdleTimeout: runtime.Sessions.Agent.IdleTimeout,
		MaxLifetime: runtime.Sessions.Agent.MaxLifetime,
		PersistPath: persistPath,
	}
}

// AgentSessionStatePath is where the agent session store persists at boot.
func AgentSessionStatePath(runtime *config.RuntimeConfig) string {
	if runtime == nil {
		return ""
	}
	return filepath.Join(runtime.StateDir, agentSessionStateFile)
}
