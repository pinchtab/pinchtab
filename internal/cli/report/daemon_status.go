package report

import "github.com/pinchtab/pinchtab/internal/daemon"

func IsDaemonInstalled() bool {
	return daemon.IsInstalled()
}

func IsDaemonRunning() bool {
	return daemon.IsRunning()
}
