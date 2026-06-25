package all_test

import (
	"context"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
)

type handleStub struct{}

func (handleStub) ID() string                                                  { return "handle-stub" }
func (handleStub) DisplayName() string                                         { return "HandleStub" }
func (handleStub) Capabilities() browsers.CapabilitySet                        { return browsers.CapabilitySet{} }
func (handleStub) DiscoverBinary() browsers.BinaryDiscovery                    { return browsers.BinaryDiscovery{} }
func (handleStub) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck { return nil }
func (handleStub) BuildLaunchArgs(_ browsers.LaunchConfig) ([]string, []string, error) {
	return nil, nil, nil
}
func (handleStub) SupportsRemoteCDP() bool { return false }
func (handleStub) GeoAlignment(_ browsers.GeoConfig) browsers.GeoStrategy {
	return browsers.GeoStrategy{}
}
func (handleStub) ValidateTarget(_ browsers.TargetConfig) error { return nil }
func (handleStub) ClassifyLaunchError(_ browsers.LaunchFailure) browsers.LaunchErrorKind {
	return browsers.LaunchErrorUnknown
}
func (handleStub) CanHandle(_ browsers.RequestIntent) browsers.HandleDecision {
	return browsers.HandleDecision{Decision: browsers.DecisionHandle}
}
func (handleStub) NewRuntimeInstance(_ context.Context, _ bool) browsers.RuntimeInstance { return nil }

type skipStub struct{}

func (skipStub) ID() string                                                  { return "skip-stub" }
func (skipStub) DisplayName() string                                         { return "SkipStub" }
func (skipStub) Capabilities() browsers.CapabilitySet                        { return browsers.CapabilitySet{} }
func (skipStub) DiscoverBinary() browsers.BinaryDiscovery                    { return browsers.BinaryDiscovery{} }
func (skipStub) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck { return nil }
func (skipStub) BuildLaunchArgs(_ browsers.LaunchConfig) ([]string, []string, error) {
	return nil, nil, nil
}
func (skipStub) SupportsRemoteCDP() bool { return false }
func (skipStub) GeoAlignment(_ browsers.GeoConfig) browsers.GeoStrategy {
	return browsers.GeoStrategy{}
}
func (skipStub) ValidateTarget(_ browsers.TargetConfig) error { return nil }
func (skipStub) ClassifyLaunchError(_ browsers.LaunchFailure) browsers.LaunchErrorKind {
	return browsers.LaunchErrorUnknown
}
func (skipStub) CanHandle(_ browsers.RequestIntent) browsers.HandleDecision {
	return browsers.HandleDecision{Decision: browsers.DecisionSkip, Reason: "not supported"}
}
func (skipStub) NewRuntimeInstance(_ context.Context, _ bool) browsers.RuntimeInstance { return nil }

type failStub struct{}

func (failStub) ID() string                                                  { return "fail-stub" }
func (failStub) DisplayName() string                                         { return "FailStub" }
func (failStub) Capabilities() browsers.CapabilitySet                        { return browsers.CapabilitySet{} }
func (failStub) DiscoverBinary() browsers.BinaryDiscovery                    { return browsers.BinaryDiscovery{} }
func (failStub) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck { return nil }
func (failStub) BuildLaunchArgs(_ browsers.LaunchConfig) ([]string, []string, error) {
	return nil, nil, nil
}
func (failStub) SupportsRemoteCDP() bool { return false }
func (failStub) GeoAlignment(_ browsers.GeoConfig) browsers.GeoStrategy {
	return browsers.GeoStrategy{}
}
func (failStub) ValidateTarget(_ browsers.TargetConfig) error { return nil }
func (failStub) ClassifyLaunchError(_ browsers.LaunchFailure) browsers.LaunchErrorKind {
	return browsers.LaunchErrorUnknown
}
func (failStub) CanHandle(_ browsers.RequestIntent) browsers.HandleDecision {
	return browsers.HandleDecision{Decision: browsers.DecisionFail, Reason: "fatal: missing dependency"}
}
func (failStub) NewRuntimeInstance(_ context.Context, _ bool) browsers.RuntimeInstance { return nil }

var (
	registerHandleOnce sync.Once
	registerSkipOnce   sync.Once
	registerFailOnce   sync.Once
)

func TestHandleContract_DirectHandle(t *testing.T) {
	registerHandleOnce.Do(func() { browsers.Register(&handleStub{}) })

	b := browsers.MustGet("handle-stub")

	intents := []browsers.RequestIntent{
		{Shape: browsers.ShapeStaticRead},
		{Shape: browsers.ShapeRenderedRead},
		{Shape: browsers.ShapeInteraction},
		{Shape: browsers.ShapeVisual},
		{Shape: browsers.ShapeStaticRead, StateChanging: true},
	}

	for _, intent := range intents {
		got := b.CanHandle(intent)
		if got.Decision != browsers.DecisionHandle {
			t.Errorf("CanHandle(%+v).Decision = %q, want %q", intent, got.Decision, browsers.DecisionHandle)
		}
		if got.Reason != "" {
			t.Errorf("CanHandle(%+v).Reason = %q, want empty", intent, got.Reason)
		}
	}
}

func TestHandleContract_SkipToNext(t *testing.T) {
	registerSkipOnce.Do(func() { browsers.Register(&skipStub{}) })

	b := browsers.MustGet("skip-stub")

	intents := []browsers.RequestIntent{
		{Shape: browsers.ShapeStaticRead},
		{Shape: browsers.ShapeRenderedRead},
		{Shape: browsers.ShapeInteraction},
	}

	for _, intent := range intents {
		got := b.CanHandle(intent)
		if got.Decision != browsers.DecisionSkip {
			t.Errorf("CanHandle(%+v).Decision = %q, want %q", intent, got.Decision, browsers.DecisionSkip)
		}
		if got.Reason == "" {
			t.Errorf("CanHandle(%+v).Reason is empty, want non-empty", intent)
		}
	}
}

func TestHandleContract_HardFailure(t *testing.T) {
	registerFailOnce.Do(func() { browsers.Register(&failStub{}) })

	b := browsers.MustGet("fail-stub")

	intents := []browsers.RequestIntent{
		{Shape: browsers.ShapeStaticRead},
		{Shape: browsers.ShapeRenderedRead},
		{Shape: browsers.ShapeInteraction},
	}

	for _, intent := range intents {
		got := b.CanHandle(intent)
		if got.Decision != browsers.DecisionFail {
			t.Errorf("CanHandle(%+v).Decision = %q, want %q", intent, got.Decision, browsers.DecisionFail)
		}
		if got.Reason == "" {
			t.Errorf("CanHandle(%+v).Reason is empty, want non-empty", intent)
		}
	}
}

func TestHandleContract_DeterministicReasons(t *testing.T) {
	b := browsers.MustGet("ghost-chrome")

	t.Run("rendered-read reason is stable", func(t *testing.T) {
		first := b.CanHandle(browsers.RequestIntent{Shape: browsers.ShapeRenderedRead})
		for i := 1; i < 10; i++ {
			got := b.CanHandle(browsers.RequestIntent{Shape: browsers.ShapeRenderedRead})
			if got.Reason != first.Reason {
				t.Fatalf("iteration %d: Reason = %q, want %q", i, got.Reason, first.Reason)
			}
		}
	})

	t.Run("state-changing reason is stable", func(t *testing.T) {
		intent := browsers.RequestIntent{
			Shape:         browsers.ShapeStaticRead,
			StateChanging: true,
		}
		first := b.CanHandle(intent)
		for i := 1; i < 10; i++ {
			got := b.CanHandle(intent)
			if got.Reason != first.Reason {
				t.Fatalf("iteration %d: Reason = %q, want %q", i, got.Reason, first.Reason)
			}
		}
	})
}

func TestHandleContract_StateChangingSafety(t *testing.T) {
	tests := []struct {
		name          string
		browserID     string
		shape         string
		stateChanging bool
		wantDecision  browsers.Decision
	}{
		{"chrome handles static-read+stateChanging", "chrome", browsers.ShapeStaticRead, true, browsers.DecisionHandle},
		{"chrome handles interaction+stateChanging", "chrome", browsers.ShapeInteraction, true, browsers.DecisionHandle},
		{"cloak handles static-read+stateChanging", "cloak", browsers.ShapeStaticRead, true, browsers.DecisionHandle},
		{"ghost-chrome handles static-read", "ghost-chrome", browsers.ShapeStaticRead, false, browsers.DecisionHandle},
		{"ghost-chrome skips static-read+stateChanging", "ghost-chrome", browsers.ShapeStaticRead, true, browsers.DecisionSkip},
		{"ghost-chrome skips static-snapshot+stateChanging", "ghost-chrome", browsers.ShapeStaticSnapshot, true, browsers.DecisionSkip},
		{"ghost-chrome skips rendered-read", "ghost-chrome", browsers.ShapeRenderedRead, false, browsers.DecisionSkip},
		{"ghost-chrome skips visual", "ghost-chrome", browsers.ShapeVisual, false, browsers.DecisionSkip},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := browsers.MustGet(tt.browserID)
			got := b.CanHandle(browsers.RequestIntent{
				Shape:         tt.shape,
				StateChanging: tt.stateChanging,
			})
			if got.Decision != tt.wantDecision {
				t.Errorf("CanHandle() = %q, want %q (reason: %q)", got.Decision, tt.wantDecision, got.Reason)
			}
		})
	}
}
