package doctor

import (
	"context"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

type ProbeResult = chrome.CDPProbeResult

func LaunchAndProbe(ctx context.Context, binary string, extraArgs []string, timeout time.Duration) (ProbeResult, error) {
	return chrome.LaunchAndProbe(ctx, binary, extraArgs, timeout)
}
