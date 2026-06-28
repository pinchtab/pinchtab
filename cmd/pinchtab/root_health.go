package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

// fetchHealthSnapshotWithToken is fetchHealthSnapshot with optional auth, so it
// can read fields like restartRequired from a server that requires auth on
// /health (the unauthenticated probe would just see a protected listener).
func fetchHealthSnapshotWithToken(port, token string) (*healthSnapshot, healthSnapshotState) {
	var headers map[string]string
	if auth := server.AuthorizationHeaderValue(token); auth != "" {
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
	if snap.Status != "ok" || snap.Mode != "dashboard" || strings.TrimSpace(snap.Version) == "" {
		return nil, healthSnapshotInvalid
	}
	return &snap, healthSnapshotRunning
}
