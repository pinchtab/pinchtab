package integration

import (
	"testing"
	"time"
)

// TestProfileConflictTwoInstancesSameProfile tests launching 2 instances with the SAME profile name.
// This should demonstrate the SingletonLock issue: the second instance should fail to start
// because Chrome can't acquire the lock on a profile that's already in use.
//
// Expected behavior (desired): Orchestrator should either:
// 1. Reject the second instance with an error, OR
// 2. Auto-generate unique profile names
//
// Current behavior: Both instances are created but the second one fails to start Chrome.
func TestProfileConflictTwoInstancesSameProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	baseURL := "http://localhost:9867"

	profileName := "conflict-test"

	// Launch instance 1 with profile "conflict-test"
	t.Log("Launching instance 1 with profile 'conflict-test'...")
	inst1, err := launchInstance(baseURL, profileName, true)
	if err != nil {
		t.Fatalf("Failed to launch instance 1: %v", err)
	}
	t.Logf("Instance 1: ID=%s, Port=%s, Status=%s", inst1.ID, inst1.Port, inst1.Status)

	// Launch instance 2 with THE SAME profile "conflict-test"
	// This should either fail immediately OR create an instance that fails to start
	t.Log("Launching instance 2 with SAME profile 'conflict-test'...")
	inst2, err := launchInstance(baseURL, profileName, true)
	if err != nil {
		t.Logf("✓ Instance 2 launch rejected (desired behavior): %v", err)
		// This is actually good — orchestrator rejected the conflict
		return
	}
	t.Logf("Instance 2: ID=%s, Port=%s, Status=%s", inst2.ID, inst2.Port, inst2.Status)

	// Wait for both instances to attempt startup
	t.Log("Waiting for instance 1 to reach 'running' status...")
	running1 := waitForInstanceRunning(t, baseURL, inst1.ID, 30*time.Second)

	t.Log("Waiting for instance 2 to reach 'running' status...")
	running2 := waitForInstanceRunning(t, baseURL, inst2.ID, 30*time.Second)

	// Expected: instance 1 should run, instance 2 should fail
	if running1 && !running2 {
		t.Log("✓ Expected behavior: Instance 1 running, Instance 2 failed (SingletonLock conflict)")
		t.Log("  This demonstrates that instances need unique profiles or orchestrator must enforce uniqueness")
	} else if running1 && running2 {
		t.Error("✗ Unexpected: Both instances are running with same profile")
		t.Error("  This suggests profiles aren't properly isolated (Chrome might be sharing process)")
	} else if !running1 && !running2 {
		t.Error("✗ Both instances failed to start")
	} else {
		t.Log("⚠️  Instance 1 failed but Instance 2 running (unexpected state)")
	}

	// Get final state
	instances, err := getInstances(baseURL)
	if err != nil {
		t.Logf("Failed to get instances: %v", err)
		return
	}

	t.Log("\nFinal instance states:")
	for _, inst := range instances {
		if instID, ok := inst["id"].(string); ok {
			if instID == inst1.ID || instID == inst2.ID {
				t.Logf("  %s: %s (port: %v)", instID, inst["status"], inst["port"])
			}
		}
	}

	// Cleanup
	_ = stopInstance(baseURL, inst1.ID)
	_ = stopInstance(baseURL, inst2.ID)
}
