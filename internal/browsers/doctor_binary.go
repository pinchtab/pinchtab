package browsers

import (
	"context"
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
)

// DoctorBinary is the browser a doctor check will exercise: the configured
// override when set, otherwise what discovery finds. Every check resolves the
// same way, so none of them can skip on a default install that discovery serves.
type DoctorBinary struct {
	Path       string
	Overridden bool
	Probed     []string
}

func ResolveDoctorBinary(env *DoctorEnv, names, paths []string) DoctorBinary {
	if env != nil {
		if override := strings.TrimSpace(env.Binary); override != "" {
			return DoctorBinary{Path: override, Overridden: true}
		}
	}
	d := browserprobe.DiscoverBinary(names, paths)
	return DoctorBinary{Path: d.Found, Probed: d.Probed}
}

func DoctorSandboxArgs(env *DoctorEnv) []string {
	if env != nil && env.NoSandbox {
		return []string{"--no-sandbox"}
	}
	return nil
}

func VersionCheck(ctx context.Context, bin DoctorBinary, minVersion string, identify func(token string) *DoctorCheckResult) (string, DoctorCheckResult) {
	line, err := browserprobe.RunVersion(ctx, bin.Path)
	if err != nil {
		detail := fmt.Sprintf("%s: --version failed: %v", bin.Path, err)
		if bin.Overridden {
			detail = fmt.Sprintf("configured browser.binary %q could not be executed: %v", bin.Path, err)
		}
		return "", DoctorCheckResult{Status: DoctorWarn, Detail: detail, Err: err}
	}
	token := browserprobe.ExtractVersionToken(line)
	if token == "" {
		return "", DoctorCheckResult{Status: DoctorWarn, Detail: fmt.Sprintf("%s: could not parse version from %q", bin.Path, line)}
	}
	if identify != nil {
		if failed := identify(token); failed != nil {
			return token, *failed
		}
	}
	if browserprobe.CompareSemver(token, minVersion) < 0 {
		return token, DoctorCheckResult{Status: DoctorWarn, Detail: fmt.Sprintf("%s -> %s (< required %s)", bin.Path, token, minVersion)}
	}
	return token, DoctorCheckResult{Status: DoctorPass, Detail: fmt.Sprintf("%s -> %s (>= %s)", bin.Path, token, minVersion)}
}

func CDPReachable(bin DoctorBinary, onAbsent func(probed []string) DoctorCheckResult, probe func(path string) (port int, err error)) DoctorCheckResult {
	if bin.Path == "" {
		return onAbsent(bin.Probed)
	}
	port, err := probe(bin.Path)
	if err != nil {
		return DoctorCheckResult{
			Status: DoctorFail,
			Detail: fmt.Sprintf("%s: headless launch did not expose CDP: %v", bin.Path, err),
			Err:    err,
		}
	}
	return DoctorCheckResult{Status: DoctorPass, Detail: fmt.Sprintf("%s: /json/version OK on port %d", bin.Path, port)}
}

func HandleDecisionsCheck(b Browser, label string) DoctorCheck {
	return DoctorCheck{
		ID:          "handle_decisions",
		Description: fmt.Sprintf("Verify %s handles all request shapes", label),
		Fn: func(context.Context, *DoctorEnv) DoctorCheckResult {
			var unexpected []string
			for _, shape := range AllShapes {
				if b.CanHandle(RequestIntent{Shape: shape}).Decision != DecisionHandle {
					unexpected = append(unexpected, shape)
				}
			}
			if len(unexpected) > 0 {
				return DoctorCheckResult{
					Status: DoctorWarn,
					Detail: "unexpected skip/fail for shapes: " + strings.Join(unexpected, ", "),
				}
			}
			return DoctorCheckResult{Status: DoctorPass, Detail: fmt.Sprintf("all %d request shapes handled", len(AllShapes))}
		},
	}
}
