package cli

import (
	"github.com/pinchtab/pinchtab/internal/cli/report"
	"github.com/pinchtab/pinchtab/internal/config"
)

type StartupBannerOptions = report.StartupBannerOptions
type SecurityWarning = report.SecurityWarning
type SecurityPosture = report.SecurityPosture
type SecurityPostureCheck = report.SecurityPostureCheck

func PrintStartupBanner(cfg *config.RuntimeConfig, opts StartupBannerOptions) {
	report.PrintStartupBanner(cfg, opts)
}

func HandleConfigShow(cfg *config.RuntimeConfig) {
	report.HandleConfigShow(cfg)
}

func IsDaemonRunning() bool {
	return report.IsDaemonRunning()
}

func AssessSecurityWarnings(cfg *config.RuntimeConfig) []SecurityWarning {
	return report.AssessSecurityWarnings(cfg)
}

func AssessSecurityPosture(cfg *config.RuntimeConfig) SecurityPosture {
	return report.AssessSecurityPosture(cfg)
}

func RecommendedSecurityDefaultLines(cfg *config.RuntimeConfig) []string {
	return report.RecommendedSecurityDefaultLines(cfg)
}

func LogSecurityWarnings(cfg *config.RuntimeConfig) {
	report.LogSecurityWarnings(cfg)
}
