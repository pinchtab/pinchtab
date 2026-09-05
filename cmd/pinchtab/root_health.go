package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/server"
)

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

func fetchHealthSnapshot(port string) (*healthSnapshot, healthSnapshotState) {
	return fetchHealthSnapshotWithToken(port, "")
}

func fetchHealthSnapshotWithToken(port, token string) (*healthSnapshot, healthSnapshotState) {
	probe := server.ProbeHealthWithToken(fmt.Sprintf("http://localhost:%s/health", port), 500*time.Millisecond, token)
	if !probe.Reachable {
		return nil, healthSnapshotStopped
	}
	switch probe.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, healthSnapshotProtected
	default:
		return nil, healthSnapshotUnhealthy
	}
	var snap healthSnapshot
	if err := json.Unmarshal(probe.Body, &snap); err != nil {
		return nil, healthSnapshotInvalid
	}
	if snap.Status != "ok" || snap.Mode != "dashboard" || strings.TrimSpace(snap.Version) == "" {
		return nil, healthSnapshotInvalid
	}
	return &snap, healthSnapshotRunning
}
