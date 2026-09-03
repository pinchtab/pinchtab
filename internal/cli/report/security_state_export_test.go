package report

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func sensitiveEndpointsCheck(t *testing.T, cfg *config.RuntimeConfig) SecurityPostureCheck {
	t.Helper()
	for _, check := range AssessSecurityPosture(cfg).Checks {
		if check.ID == "sensitive_endpoints_disabled" {
			return check
		}
	}
	t.Fatal("no sensitive_endpoints_disabled check in the posture")
	return SecurityPostureCheck{}
}

// Two servers identical but for one flag: allowStateExport alone used to pass the
// sensitive-endpoints check and print "disabled" while ten state routes were live.
func TestAllowStateExportAloneFailsTheSensitiveEndpointsCheckLikeAllowCookiesDoes(t *testing.T) {
	cookies := sensitiveEndpointsCheck(t, &config.RuntimeConfig{AllowCookies: true})
	stateExport := sensitiveEndpointsCheck(t, &config.RuntimeConfig{AllowStateExport: true})
	if cookies.Passed || cookies.Detail != "cookies" {
		t.Errorf("allowCookies alone: passed=%v detail=%q", cookies.Passed, cookies.Detail)
	}
	if stateExport.Passed || stateExport.Detail != "stateExport" {
		t.Errorf("allowStateExport alone: passed=%v detail=%q, want the check failed naming stateExport", stateExport.Passed, stateExport.Detail)
	}
	if quiet := sensitiveEndpointsCheck(t, &config.RuntimeConfig{}); !quiet.Passed || quiet.Detail != "disabled" {
		t.Errorf("nothing enabled: passed=%v detail=%q", quiet.Passed, quiet.Detail)
	}
}
