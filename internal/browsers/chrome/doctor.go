package chrome

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/browsers"
)

const chromeMinVersion = "120.0.0"

func (b Browser) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck {
	return []browsers.DoctorCheck{
		{
			ID:          "chrome_present",
			Description: "Chrome/Chromium binary found and version adequate",
			Fn:          chromePresenceCheck,
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

func chromePresenceCheck(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
	// A configured browser.binary override governs the runtime, so resolve against
	// it first. Honoring it here keeps chrome_present consistent with the binary_*
	// checks (which already validate the override) — otherwise a working configured
	// browser sitting off the static discovery path produces a false FAIL.
	override := ""
	if env, ok := cfg.(*browsers.DoctorEnv); ok && env != nil {
		override = strings.TrimSpace(env.Binary)
	}
	overridden := override != ""

	if runtime.GOOS == "windows" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorSkip,
			Detail: "chrome discovery not implemented on windows",
		}
	}
	found := override
	if found == "" {
		d := browserprobe.DiscoverBinary(BinaryNames(), CommonPaths(runtime.GOOS))
		found = d.Found
		if found == "" {
			return browsers.DoctorCheckResult{
				Status: browsers.DoctorFail,
				Detail: "no chrome/chromium found. Install Chrome/Chromium (e.g. Google Chrome " +
					"for Testing, or `apt-get install -y chromium` on Debian/Ubuntu) or set " +
					"browser.binary to an existing build. Probed: " + strings.Join(d.Probed, ", "),
			}
		}
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
