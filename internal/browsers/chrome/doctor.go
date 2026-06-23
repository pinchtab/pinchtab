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

func chromePresenceCheck(ctx context.Context, _ interface{}) browsers.DoctorCheckResult {
	if runtime.GOOS == "windows" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorSkip,
			Detail: "chrome discovery not implemented on windows",
		}
	}
	d := browserprobe.DiscoverBinary(BinaryNames(), CommonPaths(runtime.GOOS))
	if d.Found == "" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorFail,
			Detail: "no chrome/chromium found; probed: " + strings.Join(d.Probed, ", "),
		}
	}
	line, err := browserprobe.RunVersion(ctx, d.Found)
	if err != nil {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("%s: --version failed: %v", d.Found, err),
			Err:    err,
		}
	}
	token := browserprobe.ExtractVersionToken(line)
	if token == "" {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("%s: could not parse version from %q", d.Found, line),
		}
	}
	if browserprobe.CompareSemver(token, chromeMinVersion) < 0 {
		return browsers.DoctorCheckResult{
			Status: browsers.DoctorWarn,
			Detail: fmt.Sprintf("%s -> %s (< required %s)", d.Found, token, chromeMinVersion),
		}
	}
	return browsers.DoctorCheckResult{
		Status: browsers.DoctorPass,
		Detail: fmt.Sprintf("%s -> %s (>= %s)", d.Found, token, chromeMinVersion),
	}
}
