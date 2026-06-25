package doctor

import (
	"context"
	"fmt"

	"github.com/pinchtab/pinchtab/internal/config"
)

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
