package cloak

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

const cloakMinVersion = "120.0.0"
const cloakIdentityPlatform = "windows"
const cloakDoctorLaunchTimeout = 60 * time.Second

var launchAndEvaluate = chrome.LaunchAndEvaluate
var launchAndProbe = chrome.LaunchAndProbe

// DoctorChecks overrides the inherited Chrome DoctorChecks method.
func (b Browser) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck {
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
		browsers.HandleDecisionsCheck(b, "CloakBrowser"),
	}
}

func resolveDoctorBinary(env *browsers.DoctorEnv) browsers.DoctorBinary {
	return browsers.ResolveDoctorBinary(env, BinaryNames(), CommonPaths(runtime.GOOS))
}

func cloakPresenceCheck(ctx context.Context, env *browsers.DoctorEnv) browsers.DoctorCheckResult {
	binary := resolveDoctorBinary(env)
	if binary.Path == "" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorFail,
			Detail: "cloakbrowser not found; set browser.binary or install CloakBrowser. probed: " + strings.Join(binary.Probed, ", "),
		}
	}
	token, result := browsers.VersionCheck(ctx, binary, cloakMinVersion, func(token string) *browsers.DoctorCheckResult {
		return cloakIdentityCheck(ctx, env, binary, token)
	})
	if result.Status == browsers.DoctorPass && !binary.Overridden {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("CloakBrowser found at %s -> %s, but browser.binary is unset", binary.Path, token),
		}
	}
	return result
}

func cloakIdentityCheck(ctx context.Context, env *browsers.DoctorEnv, binary browsers.DoctorBinary, token string) *browsers.DoctorCheckResult {
	var platform string
	identityArgs := append([]string{"--fingerprint-platform=" + cloakIdentityPlatform}, browsers.DoctorSandboxArgs(env)...)
	_, err := launchAndEvaluate(ctx, binary.Path, identityArgs, cloakDoctorLaunchTimeout, "navigator.platform", &platform)
	if err == nil && platform == "Win32" {
		return nil
	}
	reason := fmt.Sprintf("fingerprint platform probe returned %q, want Win32", platform)
	if err != nil {
		reason = err.Error()
	}
	prefix := fmt.Sprintf("binary %q", binary.Path)
	if binary.Overridden {
		prefix = fmt.Sprintf("configured browser.binary %q", binary.Path)
	}
	return &browsers.DoctorCheckResult{
		Status: browsers.DoctorWarn,
		Detail: fmt.Sprintf("%s reports version %s but did not exhibit CloakBrowser fingerprint behavior: %s", prefix, token, reason),
		Err:    err,
	}
}

const noCloakFound = "skipped — no CloakBrowser found by browser.binary or discovery (see cloakbrowser_present)"

func cdpReachableCheck(ctx context.Context, env *browsers.DoctorEnv) browsers.DoctorCheckResult {
	cloakAbsent := func([]string) browsers.DoctorCheckResult {
		return browsers.DoctorCheckResult{Status: browsers.DoctorSkip, Detail: noCloakFound}
	}
	return browsers.CDPReachable(resolveDoctorBinary(env), cloakAbsent, func(path string) (int, error) {
		res, err := launchAndProbe(ctx, path, browsers.DoctorSandboxArgs(env), cloakDoctorLaunchTimeout)
		return res.Port, err
	})
}

func fingerprintFlagsCheck(ctx context.Context, env *browsers.DoctorEnv) browsers.DoctorCheckResult {
	if env == nil {
		return browsers.DoctorCheckResult{Status: browsers.DoctorSkip, Detail: "no runtime config"}
	}
	bin := resolveDoctorBinary(env).Path
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
	fpFlags = append(fpFlags, browsers.DoctorSandboxArgs(env)...)
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

func linuxFontsCheck(ctx context.Context, env *browsers.DoctorEnv) browsers.DoctorCheckResult {
	if runtime.GOOS != "linux" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorSkip,
			Detail: fmt.Sprintf("not applicable on %s (only enforced on linux hosts)", runtime.GOOS),
		}
	}
	if env == nil || !isWindowsPlatform(env.Cloak.Platform) {
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
