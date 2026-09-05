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
const cloakIdentityPlatform = "windows"
const cloakDoctorLaunchTimeout = 60 * time.Second

var launchAndEvaluate = chrome.LaunchAndEvaluate
var launchAndProbe = chrome.LaunchAndProbe

// DoctorChecks overrides the inherited Chrome DoctorChecks method.
func (Browser) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck {
	return []browsers.DoctorCheck{
		{
			ID:          "cloakbrowser_present",
			Description: "CloakBrowser binary found and version adequate",
			Fn:          cloakPresenceCheck,
		},
		{
			ID:              "cdp_reachable",
			Description:     "CloakBrowser accepts CDP attach headless",
			Fn:              cdpReachableCheck,
			LaunchesBrowser: true,
		},
		{
			ID:              "fingerprint_flags_accepted",
			Description:     "CloakBrowser accepts configured fingerprint flags",
			Fn:              fingerprintFlagsCheck,
			LaunchesBrowser: true,
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

func resolveDoctorBinary(cfg interface{}) browsers.DoctorBinary {
	return browsers.ResolveDoctorBinary(cfg, BinaryNames(), CommonPaths(runtime.GOOS))
}

func cloakPresenceCheck(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
	binary := resolveDoctorBinary(cfg)
	override := ""
	if binary.Overridden {
		override = binary.Path
	}
	found := binary.Path
	discovered := !binary.Overridden
	if found == "" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorFail,
			Detail: "cloakbrowser not found; set browser.binary or install CloakBrowser. probed: " + strings.Join(binary.Probed, ", "),
		}
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
	var platform string
	identityArgs := []string{"--fingerprint-platform=" + cloakIdentityPlatform}
	if env, ok := cfg.(*browsers.DoctorEnv); ok && env != nil && env.NoSandbox {
		identityArgs = append(identityArgs, "--no-sandbox")
	}
	_, err = launchAndEvaluate(ctx, found, identityArgs, cloakDoctorLaunchTimeout, "navigator.platform", &platform)
	if err != nil || platform != "Win32" {
		reason := fmt.Sprintf("fingerprint platform probe returned %q, want Win32", platform)
		if err != nil {
			reason = err.Error()
		}
		prefix := fmt.Sprintf("binary %q", found)
		if override != "" {
			prefix = fmt.Sprintf("configured browser.binary %q", found)
		}
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("%s reports version %s but did not exhibit CloakBrowser fingerprint behavior: %s", prefix, token, reason),
			Err:    err,
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

const noCloakFound = "skipped — no CloakBrowser found by browser.binary or discovery (see cloakbrowser_present)"

func cdpReachableCheck(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
	bin := resolveDoctorBinary(cfg).Path
	if bin == "" {
		return browsers.DoctorCheckResult{Status: browsers.DoctorSkip, Detail: noCloakFound}
	}
	res, err := launchAndProbe(ctx, bin, browsers.DoctorSandboxArgs(cfg), cloakDoctorLaunchTimeout)
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
	bin := resolveDoctorBinary(cfg).Path
	if bin == "" {
		return browsers.DoctorCheckResult{Status: browsers.DoctorSkip, Detail: noCloakFound}
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
	fpFlags = append(fpFlags, browsers.DoctorSandboxArgs(cfg)...)
	res, err := launchAndProbe(ctx, bin, fpFlags, cloakDoctorLaunchTimeout)
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
