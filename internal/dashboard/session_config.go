package dashboard

import (
	"path/filepath"

	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
)

const dashboardSessionStateFile = "dashboard-auth-sessions.json"

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
