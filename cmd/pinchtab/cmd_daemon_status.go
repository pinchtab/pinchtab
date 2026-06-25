package main

import (
	"strings"

	"github.com/pinchtab/pinchtab/internal/daemon"
)

type daemonStatus struct {
	Installed      bool   `json:"installed"`
	Running        bool   `json:"running"`
	PID            string `json:"pid,omitempty"`
	ServicePath    string `json:"servicePath,omitempty"`
	PreflightError string `json:"preflightError,omitempty"`
	ManagerError   string `json:"managerError,omitempty"`
}

// collectDaemonStatus probes manager/service state into a plain value so the
// rendering helpers stay free of daemon-package calls and side effects.
func collectDaemonStatus() daemonStatus {
	st := daemonStatus{
		Installed: daemon.IsInstalled(),
		Running:   daemon.IsRunning(),
	}
	manager, err := daemon.CurrentManager()
	if err != nil {
		st.ManagerError = err.Error()
		return st
	}
	if st.Running {
		if pid, err := manager.Pid(); err == nil {
			st.PID = pid
		}
	}
	if st.Installed {
		st.ServicePath = manager.ServicePath()
	}
	if err := manager.Preflight(); err != nil {
		st.PreflightError = err.Error()
	}
	return st
}

// tailDaemonLogs returns the last few daemon log lines, or "" when the manager
// or its logs are unavailable.
func tailDaemonLogs() string {
	manager, err := daemon.CurrentManager()
	if err != nil {
		return ""
	}
	logs, err := manager.Logs(5)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(logs)
}
