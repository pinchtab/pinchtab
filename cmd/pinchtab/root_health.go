package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/server"
)

// healthSnapshot is the subset of the /health response the landing banner and
// `pinchtab health` care about.
type healthSnapshot struct {
	Status          string   `json:"status"`
	Mode            string   `json:"mode"`
	Version         string   `json:"version"`
	RestartRequired bool     `json:"restartRequired"`
	RestartReasons  []string `json:"restartReasons"`
	Security        *struct {
		Level                     string   `json:"level"`
		AllowedDomains            []string `json:"allowedDomains"`
		IDPIEnabled               bool     `json:"idpiEnabled"`
		EnabledSensitiveEndpoints []string `json:"enabledSensitiveEndpoints"`
		GuardsDown                bool     `json:"guardsDown"`
	} `json:"security"`
}

type healthSnapshotState string

const (
	healthSnapshotStopped   healthSnapshotState = "stopped"
	healthSnapshotRunning   healthSnapshotState = "running"
	healthSnapshotProtected healthSnapshotState = "protected listener"
	healthSnapshotUnhealthy healthSnapshotState = "unhealthy"
	healthSnapshotInvalid   healthSnapshotState = "invalid health response"
)

func formatAllowedDomains(domains []string) string {
	if len(domains) == 0 {
		return "all"
	}
	for _, d := range domains {
		if strings.TrimSpace(d) == "*" {
			return "all"
		}
	}
	return strings.Join(domains, ", ")
}

// fetchHealthSnapshot probes the localhost listener and classifies the result.
// It is the only function here that performs network I/O, so callers that just
// need to print help/landing text can avoid the probe latency entirely.
func fetchHealthSnapshot(port string) (*healthSnapshot, healthSnapshotState) {
	return fetchHealthSnapshotWithToken(port, "")
}

// probeHealthSnapshot probes the localhost listener (with the token, so a
// protected /health is distinguished from a dead one) and decodes the body for
// ANY PinchTab mode. It classifies only reachability and auth; it does not judge
// the mode or serving status. fetchHealthSnapshotWithToken layers the
// dashboard-serving check on top for the landing banner and `pinchtab health`,
// while callers that only need the reported mode (the config-set restart hint)
// read snap.Mode from what this returns — a bridge, which reports no mode, comes
// back running with an empty Mode rather than being classed invalid.
func probeHealthSnapshot(port, token string) (*healthSnapshot, healthSnapshotState) {
	var headers map[string]string
	if auth := authn.AuthorizationHeaderValue(token); auth != "" {
		headers = map[string]string{"Authorization": auth}
	}
	status, body, reachable := server.ProbeHealth(fmt.Sprintf("http://localhost:%s/health", port), 500*time.Millisecond, headers)
	if !reachable {
		return nil, healthSnapshotStopped
	}
	switch status {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, healthSnapshotProtected
	default:
		return nil, healthSnapshotUnhealthy
	}
	var snap healthSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return nil, healthSnapshotInvalid
	}
	return &snap, healthSnapshotRunning
}

// fetchHealthSnapshotWithToken is fetchHealthSnapshot with optional auth, so it
// can read fields like restartRequired from a server that requires auth on
// /health (the unauthenticated probe would just see a protected listener). It
// answers "running" only for a serving dashboard front door.
func fetchHealthSnapshotWithToken(port, token string) (*healthSnapshot, healthSnapshotState) {
	snap, state := probeHealthSnapshot(port, token)
	if state != healthSnapshotRunning {
		return nil, state
	}
	if !types.HealthStatusServing(snap.Status) || snap.Mode != types.ModeDashboard || strings.TrimSpace(snap.Version) == "" {
		return nil, healthSnapshotInvalid
	}
	return snap, healthSnapshotRunning
}
