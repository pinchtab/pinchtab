//go:build !windows

package main

import "fmt"

func newPlatformDaemonManager(env daemonEnvironment, runner commandRunner) (daemonManager, error) {
	switch env.osName {
	case "linux":
		return &systemdUserManager{env: env, runner: runner}, nil
	case "darwin":
		return &launchdManager{env: env, runner: runner}, nil
	default:
		return nil, fmt.Errorf("pinchtab daemon is supported on macOS, Linux, and Windows; current OS is %s", env.osName)
	}
}

func managerEnvironmentPlatform(manager daemonManager) daemonEnvironment {
	switch m := manager.(type) {
	case *systemdUserManager:
		return m.env
	case *launchdManager:
		return m.env
	default:
		return daemonEnvironment{}
	}
}
