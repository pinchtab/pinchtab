// Package doctor implements the `pinchtab doctor` diagnostic command.
package doctor

import (
	"context"
	"runtime"
	"strings"
	"time"

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
	Name     string        `json:"name"`
	Status   CheckStatus   `json:"status"`
	Detail   string        `json:"detail"`
	Err      error         `json:"-"`
	ErrMsg   string        `json:"error,omitempty"`
	Duration time.Duration `json:"durationMs"`
}

type CheckFunc func(ctx context.Context, cfg *config.RuntimeConfig) CheckResult

type CheckEntry struct {
	Name string
	Fn   CheckFunc
}

type Summary struct {
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Warnings int `json:"warnings"`
	Skipped  int `json:"skipped"`
}

func Summarize(results []CheckResult) Summary {
	var s Summary
	for _, r := range results {
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
		{Name: "chrome_present", Fn: checkChromePresent},
		{Name: "cloakbrowser_present", Fn: checkCloakBrowserPresent},
		{Name: "binary_exists", Fn: checkBinaryExists},
		{Name: "binary_executable", Fn: checkBinaryExecutable},
		{Name: "binary_starts", Fn: checkBinaryStarts},
	}
	if cfg != nil && config.IsCloakBrowserProvider(cfg.BrowserProvider) {
		entries = append(entries,
			CheckEntry{Name: "cdp_reachable", Fn: checkCDPReachable},
			CheckEntry{Name: "fingerprint_flags_accepted", Fn: checkFingerprintFlagsAccepted},
			CheckEntry{Name: "linux_fonts_present", Fn: checkLinuxFontsPresent},
		)
	}
	return entries
}

// Run executes the diagnostic pipeline; when checkFilter is non-empty only
// the named check runs.
func Run(ctx context.Context, cfg *config.RuntimeConfig, checkFilter string) []CheckResult {
	entries := Registry(cfg)
	checkFilter = strings.TrimSpace(checkFilter)

	out := make([]CheckResult, 0, len(entries))
	for _, e := range entries {
		if checkFilter != "" && e.Name != checkFilter {
			continue
		}
		start := time.Now()
		r := e.Fn(ctx, cfg)
		r.Name = e.Name
		if r.Duration == 0 {
			r.Duration = time.Since(start)
		}
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

// HostOS wraps runtime.GOOS to permit test override.
var HostOS = func() string { return runtime.GOOS }
