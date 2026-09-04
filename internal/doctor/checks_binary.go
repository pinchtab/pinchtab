package doctor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/config"
)

func resolveBinary(cfg *config.RuntimeConfig) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.BrowserBinary)
}

func checkBinaryExists(_ context.Context, cfg *config.RuntimeConfig) CheckResult {
	bin := resolveBinary(cfg)
	if bin == "" {
		return CheckResult{
			Status: StatusSkip,
			Detail: "browser.binary not set; relying on provider discovery",
		}
	}
	info, err := os.Stat(bin)
	if err != nil {
		return CheckResult{
			Status: StatusFail,
			Detail: fmt.Sprintf("%s: %v", bin, err),
			Err:    err,
		}
	}
	if info.IsDir() {
		return CheckResult{
			Status: StatusFail,
			Detail: fmt.Sprintf("%s is a directory", bin),
		}
	}
	return CheckResult{
		Status: StatusPass,
		Detail: bin,
	}
}

func checkBinaryExecutable(_ context.Context, cfg *config.RuntimeConfig) CheckResult {
	bin := resolveBinary(cfg)
	if bin == "" {
		return CheckResult{Status: StatusSkip, Detail: "browser.binary not set; relying on provider discovery"}
	}
	info, err := os.Stat(bin)
	if err != nil {
		return CheckResult{Status: StatusFail, Detail: err.Error(), Err: err}
	}
	mode := info.Mode()
	// Executability is platform-specific: Windows has no execute bit and every file
	// stats as -rw-rw-rw-, so a mode check fails every configured browser.binary
	// there. browserprobe owns the one rule discovery also uses.
	if !browserprobe.IsExecutable(info, bin) {
		return CheckResult{
			Status: StatusFail,
			Detail: fmt.Sprintf("file mode %#o is not executable on this platform", mode.Perm()),
		}
	}
	return CheckResult{
		Status: StatusPass,
		Detail: fmt.Sprintf("file mode %#o", mode.Perm()),
	}
}

func checkBinaryStarts(ctx context.Context, cfg *config.RuntimeConfig) CheckResult {
	bin := resolveBinary(cfg)
	if bin == "" {
		return CheckResult{Status: StatusSkip, Detail: "browser.binary not set; relying on provider discovery"}
	}
	// This used to run `bin --version` itself, a second copy of a probe
	// browserprobe already owns — and the copy still had the Windows defect the
	// original was fixed for. There `--version` prints no version: the process
	// forwards its arguments to the running browser and prints "Opening in
	// existing browser session.", so `pinchtab doctor` opened a window on the
	// user's desktop and then reported that sentence as the browser's version
	// (issue #649). browserprobe.RunVersion reads the install layout on Windows
	// and starts nothing.
	version, err := browserprobe.RunVersion(ctx, bin)
	if err != nil {
		return CheckResult{
			Status: StatusFail,
			Detail: fmt.Sprintf("version probe failed: %v", err),
			Err:    err,
		}
	}
	if parsed := parseVersionLine(version); parsed != "" {
		version = parsed
	}
	// A probe that answers something with no version in it has not identified the
	// browser, whatever its exit code was. Reporting Pass on that is how the
	// "Opening in existing browser session." sentence reached users as a version.
	if extractVersionToken(version) == "" {
		return CheckResult{
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s: probe returned no version (%q)", bin, version),
		}
	}
	return CheckResult{
		Status: StatusPass,
		Detail: version,
	}
}

func parseVersionLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
