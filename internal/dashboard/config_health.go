package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cli/report"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
)

// frontDoorConfigurationScope labels health.security as this process's own
// configuration: enforcement lives in the instance processes and is reported
// under enforcedSecurity.
const frontDoorConfigurationScope = "frontDoorConfiguration"

type healthInstanceInfo struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type healthSecurityInfo struct {
	Scope                     string   `json:"scope"`
	Level                     string   `json:"level"`
	Bind                      string   `json:"bind"`
	AllowedDomains            []string `json:"allowedDomains"`
	IDPIEnabled               bool     `json:"idpiEnabled"`
	EnabledSensitiveEndpoints []string `json:"enabledSensitiveEndpoints"`
	GuardsDown                bool     `json:"guardsDown"`
}

type healthEnvelope struct {
	Status              string                  `json:"status"`
	Mode                string                  `json:"mode"`
	Version             string                  `json:"version"`
	DashboardBuild      string                  `json:"dashboardBuild"`
	Uptime              int64                   `json:"uptime"`
	AuthRequired        bool                    `json:"authRequired"`
	Profiles            int                     `json:"profiles"`
	TemporaryProfiles   int                     `json:"temporaryProfiles"`
	QuarantinedProfiles int                     `json:"quarantinedProfiles"`
	Instances           int                     `json:"instances"`
	DefaultInstance     *healthInstanceInfo     `json:"defaultInstance,omitempty"`
	Agents              int                     `json:"agents"`
	RestartRequired     bool                    `json:"restartRequired"`
	RestartReasons      []string                `json:"restartReasons,omitempty"`
	Security            *healthSecurityInfo     `json:"security,omitempty"`
	EnforcedSecurity    *healthEnforcedSecurity `json:"enforcedSecurity,omitempty"`
	Crashes             *bridge.CrashSummary    `json:"crashes,omitempty"`
}

// healthEnforcedInstance is one instance process's posture. Comparison is the
// three-state answer the front door can honestly give: "match", "diverges", or
// "unknown" when the instance did not answer — an instance nobody could query
// must not read as one enforcing nothing.
type healthEnforcedInstance struct {
	ID         string                     `json:"id"`
	Queried    bool                       `json:"queried"`
	Comparison string                     `json:"comparison"`
	Policy     *workflow.EnforcedSecurity `json:"policy,omitempty"`
}

type healthEnforcedSecurity struct {
	Instances []healthEnforcedInstance `json:"instances"`
	Divergent bool                     `json:"divergent"`
}

type enforcedSecurityReporter interface {
	EnforcedSecurity() map[string]*workflow.EnforcedSecurity
}

type crashReporter interface {
	CrashSummary() bridge.CrashSummary
}

func (c *ConfigAPI) healthInfo(includeSecurity bool) (healthEnvelope, error) {
	_, _, restartReasons, err := c.currentConfig()
	if err != nil {
		return healthEnvelope{}, err
	}

	profileCount, temporaryCount, quarantinedCount := 0, 0, 0
	if c.profiles != nil {
		if profiles, err := c.profiles.List(); err == nil {
			profileCount, temporaryCount, quarantinedCount = tallyProfiles(profiles)
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
	cfg := c.cfg()
	out := healthEnvelope{
		Status:              "ok",
		Mode:                "dashboard",
		Version:             c.version,
		DashboardBuild:      BundleStamp(),
		Uptime:              int64(time.Since(c.startedAt).Milliseconds()),
		AuthRequired:        cfg != nil && strings.TrimSpace(cfg.Token) != "",
		Profiles:            profileCount,
		TemporaryProfiles:   temporaryCount,
		QuarantinedProfiles: quarantinedCount,
		Instances:           instanceCount,
		DefaultInstance:     defaultInst,
		Agents:              agentCount,
		RestartRequired:     len(restartReasons) > 0,
		RestartReasons:      restartReasons,
	}
	if includeSecurity {
		security := runtimeSecurityInfo(cfg)
		out.Security = &security
		out.EnforcedSecurity = c.enforcedSecurityInfo(cfg)
	}
	if reporter, ok := c.instances.(crashReporter); ok {
		if crashes := reporter.CrashSummary(); crashes.Total > 0 {
			out.Crashes = &crashes
		}
	}
	return out, nil
}

func tallyProfiles(profiles []bridge.ProfileInfo) (live, temporary, quarantined int) {
	for _, p := range profiles {
		switch {
		case p.Temporary:
			temporary++
		case p.Quarantined:
			quarantined++
		default:
			live++
		}
	}
	return live, temporary, quarantined
}

func healthSecurityVisibleTo(r *http.Request) bool {
	switch authn.CredentialsFromRequest(r).Method {
	case authn.MethodHeader, authn.MethodCookie:
		return true
	default:
		return false
	}
}

// enforcedSecurityInfo reports what the instance processes are enforcing, which
// is the posture they snapshotted at their own boot — the front door's configured
// policy speaks only for the front door.
func (c *ConfigAPI) enforcedSecurityInfo(cfg *config.RuntimeConfig) *healthEnforcedSecurity {
	reporter, ok := c.instances.(enforcedSecurityReporter)
	if !ok {
		return nil
	}
	postures := reporter.EnforcedSecurity()
	configured := workflow.EnforcedSecurityFor(cfg)

	out := healthEnforcedSecurity{Instances: []healthEnforcedInstance{}}
	for _, inst := range c.instances.List() {
		if inst.Status != bridge.InstanceStatusRunning {
			continue
		}
		entry := healthEnforcedInstance{ID: inst.ID, Comparison: "unknown"}
		if posture, present := postures[inst.ID]; present && posture != nil {
			entry.Queried = true
			entry.Policy = posture
			entry.Comparison = "diverges"
			if sameEnforcement(*posture, configured) {
				entry.Comparison = "match"
			}
		}
		if entry.Comparison != "match" {
			out.Divergent = true
		}
		out.Instances = append(out.Instances, entry)
	}
	sort.Slice(out.Instances, func(i, j int) bool { return out.Instances[i].ID < out.Instances[j].ID })
	return &out
}

func sameEnforcement(a, b workflow.EnforcedSecurity) bool {
	left, err := json.Marshal(normalizeEnforcement(a))
	if err != nil {
		return false
	}
	right, err := json.Marshal(normalizeEnforcement(b))
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}

func normalizeEnforcement(p workflow.EnforcedSecurity) workflow.EnforcedSecurity {
	p.AllowedDomains = append([]string{}, p.AllowedDomains...)
	p.EnabledSensitiveEndpoints = append([]string{}, p.EnabledSensitiveEndpoints...)
	sort.Strings(p.AllowedDomains)
	sort.Strings(p.EnabledSensitiveEndpoints)
	return p
}

func runtimeSecurityInfo(cfg *config.RuntimeConfig) healthSecurityInfo {
	if cfg == nil {
		return healthSecurityInfo{Scope: frontDoorConfigurationScope, Level: "UNKNOWN"}
	}
	posture := report.AssessSecurityPosture(cfg)
	enabled := append([]string(nil), cfg.EnabledSensitiveEndpoints()...)
	domains := append([]string(nil), cfg.AllowedDomains...)
	return healthSecurityInfo{
		Scope:                     frontDoorConfigurationScope,
		Level:                     posture.Level,
		Bind:                      cfg.Bind,
		AllowedDomains:            domains,
		IDPIEnabled:               cfg.IDPI.Enabled,
		EnabledSensitiveEndpoints: enabled,
		GuardsDown:                workflow.GuardsDownPostureActive(cfg),
	}
}
