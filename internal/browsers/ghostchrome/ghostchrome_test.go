package ghostchrome

import (
	"context"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browsers"
)

func TestRegistered(t *testing.T) {
	b, ok := browsers.Get("ghost-chrome")
	if !ok {
		t.Fatal("ghost-chrome not registered")
	}
	if b.ID() != "ghost-chrome" {
		t.Errorf("ID() = %q, want %q", b.ID(), "ghost-chrome")
	}
	if b.DisplayName() != "Ghost + Chrome" {
		t.Errorf("DisplayName() = %q, want %q", b.DisplayName(), "Ghost + Chrome")
	}
}

func TestBuildLaunchArgsDelegatesToChrome(t *testing.T) {
	b := &Browser{}
	args, _, err := b.BuildLaunchArgs(browsers.LaunchConfig{})
	if err != nil {
		t.Fatalf("BuildLaunchArgs returned error: %v", err)
	}
	if len(args) == 0 {
		t.Fatal("BuildLaunchArgs returned empty args, expected Chrome flags")
	}
}

func TestSupportsRemoteCDP(t *testing.T) {
	b := &Browser{}
	if !b.SupportsRemoteCDP() {
		t.Error("SupportsRemoteCDP() = false, want true")
	}
}

func TestGhostChrome_CanHandle(t *testing.T) {
	b := &Browser{}
	tests := []struct {
		name       string
		intent     browsers.RequestIntent
		want       browsers.Decision
		wantReason string
	}{
		{name: "handles static-read", intent: browsers.RequestIntent{Shape: browsers.ShapeStaticRead}, want: browsers.DecisionHandle, wantReason: ""},
		{name: "handles static-snapshot", intent: browsers.RequestIntent{Shape: browsers.ShapeStaticSnapshot}, want: browsers.DecisionHandle, wantReason: ""},
		{name: "skips rendered-read", intent: browsers.RequestIntent{Shape: browsers.ShapeRenderedRead}, want: browsers.DecisionSkip, wantReason: "ghost requires Chrome for rendered-read"},
		{name: "skips visual", intent: browsers.RequestIntent{Shape: browsers.ShapeVisual}, want: browsers.DecisionSkip, wantReason: "ghost requires Chrome for visual"},
		{name: "skips interaction", intent: browsers.RequestIntent{Shape: browsers.ShapeInteraction}, want: browsers.DecisionSkip, wantReason: "ghost requires Chrome for interaction"},
		{name: "skips session-state", intent: browsers.RequestIntent{Shape: browsers.ShapeSessionState}, want: browsers.DecisionSkip, wantReason: "ghost requires Chrome for session-state"},
		{name: "skips network-control", intent: browsers.RequestIntent{Shape: browsers.ShapeNetworkControl}, want: browsers.DecisionSkip, wantReason: "ghost requires Chrome for network-control"},
		{name: "skips download-upload", intent: browsers.RequestIntent{Shape: browsers.ShapeDownloadUpload}, want: browsers.DecisionSkip, wantReason: "ghost requires Chrome for download-upload"},
		{name: "skips state-changing static-read", intent: browsers.RequestIntent{Shape: browsers.ShapeStaticRead, StateChanging: true}, want: browsers.DecisionSkip, wantReason: "state-changing requests require a full browser"},
		{name: "skips unknown shape", intent: browsers.RequestIntent{Shape: "unknown"}, want: browsers.DecisionSkip, wantReason: "ghost requires Chrome for unknown"},
		{name: "skips empty shape", intent: browsers.RequestIntent{}, want: browsers.DecisionSkip, wantReason: "ghost requires Chrome for "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.CanHandle(tt.intent)
			if got.Decision != tt.want {
				t.Errorf("CanHandle(%+v).Decision = %q, want %q", tt.intent, got.Decision, tt.want)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("CanHandle(%+v).Reason = %q, want %q", tt.intent, got.Reason, tt.wantReason)
			}
		})
	}
}

func TestGhostCanHandle_DirectCall(t *testing.T) {
	tests := []struct {
		name   string
		intent browsers.RequestIntent
		want   browsers.Decision
	}{
		{"static-read handled", browsers.RequestIntent{Shape: browsers.ShapeStaticRead}, browsers.DecisionHandle},
		{"static-snapshot handled", browsers.RequestIntent{Shape: browsers.ShapeStaticSnapshot}, browsers.DecisionHandle},
		{"rendered-read skipped", browsers.RequestIntent{Shape: browsers.ShapeRenderedRead}, browsers.DecisionSkip},
		{"state-changing skipped", browsers.RequestIntent{Shape: browsers.ShapeStaticRead, StateChanging: true}, browsers.DecisionSkip},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ghostCanHandle(tt.intent)
			if got.Decision != tt.want {
				t.Errorf("ghostCanHandle(%+v).Decision = %q, want %q", tt.intent, got.Decision, tt.want)
			}
		})
	}
}

func TestValidateTargetAcceptsEmpty(t *testing.T) {
	b := &Browser{}
	if err := b.ValidateTarget(browsers.TargetConfig{}); err != nil {
		t.Errorf("ValidateTarget() = %v, want nil", err)
	}
}

func TestGhostChromeHandleDecisionsCheck(t *testing.T) {
	b := &Browser{}
	checks := b.DoctorChecks(browsers.TargetConfig{})
	if len(checks) == 0 {
		t.Fatal("DoctorChecks returned empty slice")
	}
	var found *browsers.DoctorCheck
	for i := range checks {
		if checks[i].ID == "handle_decisions" {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatal("handle_decisions check not found in ghost-chrome DoctorChecks")
	}
	if found.Description == "" {
		t.Error("handle_decisions check has empty description")
	}
	result := found.Fn(context.Background(), nil)
	if result.Status != browsers.DoctorPass {
		t.Errorf("handle_decisions status = %v, want DoctorPass; detail: %s", result.Status, result.Detail)
	}
	wantDetail := "static shapes handled, interactive shapes skipped, state-changing safety enforced"
	if result.Detail != wantDetail {
		t.Errorf("handle_decisions detail = %q, want %q", result.Detail, wantDetail)
	}
}
