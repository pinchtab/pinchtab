package chrome

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers"
)

const chromeMinVersion = "120.0.0"
const chromeDoctorLaunchTimeout = 60 * time.Second

var probeCDP = LaunchAndProbe

func (b Browser) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck {
	return []browsers.DoctorCheck{
		{
			ID:          "chrome_present",
			Description: "Chrome/Chromium binary found and version adequate",
			Fn:          chromePresenceCheck,
		},
		{
			ID:              "cdp_reachable",
			Description:     "Chrome launches headless and accepts CDP attach",
			Fn:              cdpReachableCheck,
			LaunchesBrowser: true,
		},
		browsers.HandleDecisionsCheck(b, "Chrome"),
	}
}

func resolveDoctorBinary(env *browsers.DoctorEnv) browsers.DoctorBinary {
	return browsers.ResolveDoctorBinary(env, BinaryNames(), CommonPaths(runtime.GOOS))
}

func chromeAbsent(probed []string) browsers.DoctorCheckResult {
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorFail,
		Detail: "no chrome/chromium found. Install Chrome/Chromium (" +
			browsers.InstallHint(runtime.GOOS) + ") or set " +
			"browser.binary to an existing build. Probed: " + strings.Join(probed, ", "),
	}
}

func cdpReachableCheck(ctx context.Context, env *browsers.DoctorEnv) browsers.DoctorCheckResult {
	return browsers.CDPReachable(resolveDoctorBinary(env), chromeAbsent, func(path string) (int, error) {
		res, err := probeCDP(ctx, path, browsers.DoctorSandboxArgs(env), chromeDoctorLaunchTimeout)
		return res.Port, err
	})
}

func chromePresenceCheck(ctx context.Context, env *browsers.DoctorEnv) browsers.DoctorCheckResult {
	binary := resolveDoctorBinary(env)
	if binary.Path == "" {
		return chromeAbsent(binary.Probed)
	}
	token, result := browsers.VersionCheck(ctx, binary, chromeMinVersion, nil)
	if result.Status != browsers.DoctorPass || binary.Overridden || runtime.GOOS != "darwin" || binary.Path != primaryChromeAppMacOS {
		return result
	}
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorPass,
		Detail: fmt.Sprintf("%s -> %s (>= %s); note: this is your primary Google Chrome — "+
			"automating it on macOS can stop your normal Chrome from opening (issue #583). "+
			"Install Google Chrome for Testing or Chromium, or set browser.binary.",
			binary.Path, token, chromeMinVersion),
	}
}
