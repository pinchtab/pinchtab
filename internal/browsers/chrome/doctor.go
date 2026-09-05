package chrome

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
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
		{
			ID:          "handle_decisions",
			Description: "Verify Chrome handles all request shapes",
			Fn: func(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
				b := &Browser{}
				allShapes := []string{
					browsers.ShapeStaticRead, browsers.ShapeStaticSnapshot,
					browsers.ShapeRenderedRead, browsers.ShapeVisual,
					browsers.ShapeInteraction, browsers.ShapeSessionState,
					browsers.ShapeNetworkControl, browsers.ShapeDownloadUpload,
				}
				var unexpected []string
				for _, shape := range allShapes {
					d := b.CanHandle(browsers.RequestIntent{Shape: shape})
					if d.Decision != browsers.DecisionHandle {
						unexpected = append(unexpected, shape)
					}
				}
				if len(unexpected) > 0 {
					return browsers.DoctorCheckResult{
						Status: browsers.DoctorWarn,
						Detail: fmt.Sprintf("unexpected skip/fail for shapes: %s", strings.Join(unexpected, ", ")),
					}
				}
				return browsers.DoctorCheckResult{
					Status: browsers.DoctorPass,
					Detail: "all 8 request shapes handled",
				}
			},
		},
	}
}

func resolveDoctorBinary(cfg interface{}) browsers.DoctorBinary {
	return browsers.ResolveDoctorBinary(cfg, BinaryNames(), CommonPaths(runtime.GOOS))
}

func chromeAbsent(probed []string) browsers.DoctorCheckResult {
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorFail,
		Detail: "no chrome/chromium found. Install Chrome/Chromium (" +
			browsers.InstallHint(runtime.GOOS) + ") or set " +
			"browser.binary to an existing build. Probed: " + strings.Join(probed, ", "),
	}
}

func cdpReachableCheck(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
	binary := resolveDoctorBinary(cfg)
	if binary.Path == "" {
		return chromeAbsent(binary.Probed)
	}
	res, err := probeCDP(ctx, binary.Path, browsers.DoctorSandboxArgs(cfg), chromeDoctorLaunchTimeout)
	if err != nil {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorFail,
			Detail: fmt.Sprintf("%s: headless launch did not expose CDP: %v", binary.Path, err),
			Err:    err,
		}
	}
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorPass,
		Detail: fmt.Sprintf("%s: /json/version OK on port %d", binary.Path, res.Port),
	}
}

func chromePresenceCheck(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
	binary := resolveDoctorBinary(cfg)
	overridden := binary.Overridden
	found := binary.Path
	if found == "" {
		return chromeAbsent(binary.Probed)
	}
	line, err := browserprobe.RunVersion(ctx, found)
	if err != nil {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("%s: --version failed: %v", found, err),
			Err:    err,
		}
	}
	token := browserprobe.ExtractVersionToken(line)
	if token == "" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("%s: could not parse version from %q", found, line),
		}
	}
	if browserprobe.CompareSemver(token, chromeMinVersion) < 0 {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("%s -> %s (< required %s)", found, token, chromeMinVersion),
		}
	}
	if !overridden && runtime.GOOS == "darwin" && found == primaryChromeAppMacOS {
		// Works, but flag the macOS collision (issue #583) without downgrading
		// status — the browser launches fine, it's just sub-optimal here.
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorPass,
			Detail: fmt.Sprintf("%s -> %s (>= %s); note: this is your primary Google Chrome — "+
				"automating it on macOS can stop your normal Chrome from opening (issue #583). "+
				"Install Google Chrome for Testing or Chromium, or set browser.binary.",
				found, token, chromeMinVersion),
		}
	}
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorPass,
		Detail: fmt.Sprintf("%s -> %s (>= %s)", found, token, chromeMinVersion),
	}
}
