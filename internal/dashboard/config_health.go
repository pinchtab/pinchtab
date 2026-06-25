package dashboard

import (
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/cli/report"
	"github.com/pinchtab/pinchtab/internal/config"
)

type healthInstanceInfo struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type healthSecurityInfo struct {
	Level                     string   `json:"level"`
	Bind                      string   `json:"bind"`
	AllowedDomains            []string `json:"allowedDomains"`
	IDPIEnabled               bool     `json:"idpiEnabled"`
	EnabledSensitiveEndpoints []string `json:"enabledSensitiveEndpoints"`
	GuardsDown                bool     `json:"guardsDown"`
}

type healthEnvelope struct {
	Status          string              `json:"status"`
	Mode            string              `json:"mode"`
	Version         string              `json:"version"`
	Uptime          int64               `json:"uptime"`
	AuthRequired    bool                `json:"authRequired"`
	Profiles        int                 `json:"profiles"`
	Instances       int                 `json:"instances"`
	DefaultInstance *healthInstanceInfo `json:"defaultInstance,omitempty"`
	Agents          int                 `json:"agents"`
	RestartRequired bool                `json:"restartRequired"`
	RestartReasons  []string            `json:"restartReasons,omitempty"`
	Security        *healthSecurityInfo `json:"security,omitempty"`
}

func (c *ConfigAPI) healthInfo(includeSecurity bool) (healthEnvelope, error) {
	_, _, restartReasons, err := c.currentConfig()
	if err != nil {
		return healthEnvelope{}, err
	}

	profileCount := 0
	if c.profiles != nil {
		profiles, err := c.profiles.List()
		if err == nil {
			profileCount = len(profiles)
		}
	}

	instanceCount := 0
	var defaultInst *healthInstanceInfo
	if c.instances != nil {
		instances := c.instances.List()
		instanceCount = len(instances)
		if len(instances) > 0 {
			defaultInst = &healthInstanceInfo{
				ID:     instances[0].ID,
				Status: instances[0].Status,
			}
		}
	}
	agentCount := 0
	if c.agents != nil {
		agentCount = c.agents.AgentCount()
	}
	out := healthEnvelope{
		Status:          "ok",
		Mode:            "dashboard",
		Version:         c.version,
		Uptime:          int64(time.Since(c.startedAt).Milliseconds()),
		AuthRequired:    strings.TrimSpace(c.runtime.Token) != "",
		Profiles:        profileCount,
		Instances:       instanceCount,
		DefaultInstance: defaultInst,
		Agents:          agentCount,
		RestartRequired: len(restartReasons) > 0,
		RestartReasons:  restartReasons,
	}
	if includeSecurity {
		security := runtimeSecurityInfo(c.runtime)
		out.Security = &security
	}
	return out, nil
}

func healthSecurityVisibleTo(r *http.Request) bool {
	switch authn.CredentialsFromRequest(r).Method {
	case authn.MethodHeader, authn.MethodCookie:
		return true
	default:
		return false
	}
}

func runtimeSecurityInfo(cfg *config.RuntimeConfig) healthSecurityInfo {
	if cfg == nil {
		return healthSecurityInfo{Level: "UNKNOWN"}
	}
	posture := report.AssessSecurityPosture(cfg)
	enabled := append([]string(nil), cfg.EnabledSensitiveEndpoints()...)
	domains := append([]string(nil), cfg.AllowedDomains...)
	return healthSecurityInfo{
		Level:                     posture.Level,
		Bind:                      cfg.Bind,
		AllowedDomains:            domains,
		IDPIEnabled:               cfg.IDPI.Enabled,
		EnabledSensitiveEndpoints: enabled,
		GuardsDown:                isGuardsDownPosture(cfg),
	}
}

// isGuardsDownPosture reports whether the runtime config matches the
// guards-down preset signature (all sensitive endpoints + attach + IDPI off).
func isGuardsDownPosture(cfg *config.RuntimeConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.AllowEvaluate &&
		cfg.AllowMacro &&
		cfg.AllowScreencast &&
		cfg.AllowDownload &&
		cfg.AllowCookies &&
		cfg.AllowUpload &&
		cfg.AllowNetworkIntercept &&
		cfg.AttachEnabled &&
		!cfg.IDPI.Enabled
}
