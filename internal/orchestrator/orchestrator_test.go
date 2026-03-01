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
