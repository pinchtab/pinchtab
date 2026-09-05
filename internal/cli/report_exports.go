package cli

import (
	"github.com/pinchtab/pinchtab/internal/cli/report"
	"github.com/pinchtab/pinchtab/internal/config"
)

type StartupBannerOptions = report.StartupBannerOptions

func PrintStartupBanner(cfg *config.RuntimeConfig, opts StartupBannerOptions) {
	report.PrintStartupBanner(cfg, opts)
}

func HandleConfigShow(cfg *config.RuntimeConfig) {
	report.HandleConfigShow(cfg)
}

func IsDaemonRunning() bool {
	return report.IsDaemonRunning()
}

func AssessSecurityWarnings(cfg *config.RuntimeConfig) []report.SecurityWarning {
	return report.AssessSecurityWarnings(cfg)
}

func AssessSecurityPosture(cfg *config.RuntimeConfig) report.SecurityPosture {
	return report.AssessSecurityPosture(cfg)
}

func PostureRowsForSetting(path string) []string {
	return report.PostureRowsForSetting(path)
}

func RecommendedSecurityDefaultLines(cfg *config.RuntimeConfig) []string {
	return report.RecommendedSecurityDefaultLines(cfg)
}

func LogSecurityWarnings(cfg *config.RuntimeConfig) {
	report.LogSecurityWarnings(cfg)
}
