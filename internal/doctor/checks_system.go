package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/config"
)

// chromeMinVersion is the lowest Chrome we accept: 120 (Dec 2023) ships
// headless=new and the CDP/BiDi surfaces PinchTab drives.
const chromeMinVersion = "120.0.0"

// cloakMinVersion matches the documented CloakBrowser floor tested against.
const cloakMinVersion = "100.0.0"

func runVersion(ctx context.Context, bin string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin, "--version").CombinedOutput()
	if err != nil {
		return "", err
	}
	line := parseVersionLine(string(out))
	if line == "" {
		line = strings.TrimSpace(string(out))
	}
	return line, nil
}

func checkConfigFile(_ context.Context, _ *config.RuntimeConfig) CheckResult {
	status := config.InspectConfigFile()
	if !status.Found {
		detail := fmt.Sprintf("not found at %s", status.Path)
		if status.EnvOverride {
			detail += " (PINCHTAB_CONFIG override; default would be " + status.DefaultPath + ")"
		} else {
			detail += " (default search path; set PINCHTAB_CONFIG to override)"
		}
		return CheckResult{Status: StatusWarn, Detail: detail}
	}
	if status.ParseErr != nil {
		return CheckResult{
			Status: StatusFail,
			Detail: fmt.Sprintf("%s: parse error: %v", status.Path, status.ParseErr),
			Err:    status.ParseErr,
		}
	}
	return CheckResult{
		Status: StatusPass,
		Detail: status.Path + " (loaded)",
	}
}

// checkChromePresent answers "is any Chrome on this machine?" independent
// of cfg.ChromeBinary (which the binary_* checks already cover).
func checkChromePresent(ctx context.Context, _ *config.RuntimeConfig) CheckResult {
	if HostOS() == "windows" {
		// TODO(windows): implement Windows install-path discovery.
		return CheckResult{
			Status: StatusSkip,
			Detail: "chrome discovery not implemented on windows",
		}
	}
	discovery := browserprobe.DiscoverChromeBinary(HostOS())
	if discovery.Found == "" {
		return CheckResult{
			Status: StatusFail,
			Detail: "no chrome/chromium found; probed: " + strings.Join(discovery.Probed, ", "),
		}
	}
	line, err := runVersion(ctx, discovery.Found)
	if err != nil {
		return CheckResult{
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s: --version failed: %v", discovery.Found, err),
			Err:    err,
		}
	}
	token := extractVersionToken(line)
	if token == "" {
		return CheckResult{
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s: could not parse version from %q", discovery.Found, line),
		}
	}
	if compareSemver(token, chromeMinVersion) < 0 {
		return CheckResult{
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s -> %s (< required %s)", discovery.Found, token, chromeMinVersion),
		}
	}
	return CheckResult{
		Status: StatusPass,
		Detail: fmt.Sprintf("%s -> %s (>= %s)", discovery.Found, token, chromeMinVersion),
	}
}

// checkCloakBrowserPresent skips (does not fail) when CloakBrowser is absent
// and no cloak provider is configured.
func checkCloakBrowserPresent(ctx context.Context, cfg *config.RuntimeConfig) CheckResult {
	if HostOS() == "windows" {
		return CheckResult{
			Status: StatusSkip,
			Detail: "cloakbrowser discovery not implemented on windows",
		}
	}
	cloakConfigured := isCloakConfigured(cfg)

	discovery := browserprobe.DiscoverCloakBrowserBinary(HostOS())
	if discovery.Found == "" {
		if cloakConfigured {
			return CheckResult{
				Status: StatusFail,
				Detail: "cloakbrowser not found but a cloak provider is configured; set browser.binary or install CloakBrowser. probed: " + strings.Join(discovery.Probed, ", "),
			}
		}
		return CheckResult{
			Status: StatusSkip,
			Detail: "not found (skipped — no cloak provider configured)",
		}
	}
	line, err := runVersion(ctx, discovery.Found)
	if err != nil {
		return CheckResult{
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s: --version failed: %v", discovery.Found, err),
			Err:    err,
		}
	}
	token := extractVersionToken(line)
	if token == "" {
		return CheckResult{
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s: could not parse version from %q", discovery.Found, line),
		}
	}
	if compareSemver(token, cloakMinVersion) < 0 {
		return CheckResult{
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s -> %s (< required %s)", discovery.Found, token, cloakMinVersion),
		}
	}
	return CheckResult{
		Status: StatusPass,
		Detail: fmt.Sprintf("%s -> %s (>= %s)", discovery.Found, token, cloakMinVersion),
	}
}

func isCloakConfigured(cfg *config.RuntimeConfig) bool {
	if cfg == nil {
		return false
	}
	if config.IsCloakBrowserProvider(cfg.BrowserProvider) {
		return true
	}
	for _, t := range cfg.Targets {
		if config.IsCloakBrowserProvider(t.Provider) {
			return true
		}
	}
	return false
}
