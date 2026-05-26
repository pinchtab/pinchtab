package doctor

import (
	"context"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

// ProbeResult holds the outcome of a successful LaunchAndProbe call.
type ProbeResult = chrome.CDPProbeResult

// LaunchAndProbe starts binary headless, waits for the DevTools banner,
// confirms /json/version responds, and tears the browser down before return.
// It delegates to chrome.LaunchAndProbe.
func LaunchAndProbe(ctx context.Context, binary string, extraArgs []string, timeout time.Duration) (ProbeResult, error) {
	return chrome.LaunchAndProbe(ctx, binary, extraArgs, timeout)
}
