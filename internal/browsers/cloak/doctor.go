package cloak

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

const cloakMinVersion = "120.0.0"

// DoctorChecks overrides the inherited Chrome DoctorChecks method.
func (Browser) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck {
	return []browsers.DoctorCheck{
		{
			ID:          "cloakbrowser_present",
			Description: "CloakBrowser binary found and version adequate",
			Fn:          cloakPresenceCheck,
		},
		{
			ID:          "cdp_reachable",
			Description: "CloakBrowser accepts CDP attach headless",
			Fn:          cdpReachableCheck,
		},
		{
			ID:          "fingerprint_flags_accepted",
			Description: "CloakBrowser accepts configured fingerprint flags",
			Fn:          fingerprintFlagsCheck,
		},
		{
			ID:          "linux_fonts_present",
			Description: "Windows fingerprint fonts available on Linux host",
			Fn:          linuxFontsCheck,
		},
		{
			ID:          "handle_decisions",
			Description: "Verify CloakBrowser handles all request shapes",
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

func cloakPresenceCheck(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
	if runtime.GOOS == "windows" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorSkip,
			Detail: "cloakbrowser discovery not implemented on windows",
		}
	}
	// An explicit browser.binary override is what the runtime launches, so resolve
	// against it before falling back to discovery. The CloakBrowser installer drops
	// its Chromium in a versioned dir that isn't on the static probe list, so without
	// this a perfectly working configured binary reports a false FAIL while the
	// binary_* checks below it pass.
	override := ""
	if env, ok := cfg.(*browsers.DoctorEnv); ok && env != nil {
		override = strings.TrimSpace(env.Binary)
	}
	found := override
	discovered := false
	if found == "" {
		d := browserprobe.DiscoverBinary(BinaryNames(), CommonPaths(runtime.GOOS))
		found = d.Found
		if found == "" {
			return browsers.DoctorCheckResult{
				Status: browsers.DoctorFail,
				Detail: "cloakbrowser not found; set browser.binary or install CloakBrowser. probed: " + strings.Join(d.Probed, ", "),
			}
		}
		discovered = true
	}
	line, err := browserprobe.RunVersion(ctx, found)
	if err != nil {
		if override != "" {
			return browsers.DoctorCheckResult{
				Status: browsers.DoctorWarn,
				Detail: fmt.Sprintf("configured browser.binary %q could not be executed: %v", found, err),
				Err:    err,
			}
		}
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
	if browserprobe.CompareSemver(token, cloakMinVersion) < 0 {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("%s -> %s (< required %s)", found, token, cloakMinVersion),
		}
	}
	if discovered {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("CloakBrowser found at %s -> %s, but browser.binary is unset", found, token),
		}
	}
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorPass,
		Detail: fmt.Sprintf("%s -> %s (>= %s)", found, token, cloakMinVersion),
	}
}

func cdpReachableCheck(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
	env, ok := cfg.(*browsers.DoctorEnv)
	if !ok || env == nil {
		return browsers.DoctorCheckResult{Status: browsers.DoctorSkip, Detail: "no runtime config"}
	}
	bin := strings.TrimSpace(env.Binary)
	if bin == "" {
		return browsers.DoctorCheckResult{Status: browsers.DoctorSkip, Detail: "skipped — no browser.binary set (see cloakbrowser_present)"}
	}
	res, err := chrome.LaunchAndProbe(ctx, bin, nil, 10*time.Second)
	if err != nil {
		return browsers.DoctorCheckResult{Status: browsers.DoctorFail, Detail: err.Error(), Err: err}
	}
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorPass,
		Detail: fmt.Sprintf("/json/version OK on port %d", res.Port),
	}
}

func fingerprintFlagsCheck(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
	env, ok := cfg.(*browsers.DoctorEnv)
	if !ok || env == nil {
		return browsers.DoctorCheckResult{Status: browsers.DoctorSkip, Detail: "no runtime config"}
	}
	bin := strings.TrimSpace(env.Binary)
	if bin == "" {
		return browsers.DoctorCheckResult{Status: browsers.DoctorSkip, Detail: "skipped — no browser.binary set (see cloakbrowser_present)"}
	}

	launchCfg := browsers.LaunchConfig{
		Cloak: env.Cloak,
	}
	allArgs, _, err := Browser{}.BuildLaunchArgs(launchCfg)
	if err != nil {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorFail,
			Detail: fmt.Sprintf("building fingerprint flags failed: %v", err),
			Err:    err,
		}
	}
	var fpFlags []string
	for _, a := range allArgs {
		if strings.HasPrefix(a, "--fingerprint") {
			fpFlags = append(fpFlags, a)
		}
	}
	if len(fpFlags) == 0 {
		return browsers.DoctorCheckResult{Status: browsers.DoctorSkip, Detail: "no cloak fingerprint flags configured"}
	}
	res, err := chrome.LaunchAndProbe(ctx, bin, fpFlags, 10*time.Second)
	if err != nil {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorFail,
			Detail: fmt.Sprintf("flags rejected or browser crashed: %v", err),
			Err:    err,
		}
	}
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorPass,
		Detail: fmt.Sprintf("flags accepted, /json/version OK on port %d", res.Port),
	}
}

func linuxFontsCheck(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
	if runtime.GOOS != "linux" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorSkip,
			Detail: fmt.Sprintf("not applicable on %s (only enforced on linux hosts)", runtime.GOOS),
		}
	}
	env, ok := cfg.(*browsers.DoctorEnv)
	if !ok || env == nil || !isWindowsPlatform(env.Cloak.Platform) {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorSkip,
			Detail: "skipped (cloak.platform != windows)",
		}
	}

	probe := ProbeWindowsFingerprintFonts(ctx)
	if len(probe.Matched) == 0 {
		detail := "no windows fingerprint fonts found via filesystem scan (install msttcorefonts or mount a Windows fonts dir)"
		if probe.Source == "fc-list" {
			detail = fmt.Sprintf("%s found no windows fingerprint fonts (expected one of: %s)", probe.Source, strings.Join(probe.Expected, ", "))
		}
		return browsers.DoctorCheckResult{Status: browsers.DoctorWarn, Detail: detail}
	}
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorPass,
		Detail: fmt.Sprintf("%s: %s", probe.Source, strings.Join(probe.Matched, ", ")),
	}
}

func isWindowsPlatform(p string) bool {
	p = strings.ToLower(strings.TrimSpace(p))
	return p == "windows" || strings.HasPrefix(p, "win")
}
