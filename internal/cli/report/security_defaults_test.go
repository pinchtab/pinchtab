package report

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
)

// Every key security up writes is either summarised by a posture row or resolves to
// none, in which case the change report says so; the keys are read off the preset
// itself and the mapping off the rows, so neither side can drift silently.
func TestEveryKeySecurityUpWritesResolvesToAPostureRowOrToNone(t *testing.T) {
	paths, err := workflow.RecommendedSecuritySettingPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 5 {
		t.Fatalf("security up names only %d settings; too few to prove the mapping", len(paths))
	}
	claimed := 0
	for _, path := range paths {
		if len(PostureRowsForSetting(path)) > 0 {
			claimed++
		}
	}
	if claimed == 0 {
		t.Fatal("no recommended setting resolves to a posture row; the mapping is empty")
	}
	for _, check := range AssessSecurityPosture(config.Load()).Checks {
		if len(check.Settings) == 0 {
			t.Errorf("posture row %q names no settings, so no change can be attributed to it", check.Label)
		}
	}
	for _, path := range []string{"security.idpi.wrapContent", "security.idpi.scanContent", "security.idpi.enabled"} {
		if len(PostureRowsForSetting(path)) == 0 {
			t.Errorf("%s feeds the IDPI rows yet resolves to none", path)
		}
	}
	if rows := PostureRowsForSetting("security.idpi.scanTimeoutSec"); len(rows) != 0 {
		t.Errorf("scanTimeoutSec is not summarised by any row but resolves to %v", rows)
	}
}
