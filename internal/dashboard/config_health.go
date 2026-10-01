package dashboard

import (
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cli/report"
	"github.com/pinchtab/pinchtab/internal/config"
)

func unresponsiveInstanceIDs(instances []bridge.Instance) []string {
	var ids []string
	for _, inst := range instances {
		if inst.Responsiveness == bridge.ResponsivenessUnresponsive {
			ids = append(ids, inst.ID)
		}
	}
	return ids
}

type crashReporter interface {
	CrashSummary() bridge.CrashSummary
}

type defaultInstancer interface {
	DefaultInstance() (bridge.Instance, bool)
}

func defaultInstanceInfo(lister InstanceLister, instances []bridge.Instance) *types.HealthInstanceInfo {
	def, ok := bridge.Instance{}, false
	if d, isDefaulter := lister.(defaultInstancer); isDefaulter {
		def, ok = d.DefaultInstance()
	}
	if !ok {
		if len(instances) == 0 {
			return nil
		}
		def = instances[0]
	}
	return &types.HealthInstanceInfo{
		ID:             def.ID,
		Status:         string(def.Status),
		Responsiveness: def.Responsiveness,
	}
}

func (c *ConfigAPI) healthInfo(includeSecurity bool) (types.DashboardHealth, error) {
	_, _, restartReasons, err := c.currentConfig()
	if err != nil {
		return types.DashboardHealth{}, err
	}

	profileCount, temporaryCount, quarantinedCount := 0, 0, 0
	if c.profiles != nil {
		profiles, err := c.profiles.List()
		if err == nil {
			for _, p := range profiles {
				// Temporary is checked before Quarantined to match GET /profiles'
				// own precedence, which hides Temporary first: a quarantined
				// temporary (an instance-*.quarantine-* dir) is a temporary the
				// listing hides, so counting it as quarantined would break the
				// reconciliation profiles + quarantinedProfiles == default list length.
				switch {
				case p.Temporary:
					temporaryCount++
				case p.Quarantined:
					quarantinedCount++
				default:
					profileCount++
				}
			}
		}
	}

	var crashes *types.CrashSummary
	if reporter, ok := c.instances.(crashReporter); ok {
		if summary := reporter.CrashSummary(); summary.Total > 0 {
			crashes = wireCrashSummary(summary)
		}
	}
	instanceCount := 0
	var defaultInst *types.HealthInstanceInfo
	var unresponsive []string
	if c.instances != nil {
		instances := c.instances.List()
		instanceCount = len(instances)
		unresponsive = unresponsiveInstanceIDs(instances)
		defaultInst = defaultInstanceInfo(c.instances, instances)
	}
	status := types.HealthStatusOK
	if len(unresponsive) > 0 {
		status = types.HealthStatusDegraded
	}
	agentCount := 0
	if c.agents != nil {
		agentCount = c.agents.AgentCount()
	}
	cfg := c.cfg()
	out := types.DashboardHealth{
		Status:                status,
		Mode:                  types.ModeDashboard,
		Version:               c.version,
		Uptime:                int64(time.Since(c.startedAt).Milliseconds()),
		AuthRequired:          cfg != nil && strings.TrimSpace(cfg.Token) != "",
		Profiles:              profileCount,
		TemporaryProfiles:     temporaryCount,
		QuarantinedProfiles:   quarantinedCount,
		Instances:             instanceCount,
		DefaultInstance:       defaultInst,
		Agents:                agentCount,
		RestartRequired:       len(restartReasons) > 0,
		RestartReasons:        restartReasons,
		Crashes:               crashes,
		UnresponsiveInstances: unresponsive,
	}
	if includeSecurity {
		security := runtimeSecurityInfo(cfg)
		out.Security = &security
	}
	return out, nil
}

func wireCrashSummary(summary bridge.CrashSummary) *types.CrashSummary {
	recent := make([]types.CrashEvent, len(summary.Recent))
	for i, ev := range summary.Recent {
		recent[i] = types.CrashEvent{
			Time:       ev.Time,
			TargetID:   ev.TargetID,
			TabID:      ev.TabID,
			URL:        ev.URL,
			Reason:     ev.Reason,
			LastError:  ev.LastError,
			InstanceID: ev.InstanceID,
		}
	}
	return &types.CrashSummary{Total: summary.Total, Recent: recent}
}

func healthSecurityVisibleTo(r *http.Request) bool {
	switch authn.CredentialsFromRequest(r).Method {
	case authn.MethodHeader, authn.MethodCookie:
		return true
	default:
		return false
	}
}

func runtimeSecurityInfo(cfg *config.RuntimeConfig) types.HealthSecurityInfo {
	if cfg == nil {
		return types.HealthSecurityInfo{Level: "UNKNOWN"}
	}
	posture := report.AssessSecurityPosture(cfg)
	enabled := append([]string(nil), cfg.EnabledSensitiveEndpoints()...)
	domains := append([]string(nil), cfg.AllowedDomains...)
	return types.HealthSecurityInfo{
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
		cfg.AllowMemory &&
		cfg.AttachEnabled &&
		!cfg.IDPI.Enabled
}
