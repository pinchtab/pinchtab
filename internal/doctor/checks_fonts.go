package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/config"
)

func isWindowsPlatform(p string) bool {
	p = strings.ToLower(strings.TrimSpace(p))
	return p == "windows" || strings.HasPrefix(p, "win")
}

func checkLinuxFontsPresent(ctx context.Context, cfg *config.RuntimeConfig) CheckResult {
	host := HostOS()
	if host != "linux" {
		return CheckResult{
			Status: StatusSkip,
			Detail: fmt.Sprintf("not applicable on %s (only enforced on linux hosts)", host),
		}
	}
	if cfg == nil || !isWindowsPlatform(cfg.Cloak.Platform) {
		return CheckResult{
			Status: StatusSkip,
			Detail: "skipped (cloak.platform != windows)",
		}
	}

	probe := browserprobe.ProbeWindowsFingerprintFonts(ctx)
	if len(probe.Matched) == 0 {
		detail := "no windows fingerprint fonts found via filesystem scan (install msttcorefonts or mount a Windows fonts dir)"
		if probe.Source == "fc-list" {
			detail = fmt.Sprintf("%s found no windows fingerprint fonts (expected one of: %s)", probe.Source, strings.Join(probe.Expected, ", "))
		}
		return CheckResult{
			Status: StatusWarn,
			Detail: detail,
		}
	}
	return CheckResult{
		Status: StatusPass,
		Detail: fmt.Sprintf("%s: %s", probe.Source, strings.Join(probe.Matched, ", ")),
	}
}
