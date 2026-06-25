package browsers

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

type stubBrowser struct {
	id          string
	displayName string
}

func (s *stubBrowser) ID() string                                { return s.id }
func (s *stubBrowser) DisplayName() string                       { return s.displayName }
func (s *stubBrowser) Capabilities() CapabilitySet               { return NewCapabilitySet() }
func (s *stubBrowser) DiscoverBinary() BinaryDiscovery           { return BinaryDiscovery{} }
func (s *stubBrowser) DoctorChecks(_ TargetConfig) []DoctorCheck { return nil }
func (s *stubBrowser) BuildLaunchArgs(_ LaunchConfig) ([]string, []string, error) {
	return nil, nil, nil
}
func (s *stubBrowser) SupportsRemoteCDP() bool                             { return false }
func (s *stubBrowser) GeoAlignment(_ GeoConfig) GeoStrategy                { return GeoStrategy{} }
func (s *stubBrowser) ValidateTarget(_ TargetConfig) error                 { return nil }
func (s *stubBrowser) ClassifyLaunchError(_ LaunchFailure) LaunchErrorKind { return LaunchErrorUnknown }
func (s *stubBrowser) CanHandle(_ RequestIntent) HandleDecision {
	return HandleDecision{Decision: DecisionHandle}
}
func (s *stubBrowser) NewRuntimeInstance(_ context.Context, _ bool) RuntimeInstance { return nil }

func stub(id string) Browser {
	return &stubBrowser{id: id, displayName: id}
}

func TestDuplicateRegistrationPanics(t *testing.T) {
	resetForTesting()

	Register(stub("chrome"))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "chrome") {
			t.Fatalf("panic message should mention duplicate id, got: %v", r)
		}
	}()

	Register(stub("chrome"))
}

func TestGetUnknown(t *testing.T) {
	resetForTesting()

	b, ok := Get("nonexistent")
	if ok {
		t.Fatal("expected ok=false for unknown browser")
	}
	if b != nil {
		t.Fatal("expected nil browser for unknown id")
	}
}

func TestMustGetUnknownPanicsWithKnownIDs(t *testing.T) {
	resetForTesting()

	Register(stub("alpha"))
	Register(stub("bravo"))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic from MustGet with unknown id")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "unknown") {
			t.Fatalf("panic message should contain 'unknown', got: %s", msg)
		}
		if !strings.Contains(msg, "alpha") || !strings.Contains(msg, "bravo") {
			t.Fatalf("panic message should list known IDs, got: %s", msg)
		}
	}()

	MustGet("charlie")
}

func TestIDsSorted(t *testing.T) {
	resetForTesting()

	Register(stub("zulu"))
	Register(stub("alpha"))
	Register(stub("mike"))

	ids := IDs()
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "alpha" || ids[1] != "mike" || ids[2] != "zulu" {
		t.Fatalf("expected [alpha mike zulu], got %v", ids)
	}
}

func TestIDsEmptyRegistryReturnsEmptySlice(t *testing.T) {
	resetForTesting()

	ids := IDs()
	if ids == nil {
		t.Fatal("IDs() should return empty slice, not nil")
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 IDs, got %d", len(ids))
	}
}

func TestHandleDecisionJSON(t *testing.T) {
	hd := HandleDecision{Decision: DecisionHandle, Reason: "test"}
	data, err := json.Marshal(hd)
	if err != nil {
		t.Fatalf("json.Marshal(HandleDecision) error: %v", err)
	}

	var got HandleDecision
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if got.Decision != DecisionHandle {
		t.Errorf("Decision = %q, want %q", got.Decision, DecisionHandle)
	}
	if got.Reason != "test" {
		t.Errorf("Reason = %q, want %q", got.Reason, "test")
	}

	hdNoReason := HandleDecision{Decision: DecisionSkip}
	data2, _ := json.Marshal(hdNoReason)
	if strings.Contains(string(data2), "reason") {
		t.Errorf("expected reason to be omitted; got %s", data2)
	}
}

func TestConcurrentRegisterAndGet(t *testing.T) {
	resetForTesting()

	const n = 100
	var wg sync.WaitGroup

	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			id := "browser-" + strings.Repeat("x", i%10) + string(rune('a'+i%26))
			// Unique-enough id; ignore panic from rare collisions.
			func() {
				defer func() { _ = recover() }()
				Register(stub(id))
			}()
		}(i)
	}

	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			Get("anything")
			IDs()
		}()
	}

	wg.Wait()
}
