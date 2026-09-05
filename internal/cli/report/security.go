package report

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/routes"
)

type SecurityWarning struct {
	ID      string
	Message string
	Attrs   []any
}

type SecurityPostureCheck struct {
	ID     string
	Label  string
	Passed bool
	Detail string
	// Settings are the config keys the row summarises, so a change report can say
	// which row a written key shows up in, and which keys no row shows.
	Settings []string
}

type SecurityPosture struct {
	Checks []SecurityPostureCheck
	Passed int
	Total  int
	Level  string
}

func AssessSecurityPosture(cfg *config.RuntimeConfig) SecurityPosture {
	if cfg == nil {
		return SecurityPosture{Level: "UNKNOWN"}
	}

	checks := []SecurityPostureCheck{
		{
			ID:       "bind_loopback",
			Settings: []string{"server.bind"},
			Label:    "loopback bind",
			Passed:   isLoopbackBind(cfg.Bind),
			Detail:   cfg.Bind,
		},
		{
			ID:       "api_auth_enabled",
			Settings: []string{"server.token"},
			Label:    "api auth",
			Passed:   strings.TrimSpace(cfg.Token) != "",
			Detail:   map[bool]string{true: "required", false: "disabled"}[strings.TrimSpace(cfg.Token) != ""],
		},
		{
			ID:       "sensitive_endpoints_disabled",
			Settings: capabilitySettings(),
			Label:    "sensitive endpoints",
			Passed:   len(cfg.EnabledSensitiveEndpoints()) == 0,
			Detail:   formatEndpointStatus(cfg.EnabledSensitiveEndpoints()),
		},
		{
			ID:       "attach_local_only",
			Settings: []string{"security.attach.allowHosts"},
			Label:    "attach host scope",
			Passed:   !attachAllowsNonLocalHosts(cfg.AttachAllowHosts),
			Detail:   formatHostScope(cfg.AttachAllowHosts),
		},
		{
			ID:       "idpi_whitelist_scoped",
			Settings: []string{"security.idpi.enabled", "security.allowedDomains"},
			Label:    "website whitelist",
			Passed:   cfg.IDPI.Enabled && len(cfg.AllowedDomains) > 0 && !allowsAllDomains(cfg.AllowedDomains),
			Detail:   formatWhitelistStatus(cfg),
		},
		{
			ID:       "idpi_strict_mode",
			Settings: []string{"security.idpi.enabled", "security.idpi.strictMode"},
			Label:    "IDPI strict mode",
			Passed:   cfg.IDPI.Enabled && cfg.IDPI.StrictMode,
			Detail:   formatStrictModeStatus(cfg),
		},
		{
			ID:       "idpi_content_protection",
			Settings: []string{"security.idpi.enabled", "security.idpi.scanContent", "security.idpi.wrapContent"},
			Label:    "IDPI content guard",
			Passed:   cfg.IDPI.Enabled && (cfg.IDPI.ScanContent || cfg.IDPI.WrapContent),
			Detail:   formatContentGuardStatus(cfg),
		},
	}

	passed := 0
	for _, check := range checks {
		if check.Passed {
			passed++
		}
	}

	return SecurityPosture{
		Checks: checks,
		Passed: passed,
		Total:  len(checks),
		Level:  securityPostureLevel(passed, len(checks)),
	}
}

func capabilitySettings() []string {
	settings := make([]string, 0)
	for cap := range routes.CapabilityEndpoints() {
		if meta, ok := routes.Meta(cap); ok {
			settings = append(settings, meta.Setting)
		}
	}
	sort.Strings(settings)
	return settings
}

// PostureRowsForSetting names the posture rows a config key feeds; empty means the
// key is invisible in the posture table and a change report must say so itself.
func PostureRowsForSetting(path string) []string {
	rows := make([]string, 0)
	for _, check := range AssessSecurityPosture(&config.RuntimeConfig{}).Checks {
		for _, setting := range check.Settings {
			if setting == path {
				rows = append(rows, check.Label)
			}
		}
	}
	return rows
}

func assessSecurityPosture(cfg *config.RuntimeConfig) SecurityPosture {
	return AssessSecurityPosture(cfg)
}

func AssessSecurityWarnings(cfg *config.RuntimeConfig) []SecurityWarning {
	if cfg == nil {
		return nil
	}

	warnings := make([]SecurityWarning, 0, 8)
	enabled := cfg.EnabledSensitiveEndpoints()

	if len(enabled) > 0 {
		warnings = append(warnings, SecurityWarning{
			ID:      "sensitive_endpoints_enabled",
			Message: "sensitive endpoints enabled",
			Attrs:   []any{"endpoints", enabled, "hint", "only enable them in trusted environments"},
		})
	}

	if cfg.Token == "" {
		warnings = append(warnings, SecurityWarning{
			ID:      "api_auth_disabled",
			Message: "api authentication disabled",
			Attrs:   []any{"hint", "set PINCHTAB_TOKEN to require bearer auth for all endpoints"},
		})
	}

	if len(enabled) > 0 && cfg.Token == "" {
		warnings = append(warnings, SecurityWarning{
			ID:      "sensitive_endpoints_without_auth",
			Message: "high-risk configuration: sensitive endpoints enabled without API authentication",
			Attrs:   []any{"endpoints", enabled, "hint", "set PINCHTAB_TOKEN or disable the sensitive endpoints"},
		})
	}

	if cfg.TrustProxyHeaders {
		hops := config.TrustedProxyHopsOrDefault(cfg.TrustedProxyHops)
		warnings = append(warnings, SecurityWarning{
			ID:      "trusted_proxy_hops",
			Message: fmt.Sprintf("forwarding headers are trusted; client identity is read %d hop(s) from the right of X-Forwarded-For", hops),
			Attrs: []any{
				"setting", "server.trustedProxyHops",
				"hops", hops,
				"hint", fmt.Sprintf("confirm %d equals the number of proxies in front that APPEND to X-Forwarded-For (nginx proxy_add_x_forwarded_for, Caddy and Traefik defaults each count as one); a higher number lets a client choose its own rate-limit bucket and audit identity", hops),
			},
		})
	}

	if !isLoopbackBind(cfg.Bind) {
		warnings = append(warnings, SecurityWarning{
			ID:      "non_loopback_bind",
			Message: "server exposed on a non-loopback bind address",
			Attrs:   []any{"bind", cfg.Bind, "hint", "non-loopback bind is a documented, non-default, security-reducing choice; keep a token set and review reverse proxy or port-publishing boundaries explicitly"},
		})
	}

	if !cfg.IDPI.Enabled {
		warnings = append(warnings, SecurityWarning{
			ID:      "idpi_disabled",
			Message: "IDPI disabled; website whitelist inactive",
			Attrs:   []any{"setting", "security.idpi.enabled", "hint", "enable IDPI and keep security.allowedDomains scoped to approved websites"},
		})
	} else {
		if len(cfg.AllowedDomains) == 0 {
			warnings = append(warnings, SecurityWarning{
				ID:      "idpi_whitelist_not_set",
				Message: "website whitelist is not set for IDPI",
				Attrs:   []any{"setting", "security.allowedDomains", "hint", "configure allowedDomains to restrict which websites navigation may reach"},
			})
		} else if allowsAllDomains(cfg.AllowedDomains) {
			warnings = append(warnings, SecurityWarning{
				ID:      "idpi_whitelist_allows_all",
				Message: "website whitelist allows all domains",
				Attrs:   []any{"setting", "security.allowedDomains", "hint", "remove '*' and list only approved domains"},
			})
		}

		if !cfg.IDPI.StrictMode {
			warnings = append(warnings, SecurityWarning{
				ID:      "idpi_warn_mode",
				Message: "IDPI strict mode disabled",
				Attrs:   []any{"setting", "security.idpi.strictMode", "hint", "enable strict mode to block requests instead of only emitting warnings"},
			})
		}

		if !cfg.IDPI.ScanContent && !cfg.IDPI.WrapContent {
			warnings = append(warnings, SecurityWarning{
				ID:      "idpi_content_protection_disabled",
				Message: "IDPI content protections are disabled",
				Attrs:   []any{"hint", "enable security.idpi.scanContent or security.idpi.wrapContent to protect text and snapshot responses"},
			})
		}
	}

	if allowsAllAttachHosts(cfg.AttachAllowHosts) {
		warnings = append(warnings, SecurityWarning{
			ID:      "attach_wildcard_hosts",
			Message: "attach allowHosts disables host allowlisting",
			Attrs: []any{
				"allowHosts", cfg.AttachAllowHosts,
				"hint", "remove '*' and list only approved hosts; wildcard is an explicit security-reducing override for isolated, operator-controlled networks only",
			},
		})
	} else if attachAllowsNonLocalHosts(cfg.AttachAllowHosts) {
		warnings = append(warnings, SecurityWarning{
			ID:      "attach_external_hosts",
			Message: "attach allowHosts includes non-local hosts",
			Attrs:   []any{"allowHosts", cfg.AttachAllowHosts, "hint", "keep security.attach.allowHosts limited to approved hosts you operate; broad entries expand the remote attach trust boundary"},
		})
	}

	return warnings
}

func (w SecurityWarning) Hint() string {
	for i := 0; i+1 < len(w.Attrs); i += 2 {
		if w.Attrs[i] == "hint" {
			hint, _ := w.Attrs[i+1].(string)
			return hint
		}
	}
	return ""
}

func LogSecurityWarnings(cfg *config.RuntimeConfig) {
	for _, warning := range AssessSecurityWarnings(cfg) {
		attrs := append([]any{"category", "security", "warningId", warning.ID}, warning.Attrs...)
		slog.Warn(warning.Message, attrs...)
	}
}

func isLoopbackBind(bind string) bool {
	switch strings.TrimSpace(strings.ToLower(bind)) {
	case "127.0.0.1", "localhost", "::1", "":
		return true
	default:
		return false
	}
}

func allowsAllDomains(domains []string) bool {
	for _, domain := range domains {
		if strings.TrimSpace(domain) == "*" {
			return true
		}
	}
	return false
}

func allowsAllAttachHosts(hosts []string) bool {
	for _, host := range hosts {
		if strings.TrimSpace(host) == "*" {
			return true
		}
	}
	return false
}

func attachAllowsNonLocalHosts(hosts []string) bool {
	if len(hosts) == 0 {
		return false
	}
	for _, host := range hosts {
		switch strings.TrimSpace(strings.ToLower(host)) {
		case "", "127.0.0.1", "localhost", "::1":
		default:
			return true
		}
	}
	return false
}

func securityPostureLevel(passed, total int) string {
	if total == 0 {
		return "UNKNOWN"
	}
	switch {
	case passed == total:
		return "LOCKED"
	case passed >= total-1:
		return "GUARDED"
	case passed >= 3:
		return "ELEVATED"
	default:
		return "EXPOSED"
	}
}

func formatEndpointStatus(enabled []string) string {
	if len(enabled) == 0 {
		return "disabled"
	}
	return strings.Join(enabled, ", ")
}

func formatHostScope(hosts []string) string {
	if allowsAllAttachHosts(hosts) {
		return "wildcard (*)"
	}
	if attachAllowsNonLocalHosts(hosts) {
		return "external hosts allowed"
	}
	return "local-only"
}

func formatWhitelistStatus(cfg *config.RuntimeConfig) string {
	if !cfg.IDPI.Enabled {
		return "disabled"
	}
	if len(cfg.AllowedDomains) == 0 {
		return "not set"
	}
	if allowsAllDomains(cfg.AllowedDomains) {
		return "wildcard"
	}
	return strings.Join(cfg.AllowedDomains, ", ")
}

func formatStrictModeStatus(cfg *config.RuntimeConfig) string {
	if !cfg.IDPI.Enabled {
		return "disabled"
	}
	if cfg.IDPI.StrictMode {
		return "enforcing"
	}
	return "warn-only"
}

func formatContentGuardStatus(cfg *config.RuntimeConfig) string {
	if !cfg.IDPI.Enabled {
		return "disabled"
	}
	if cfg.IDPI.ScanContent || cfg.IDPI.WrapContent {
		return "active"
	}
	return "disabled"
}
