package report

import (
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
)

func ApplyRecommendedSecurityDefaults(fc *config.FileConfig) {
	workflow.ApplyRecommendedSecurityDefaults(fc)
}

func applyRecommendedSecurityDefaults(fc *config.FileConfig) {
	ApplyRecommendedSecurityDefaults(fc)
}

func RestoreSecurityDefaults() (string, bool, error) {
	return workflow.RestoreSecurityDefaults()
}

func restoreSecurityDefaults() (string, bool, error) {
	return RestoreSecurityDefaults()
}

func RecommendedSecurityDefaultLines(cfg *config.RuntimeConfig) []string {
	posture := AssessSecurityPosture(cfg)
	ordered := []string{
		"server.bind = 127.0.0.1",
		"security.allowEvaluate = false",
		"security.allowMacro = false",
		"security.allowScreencast = false",
		"security.allowDownload = false",
		"security.allowUpload = false",
		"security.attach.enabled = false",
		"security.attach.allowHosts = 127.0.0.1,localhost,::1",
		"security.attach.allowSchemes = ws,wss",
		"security.idpi.enabled = true",
		"security.idpi.allowedDomains = 127.0.0.1,localhost,::1",
		"security.idpi.strictMode = true",
		"security.idpi.scanContent = true",
		"security.idpi.wrapContent = true",
	}
	needed := make(map[string]bool, len(ordered))

	for _, check := range posture.Checks {
		if check.Passed {
			continue
		}
		switch check.ID {
		case "bind_loopback":
			needed["server.bind = 127.0.0.1"] = true
		case "sensitive_endpoints_disabled":
			for _, line := range []string{
				"security.allowEvaluate = false",
				"security.allowMacro = false",
				"security.allowScreencast = false",
				"security.allowDownload = false",
				"security.allowUpload = false",
			} {
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
				"security.idpi.allowedDomains = 127.0.0.1,localhost,::1",
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
