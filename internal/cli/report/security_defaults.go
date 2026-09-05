package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/routes"
)

func capabilityDisableLines() []string {
	return capabilityDisableLinesWhere(func(routes.Capability) bool { return true })
}

func capabilityDisableLinesWhere(include func(routes.Capability) bool) []string {
	lines := make([]string, 0)
	for cap := range routes.CapabilityEndpoints() {
		meta, ok := routes.Meta(cap)
		if !ok || !include(cap) {
			continue
		}
		lines = append(lines, meta.Setting+" = false")
	}
	sort.Strings(lines)
	return lines
}

func RecommendedSecurityDefaultLines(cfg *config.RuntimeConfig) []string {
	if cfg == nil {
		return nil
	}
	want := config.DefaultFileConfig()
	lines := make([]string, 0)
	if cfg.Bind != want.Server.Bind {
		lines = append(lines, "server.bind = "+want.Server.Bind)
	}
	lines = append(lines, capabilityDisableLinesWhere(cfg.CapabilityEnabled)...)
	attach := want.Security.Attach
	if attach.Enabled != nil && cfg.AttachEnabled != *attach.Enabled {
		lines = append(lines, fmt.Sprintf("security.attach.enabled = %t", *attach.Enabled))
	}
	if attachAllowsNonLocalHosts(cfg.AttachAllowHosts) {
		lines = append(lines, "security.attach.allowHosts = "+strings.Join(attach.AllowHosts, ","))
	}
	if idpi := want.Security.IDPI; idpi != nil {
		for _, flag := range []struct {
			path       string
			have, want bool
		}{
			{"security.idpi.enabled", cfg.IDPI.Enabled, idpi.Enabled},
			{"security.idpi.strictMode", cfg.IDPI.StrictMode, idpi.StrictMode},
			{"security.idpi.scanContent", cfg.IDPI.ScanContent, idpi.ScanContent},
			{"security.idpi.wrapContent", cfg.IDPI.WrapContent, idpi.WrapContent},
		} {
			if flag.have != flag.want {
				lines = append(lines, fmt.Sprintf("%s = %t", flag.path, flag.want))
			}
		}
	}
	return lines
}
