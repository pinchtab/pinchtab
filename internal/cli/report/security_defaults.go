package report

import (
	"sort"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// capabilityDisableLines is the one owner of the "disable capability X" recommendation
// lines, derived from the canonical capability table rather than restated: a ninth
// capability appears here — and in the sensitive-endpoints recommendation that reuses
// these lines — with no edit. Sorted so the report output is stable.
func capabilityDisableLines() []string {
	lines := make([]string, 0)
	for cap := range routes.CapabilityEndpoints() {
		meta, ok := routes.Meta(cap)
		if !ok {
			continue
		}
		lines = append(lines, meta.Setting+" = false")
	}
	sort.Strings(lines)
	return lines
}

func RecommendedSecurityDefaultLines(cfg *config.RuntimeConfig) []string {
	posture := AssessSecurityPosture(cfg)
	capabilityLines := capabilityDisableLines()
	ordered := []string{"server.bind = 127.0.0.1"}
	ordered = append(ordered, capabilityLines...)
	ordered = append(ordered,
		"security.attach.enabled = false",
		"security.attach.allowHosts = 127.0.0.1,localhost,::1",
		"security.attach.allowSchemes = ws,wss",
		"security.idpi.enabled = true",
		"security.allowedDomains = 127.0.0.1,localhost,::1",
		"security.idpi.strictMode = true",
		"security.idpi.scanContent = true",
		"security.idpi.wrapContent = true",
	)
	needed := make(map[string]bool, len(ordered))

	for _, check := range posture.Checks {
		if check.Passed {
			continue
		}
		switch check.ID {
		case "bind_loopback":
			needed["server.bind = 127.0.0.1"] = true
		case "sensitive_endpoints_disabled":
			for _, line := range capabilityLines {
				needed[line] = true
			}
		case "attach_disabled", "attach_local_only":
			for _, line := range []string{
				"security.attach.enabled = false",
				"security.attach.allowHosts = 127.0.0.1,localhost,::1",
				"security.attach.allowSchemes = ws,wss",
			} {
				needed[line] = true
			}
		case "idpi_whitelist_scoped", "idpi_strict_mode", "idpi_content_protection":
			for _, line := range []string{
				"security.idpi.enabled = true",
				"security.allowedDomains = 127.0.0.1,localhost,::1",
				"security.idpi.strictMode = true",
				"security.idpi.scanContent = true",
				"security.idpi.wrapContent = true",
			} {
				needed[line] = true
			}
		}
	}

	lines := make([]string, 0, len(needed))
	for _, line := range ordered {
		if needed[line] {
			lines = append(lines, line)
		}
	}
	return lines
}
