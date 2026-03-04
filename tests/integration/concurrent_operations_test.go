package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestConcurrentOperationsMultipleInstances tests that 3 instances with unique profiles
// can all reach "running" status and handle sequential navigations without interference.
func TestConcurrentOperationsMultipleInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	baseURL := "http://localhost:9867"

	// Launch 3 instances with unique profiles
	t.Log("Launching 3 instances with unique profiles...")
	instances := make([]*Instance, 3)
	for i := 0; i < 3; i++ {
		profileName := fmt.Sprintf("concurrent-test-%d", i+1)
		inst, err := launchInstance(baseURL, profileName, true)
		if err != nil {
			t.Fatalf("Failed to launch instance %d: %v", i+1, err)
		}
		instances[i] = inst
		t.Logf("  Instance %d: %s (port %s)", i+1, inst.ID, inst.Port)
	}

	// Wait for all 3 to be running concurrently
	t.Log("Waiting for all 3 instances to reach 'running'...")
	var wg sync.WaitGroup
	running := make([]bool, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			running[idx] = waitForInstanceRunning(t, baseURL, instances[idx].ID, 45*time.Second)
		}(i)
	}
	wg.Wait()

	for i, r := range running {
		if !r {
			t.Errorf("Instance %d (%s) never reached 'running'", i+1, instances[i].ID)
		}
	}
	if t.Failed() {
		t.Fatal("Not all instances became ready — aborting further checks")
	}
	t.Log("✓ All 3 instances running")

	// Navigate sequentially via orchestrator shorthand
	// Simple strategy allocates across running instances
	t.Log("Navigating via orchestrator shorthand (sequential)...")
	urls := []string{
		"https://example.com",
		"https://httpbin.org/get",
		"https://example.org",
	}
	for i, url := range urls {
		if err := navigateViaOrchestrator(baseURL, url); err != nil {
			t.Errorf("Navigation %d (%s) failed: %v", i+1, url, err)
		} else {
			t.Logf("  ✓ Navigation %d ok (%s)", i+1, url)
		}
	}

	// Verify all 3 instances still present
	t.Log("Verifying all 3 instances still exist...")
	all, err := getInstances(baseURL)
	if err != nil {
		t.Fatalf("Failed to list instances: %v", err)
	}
	found := 0
	for _, orig := range instances {
		for _, inst := range all {
			if id, ok := inst["id"].(string); ok && id == orig.ID {
				found++
				t.Logf("  %s: %s", orig.ID, inst["status"])
				break
			}
		}
	}
	if found < 3 {
		t.Errorf("Expected to find 3 instances, found %d", found)
	}

	// Cleanup
	t.Log("Cleaning up...")
	for _, inst := range instances {
		_ = stopInstance(baseURL, inst.ID)
	}
}

// navigateViaOrchestrator calls POST /navigate on the orchestrator.
// The simple strategy auto-allocates a running instance.
func navigateViaOrchestrator(baseURL, url string) error {
	body, _ := json.Marshal(map[string]string{"url": url})
	resp, err := http.Post(
		fmt.Sprintf("%s/navigate", baseURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
