package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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
	if mode&0o111 == 0 {
		return CheckResult{
			Status: StatusFail,
			Detail: fmt.Sprintf("file mode %#o has no executable bit", mode.Perm()),
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
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, bin, "--version") // #nosec G204 -- bin is a browser path from config/discovery, not user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return CheckResult{
			Status: StatusFail,
			Detail: fmt.Sprintf("--version failed: %v", err),
			Err:    err,
		}
	}
	version := parseVersionLine(string(out))
	if version == "" {
		version = strings.TrimSpace(string(out))
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
