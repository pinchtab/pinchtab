package orchestrator

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestOrchestrator_Launch_Lifecycle(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, err := o.Launch("profile1", "9001", true)
	if err != nil {
		t.Fatalf("First launch failed: %v", err)
	}
	if inst.Status != "starting" {
		t.Errorf("expected status starting, got %s", inst.Status)
	}

	_, err = o.Launch("profile1", "9002", true)
	if err == nil {
		t.Error("expected error when launching duplicate profile")
	}

	runner.portAvail = false
	_, err = o.Launch("profile2", "9001", true)
	if err == nil {
		t.Error("expected error when launching on occupied port")
	}
}

func TestOrchestrator_ListAndStop(t *testing.T) {
	alive := true
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return alive }
	defer func() { processAliveFunc = old }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, _ := o.Launch("p1", "9001", true)

	if len(o.List()) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(o.List()))
	}

	alive = false
	err := o.Stop(inst.ID)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	instances := o.List()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances after stop, got %d", len(instances))
	}
}

func TestOrchestrator_StopProfile(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return true }
	defer func() { processAliveFunc = old }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	o.mu.Lock()
	instID := o.idMgr.InstanceID(o.idMgr.ProfileID("p1"), "p1")
	o.instances[instID] = &InstanceInternal{
		Instance: bridge.Instance{
			ID:          instID,
			ProfileID:   o.idMgr.ProfileID("p1"),
			ProfileName: "p1",
			Port:        "9001",
			Status:      "running",
		},
		URL: "http://localhost:9001",
	}
	o.mu.Unlock()

	processAliveFunc = func(pid int) bool { return false }

	err := o.StopProfile("p1")
	if err != nil {
		t.Fatalf("StopProfile failed: %v", err)
	}

	instances := o.List()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances after stop, got %d", len(instances))
	}
}

// === Security Validation Tests ===

func TestOrchestrator_Launch_RejectsPathTraversal(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	badNames := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"double dot prefix", "../malicious", "cannot contain '..'"},
		{"double dot suffix", "test/..", "cannot contain '..'"},
		{"double dot middle", "test/../other", "cannot contain '..'"},
		{"forward slash", "test/nested", "cannot contain '/'"},
		{"backslash", "test\\nested", "cannot contain '/'"},
		{"empty name", "", "cannot be empty"},
		{"absolute path attempt", "../../../etc/passwd", "cannot contain"},
	}

	for _, tt := range badNames {
		t.Run(tt.name, func(t *testing.T) {
			_, err := o.Launch(tt.input, "9999", true)
			if err == nil {
				t.Errorf("Launch(%q) should have returned error", tt.input)
				return
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("Launch(%q) error = %q, want containing %q", tt.input, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestOrchestrator_Launch_AcceptsValidNames(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	validNames := []string{
		"simple",
		"with-dash",
		"with_underscore",
		"with.dot",
		"CamelCase",
		"123numeric",
		"a",
	}

	for i, name := range validNames {
		t.Run(name, func(t *testing.T) {
			port := 9100 + i
			inst, err := o.Launch(name, string(rune('0'+port%10))+string(rune('0'+(port/10)%10))+string(rune('0'+(port/100)%10))+string(rune('0'+(port/1000)%10)), true)
			if err != nil {
				t.Errorf("Launch(%q) unexpected error: %v", name, err)
				return
			}
			if inst.ProfileName != name {
				t.Errorf("Launch(%q) profileName = %q", name, inst.ProfileName)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
