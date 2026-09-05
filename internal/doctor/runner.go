// Package doctor implements the `pinchtab doctor` diagnostic command.
package doctor

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/runtimekit"
	"github.com/pinchtab/pinchtab/internal/config"
)

type CheckStatus string

const (
	StatusPass CheckStatus = "pass"
	StatusFail CheckStatus = "fail"
	StatusWarn CheckStatus = "warn"
	StatusSkip CheckStatus = "skip"
)

type CheckResult struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail"`
	Err    error       `json:"-"`
	ErrMsg string      `json:"error,omitempty"`
	// Duration is for human-facing output; DurationMS is its JSON shape —
	// a raw time.Duration would marshal nanoseconds under a "Ms" key.
	Duration   time.Duration `json:"-"`
	DurationMS int64         `json:"durationMs"`
	// LaunchesBrowser marks a check that starts a browsing session; the summary
	// counts how many of those actually ran, which is what separates a clean run
	// from one whose passes never left the binary.
	LaunchesBrowser bool `json:"launchesBrowser"`
}

type CheckFunc func(ctx context.Context, cfg *config.RuntimeConfig) CheckResult

type CheckEntry struct {
	Name            string
	Fn              CheckFunc
	LaunchesBrowser bool
}

type Summary struct {
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	Warnings        int `json:"warnings"`
	Skipped         int `json:"skipped"`
	BrowserLaunched int `json:"browserLaunched"`
}

// Verdict is the sentence the counts alone could not say: passes that never
// launched a browser are not a clean bill of health.
func Verdict(s Summary) string {
	switch {
	case s.Failed > 0:
		return fmt.Sprintf("%d check(s) failed.", s.Failed)
	case s.BrowserLaunched == 0:
		return "No check launched a browser: the passes above only inspected the installation, so this is not a clean bill of health."
	default:
		return fmt.Sprintf("Clean: %d check(s) launched a browser and passed.", s.BrowserLaunched)
	}
}

func Summarize(results []CheckResult) Summary {
	var s Summary
	for _, r := range results {
		if r.LaunchesBrowser && r.Status != StatusSkip {
			s.BrowserLaunched++
		}
		switch r.Status {
		case StatusPass:
			s.Passed++
		case StatusFail:
			s.Failed++
		case StatusWarn:
			s.Warnings++
		case StatusSkip:
			s.Skipped++
		}
	}
	return s
}

// ExitCode returns 1 when any check failed; skipped/warn do not fail the run.
func ExitCode(s Summary) int {
	if s.Failed > 0 {
		return 1
	}
	return 0
}

// Registry returns the ordered list of checks that apply to cfg. Inapplicable
// checks are omitted entirely so `--check=<name>` reports "unknown check"
// rather than silently skipping.
func Registry(cfg *config.RuntimeConfig) []CheckEntry {
	entries := []CheckEntry{
		{Name: "config_file", Fn: checkConfigFile},
	}

	browserID := config.NormalizeBrowser(browserFromCfg(cfg))
	if b, ok := browsers.Get(browserID); ok {
		env := doctorEnvForBrowser(cfg, browserID)
		for _, dc := range b.DoctorChecks(browsers.TargetConfig{Provider: browserID}) {
			dc := dc
			entries = append(entries, CheckEntry{
				Name:            dc.ID,
				LaunchesBrowser: dc.LaunchesBrowser,
				Fn: func(ctx context.Context, _ *config.RuntimeConfig) CheckResult {
					return browserCheckResult(ctx, dc, env)
				},
			})
		}
	}

	// Keep non-provider-specific checks after provider ones.
	entries = append(entries,
		CheckEntry{Name: "binary_exists", Fn: checkBinaryExists},
		CheckEntry{Name: "binary_executable", Fn: checkBinaryExecutable},
		CheckEntry{Name: "binary_starts", Fn: checkBinaryStarts},
	)

	return entries
}

func browserFromCfg(cfg *config.RuntimeConfig) string {
	return defaultBrowserForDoctor(cfg)
}

// defaultBrowserForDoctor mirrors launch-time resolution: when targets are
// configured, the default target's provider is what actually launches, so the
// doctor must check/flag that — not the raw browsers.default (which the config
// store keeps verbatim). Falls back to cfg.DefaultBrowser when no targets are
// configured or the default target can't be resolved.
func defaultBrowserForDoctor(cfg *config.RuntimeConfig) string {
	if cfg == nil {
		return ""
	}
	if len(cfg.Targets) > 0 {
		if resolved, err := config.ResolveDefaultBrowserTarget(cfg); err == nil && resolved != nil && resolved.Provider != "" {
			return config.NormalizeBrowser(resolved.Provider)
		}
	}
	return cfg.DefaultBrowser
}

func mapDoctorStatus(s browsers.DoctorStatus) CheckStatus {
	switch s {
	case browsers.DoctorPass:
		return StatusPass
	case browsers.DoctorFail:
		return StatusFail
	case browsers.DoctorWarn:
		return StatusWarn
	case browsers.DoctorSkip:
		return StatusSkip
	default:
		return StatusSkip
	}
}

// browserCheckResult runs one provider DoctorCheck against env and maps it to a
// CheckResult, shared by the registry (`doctor --check`) and ReportBrowsers
// (`doctor browser`) paths so check translation can't drift between them.
func browserCheckResult(ctx context.Context, dc browsers.DoctorCheck, env *browsers.DoctorEnv) CheckResult {
	r := dc.Fn(ctx, env)
	return CheckResult{
		Name:   dc.ID,
		Status: mapDoctorStatus(r.Status),
		Detail: r.Detail,
		Err:    r.Err,
		ErrMsg: errMsg(r.Err),
	}
}

// Run executes the diagnostic pipeline; when checkFilter is non-empty only
// the named check runs.
func Run(ctx context.Context, cfg *config.RuntimeConfig, checkFilter string) []CheckResult {
	return run(ctx, cfg, checkFilter, nil)
}

// RunWithConfigError produces a useful diagnostic report when configuration
// validation failed. The config check carries the fatal error; checks whose
// inputs cannot be trusted are retained as explicit skips.
func RunWithConfigError(ctx context.Context, cfg *config.RuntimeConfig, checkFilter string, configErr error) []CheckResult {
	return run(ctx, cfg, checkFilter, configErr)
}

func run(ctx context.Context, cfg *config.RuntimeConfig, checkFilter string, configErr error) []CheckResult {
	entries := Registry(cfg)
	checkFilter = strings.TrimSpace(checkFilter)

	out := make([]CheckResult, 0, len(entries))
	for _, e := range entries {
		if checkFilter != "" && e.Name != checkFilter && (configErr == nil || e.Name != "config_file") {
			continue
		}
		start := time.Now()
		var r CheckResult
		switch {
		case configErr == nil:
			r = e.Fn(ctx, cfg)
		case e.Name == "config_file":
			r = CheckResult{Status: StatusFail, Detail: "configuration could not be loaded: " + configErr.Error(), Err: configErr}
		default:
			r = CheckResult{Status: StatusSkip, Detail: "requires a valid loaded configuration"}
		}
		r.Name = e.Name
		r.LaunchesBrowser = e.LaunchesBrowser
		if r.Duration == 0 {
			r.Duration = time.Since(start)
		}
		r.DurationMS = r.Duration.Milliseconds()
		if r.Err != nil && r.ErrMsg == "" {
			r.ErrMsg = r.Err.Error()
		}
		out = append(out, r)
	}
	return out
}

func KnownCheck(cfg *config.RuntimeConfig, name string) bool {
	for _, e := range Registry(cfg) {
		if e.Name == name {
			return true
		}
	}
	return false
}

// buildDoctorEnv constructs a browsers.DoctorEnv from a RuntimeConfig,
// giving browser doctor checks access to the fields they need without
// requiring browser sub-packages to import the config package.
func buildDoctorEnv(cfg *config.RuntimeConfig) *browsers.DoctorEnv {
	if cfg == nil {
		return nil
	}
	_, containerErr := os.Stat("/.dockerenv")
	return &browsers.DoctorEnv{
		Binary:    strings.TrimSpace(cfg.BrowserBinary),
		NoSandbox: runtimekit.ChromeNeedsNoSandbox(runtime.GOOS, os.Geteuid(), containerErr == nil),
		Cloak: browsers.CloakFingerprint{
			FingerprintSeed: cfg.Cloak.FingerprintSeed,
			Platform:        cfg.Cloak.Platform,
			Locale:          cfg.Cloak.Locale,
			Timezone:        cfg.Cloak.Timezone,
			WebRTCIP:        cfg.Cloak.WebRTCIP,
			FontsDir:        cfg.Cloak.FontsDir,
			StorageQuotaMB:  cfg.Cloak.StorageQuotaMB,
		},
	}
}
