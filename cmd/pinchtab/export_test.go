package main

import "time"

func ensureServerWith(baseURL, token, command string, start serverStartFunc, probe serverProbeFunc, timeout time.Duration) error {
	return ensureServerWithAutoStart(baseURL, token, command, true, start, probe, timeout)
}

func isServerHealthy(baseURL, token string) bool {
	return serverProbeHealthy(probeServerHealth(baseURL, token))
}

func waitForServer(baseURL, token string, timeout time.Duration) bool {
	return waitForServerWith(baseURL, token, timeout, isServerHealthy)
}

func useLocalTabStateFile() bool { return defaultTabState.useLocal() }

func tabStateFile() string { return defaultTabState.path() }
