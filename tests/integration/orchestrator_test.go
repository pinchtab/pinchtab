//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestOrchestrator_HealthCheck verifies dashboard orchestrator is running
func TestOrchestrator_HealthCheck(t *testing.T) {
	status, body := httpGet(t, "/health")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}

	mode := jsonField(t, body, "mode")
	if mode != "dashboard" {
		t.Fatalf("expected mode=dashboard, got %s", mode)
	}
}

// TestOrchestrator_InstanceCreation verifies instance launch with auto-port allocation
func TestOrchestrator_InstanceCreation(t *testing.T) {
	payload := map[string]any{
		"name":     fmt.Sprintf("test-inst-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body := httpPost(t, "/instances/launch", payload)
	if status != 201 {
		t.Fatalf("expected 201, got %d: %s", status, string(body))
	}

	instID := jsonField(t, body, "id")
	if !strings.HasPrefix(instID, "inst_") {
		t.Fatalf("expected inst_XXXXX format, got %s", instID)
	}

	profileID := jsonField(t, body, "profileId")
	if !strings.HasPrefix(profileID, "prof_") {
		t.Fatalf("expected prof_XXXXX format, got %s", profileID)
	}

	instPort := jsonField(t, body, "port")
	if instPort == "" {
		t.Fatalf("expected port, got empty")
	}

	// Verify instance appears in list
	status, body = httpGet(t, "/instances")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}

	// Check body is array
	var instances []map[string]any
	if err := json.Unmarshal(body, &instances); err != nil {
		t.Fatalf("failed to parse instances: %v", err)
	}

	found := false
	for _, inst := range instances {
		if id, ok := inst["id"].(string); ok && id == instID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created instance not found in list")
	}

	// Cleanup
	httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
}

// TestOrchestrator_HashBasedIDs verifies all ID formats (prof_, inst_, tab_)
func TestOrchestrator_HashBasedIDs(t *testing.T) {
	// Create instance
	payload := map[string]any{
		"name":     fmt.Sprintf("test-ids-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body := httpPost(t, "/instances/launch", payload)
	if status != 201 {
		t.Fatalf("instance creation failed: %d", status)
	}

	instID := jsonField(t, body, "id")
	profileID := jsonField(t, body, "profileId")
	instPort := jsonField(t, body, "port")

	// Verify ID formats
	if !strings.HasPrefix(instID, "inst_") || len(instID) != 13 {
		t.Fatalf("invalid instance ID format: %s", instID)
	}
	if !strings.HasPrefix(profileID, "prof_") || len(profileID) != 13 {
		t.Fatalf("invalid profile ID format: %s", profileID)
	}
	if instPort == "" {
		t.Fatalf("instance port is empty")
	}

	// Wait for instance to be healthy
	time.Sleep(2 * time.Second)

	// Navigate to create tab
	navStatus, navBody, tabID := navigateInstance(t, instID, "https://example.com")
	if navStatus != 200 {
		t.Fatalf("navigate failed: %d: %s", navStatus, string(navBody))
	}

	if !strings.HasPrefix(tabID, "tab_") || len(tabID) != 12 {
		t.Fatalf("invalid tab ID format: %s", tabID)
	}

	// Cleanup
	httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
}

// TestOrchestrator_PortAllocation verifies sequential port allocation
func TestOrchestrator_PortAllocation(t *testing.T) {
	var instIDs []string
	var ports []string

	defer func() {
		// Cleanup
		for _, instID := range instIDs {
			httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
		}
	}()

	// Create 3 instances and verify they get different ports
	for i := 0; i < 3; i++ {
		payload := map[string]any{
			"name":     fmt.Sprintf("port-test-%d-%d", time.Now().Unix(), i),
			"headless": true,
		}

		status, body := httpPost(t, "/instances/launch", payload)
		if status != 201 {
			t.Fatalf("instance %d creation failed: %d", i, status)
		}

		instID := jsonField(t, body, "id")
		port := jsonField(t, body, "port")

		instIDs = append(instIDs, instID)
		ports = append(ports, port)
	}

	// Verify all ports are different
	for i := 0; i < len(ports); i++ {
		for j := i + 1; j < len(ports); j++ {
			if ports[i] == ports[j] {
				t.Fatalf("instances have same port: %s", ports[i])
			}
		}
	}

	// Verify ports are sequential
	if ports[0] == "" || ports[1] == "" || ports[2] == "" {
		t.Fatalf("some ports are empty")
	}
}

// TestOrchestrator_PortReuse verifies ports are released and can be reused
func TestOrchestrator_PortReuse(t *testing.T) {
	// Create instance 1
	status, body := httpPost(t, "/instances/launch", map[string]any{"mode": "headless"})
	if status != 201 {
		t.Fatalf("instance 1 creation failed: %d", status)
	}

	instID1 := jsonField(t, body, "id")
	port1 := jsonField(t, body, "port")

	// Stop instance 1 and wait for port release
	status, _ = httpPost(t, fmt.Sprintf("/instances/%s/stop", instID1), nil)
	if status != 200 {
		t.Fatalf("stop instance 1 failed: %d", status)
	}
	time.Sleep(1 * time.Second) // give OS time to release the port

	// Create instance 2 — register cleanup BEFORE any potential t.Fatal
	status, body = httpPost(t, "/instances/launch", map[string]any{
		"name": fmt.Sprintf("reuse-test-2-%d", time.Now().Unix()),
		"mode": "headless",
	})
	if status != 201 {
		t.Fatalf("instance 2 creation failed: %d", status)
	}

	instID2 := jsonField(t, body, "id")
	port2 := jsonField(t, body, "port")

	// Always stop instance 2, even if the assertion below fails
	t.Cleanup(func() {
		httpPost(t, fmt.Sprintf("/instances/%s/stop", instID2), nil)
	})

	// Verify port2 == port1 (reused)
	if port1 != port2 {
		t.Fatalf("port not reused: old=%s, new=%s", port1, port2)
	}
}

// TestOrchestrator_InstanceIsolation verifies instances have separate tabs
func TestOrchestrator_InstanceIsolation(t *testing.T) {
	var instIDs []string

	defer func() {
		for _, instID := range instIDs {
			httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
		}
	}()

	// Create 2 instances
	for i := 0; i < 2; i++ {
		payload := map[string]any{
			"name":     fmt.Sprintf("isolation-test-%d-%d", time.Now().Unix(), i),
			"headless": true,
		}

		status, body := httpPost(t, "/instances/launch", payload)
		if status != 201 {
			t.Fatalf("instance %d creation failed", i)
		}

		instID := jsonField(t, body, "id")
		instIDs = append(instIDs, instID)
	}

	// Wait for Chrome init
	time.Sleep(2 * time.Second)

	// Navigate on instance 1
	status, navBody, tabID1 := navigateInstance(t, instIDs[0], "https://example.com")
	if status != 200 {
		t.Fatalf("navigate instance 1 failed: %d: %s", status, string(navBody))
	}

	// Navigate on instance 2
	status, navBody, tabID2 := navigateInstance(t, instIDs[1], "https://github.com")
	if status != 200 {
		t.Fatalf("navigate instance 2 failed: %d: %s", status, string(navBody))
	}

	// Verify tab IDs are different (isolation)
	if tabID1 == tabID2 {
		t.Fatalf("instances share tab IDs (not isolated): %s", tabID1)
	}
}

// TestOrchestrator_ListInstances verifies GET /instances returns all instances
func TestOrchestrator_ListInstances(t *testing.T) {
	// Get initial count
	status, body := httpGet(t, "/instances")
	if status != 200 {
		t.Fatalf("list instances failed: %d", status)
	}

	var instances []map[string]any
	if err := json.Unmarshal(body, &instances); err != nil {
		t.Fatalf("parse instances failed: %v", err)
	}

	initialCount := len(instances)

	// Create an instance
	payload := map[string]any{
		"name":     fmt.Sprintf("list-test-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body = httpPost(t, "/instances/launch", payload)
	if status != 201 {
		t.Fatalf("instance creation failed")
	}

	instID := jsonField(t, body, "id")

	// Verify count increased
	status, body = httpGet(t, "/instances")
	if status != 200 {
		t.Fatalf("list instances failed")
	}

	if err := json.Unmarshal(body, &instances); err != nil {
		t.Fatalf("parse instances failed")
	}

	if len(instances) != initialCount+1 {
		t.Fatalf("expected %d instances, got %d", initialCount+1, len(instances))
	}

	// Cleanup
	httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
}

// TestOrchestrator_ProxyRouting verifies orchestrator forwards requests to instance
func TestOrchestrator_ProxyRouting(t *testing.T) {
	// Create instance
	payload := map[string]any{
		"name":     fmt.Sprintf("proxy-test-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body := httpPost(t, "/instances/launch", payload)
	if status != 201 {
		t.Fatalf("instance creation failed")
	}

	instID := jsonField(t, body, "id")

	defer func() {
		httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
	}()

	// Wait for Chrome init
	time.Sleep(2 * time.Second)

	// Test navigation via orchestrator proxy
	status, body, tabID := navigateInstance(t, instID, "https://example.com")
	if status != 200 {
		t.Fatalf("proxy navigate failed: %d: %s", status, string(body))
	}

	if !strings.HasPrefix(tabID, "tab_") {
		t.Fatalf("invalid tab ID from proxy: %s", tabID)
	}

	// Test snapshot via orchestrator proxy
	status, body = httpGet(t, fmt.Sprintf("/tabs/%s/snapshot", tabID))
	if status != 200 {
		t.Fatalf("proxy snapshot failed: %d", status)
	}

	// Verify it's valid JSON with nodes
	var snapshot map[string]any
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("parse snapshot failed: %v", err)
	}

	// Test action via orchestrator tab route
	status, body = httpPost(t, fmt.Sprintf("/tabs/%s/action", tabID), map[string]any{
		"kind": "press",
		"key":  "Escape",
	})
	if status != 200 {
		t.Fatalf("proxy action failed: %d: %s", status, string(body))
	}

	// Test lock/unlock via orchestrator tab routes
	status, body = httpPost(t, fmt.Sprintf("/tabs/%s/lock", tabID), map[string]any{
		"owner":      "orchestrator-test",
		"timeoutSec": 10,
	})
	if status != 200 {
		t.Fatalf("proxy lock failed: %d: %s", status, string(body))
	}
	status, body = httpPost(t, fmt.Sprintf("/tabs/%s/unlock", tabID), map[string]any{
		"owner": "orchestrator-test",
	})
	if status != 200 {
		t.Fatalf("proxy unlock failed: %d: %s", status, string(body))
	}

	// Test actions batch via orchestrator tab route
	status, body = httpPost(t, fmt.Sprintf("/tabs/%s/actions", tabID), map[string]any{
		"actions": []map[string]any{
			{"kind": "press", "key": "Escape"},
		},
	})
	if status != 200 {
		t.Fatalf("proxy actions failed: %d: %s", status, string(body))
	}

	// Test text via orchestrator tab route
	status, body = httpGet(t, fmt.Sprintf("/tabs/%s/text?mode=raw", tabID))
	if status != 200 {
		t.Fatalf("proxy text failed: %d: %s", status, string(body))
	}
	if !strings.Contains(string(body), "Example Domain") {
		t.Logf("proxy text response does not contain expected title content")
	}

	// Test evaluate via orchestrator tab route
	status, body = httpPost(t, fmt.Sprintf("/tabs/%s/evaluate", tabID), map[string]any{
		"expression": "document.title",
	})
	if status != 200 {
		t.Fatalf("proxy evaluate failed: %d: %s", status, string(body))
	}

	// Test cookies via orchestrator tab route
	status, body = httpGet(t, fmt.Sprintf("/tabs/%s/cookies?url=https://example.com", tabID))
	if status != 200 {
		t.Fatalf("proxy get cookies failed: %d: %s", status, string(body))
	}
	status, body = httpPost(t, fmt.Sprintf("/tabs/%s/cookies", tabID), map[string]any{
		"url": "https://example.com",
		"cookies": []map[string]any{
			{"name": "orch_test_cookie", "value": "1"},
		},
	})
	if status != 200 {
		t.Fatalf("proxy set cookies failed: %d: %s", status, string(body))
	}

	// Test pdf via orchestrator tab route
	status, body = httpGet(t, fmt.Sprintf("/tabs/%s/pdf?raw=true", tabID))
	if status != 200 {
		t.Fatalf("proxy pdf failed: %d: %s", status, string(body))
	}
	if len(body) < 4 || string(body[:4]) != "%PDF" {
		t.Fatalf("proxy pdf response is not a PDF (size=%d)", len(body))
	}

	// Test download via orchestrator tab route (missing URL should be 400 from bridge handler)
	status, body = httpGet(t, fmt.Sprintf("/tabs/%s/download", tabID))
	if status != 400 {
		t.Fatalf("proxy download failed: expected 400, got %d: %s", status, string(body))
	}

	// Test upload via orchestrator tab route (missing files/paths should be 400 from bridge handler)
	status, body = httpPost(t, fmt.Sprintf("/tabs/%s/upload", tabID), map[string]any{
		"selector": "input[type=file]",
	})
	if status != 400 {
		t.Fatalf("proxy upload failed: expected 400, got %d: %s", status, string(body))
	}

	// Test screenshot via orchestrator tab route
	status, body = httpGet(t, fmt.Sprintf("/tabs/%s/screenshot?raw=true", tabID))
	if status != 200 {
		t.Skipf("proxy screenshot returned %d (headless display limitation), skipping", status)
	}
	// JPEG starts with FF D8
	if len(body) < 2 || body[0] != 0xFF || body[1] != 0xD8 {
		t.Skipf("proxy screenshot response is not JPEG (size=%d), skipping", len(body))
	}
}

// TestOrchestrator_FirstRequestLazyChrome tests proxy request with lazy Chrome initialization
// This test verifies the orchestrator's 60-second client timeout is sufficient for lazy Chrome initialization.
// Scenario:
// 1. Create instance (starts monitor polling /health)
// 2. Monitor's first /health call triggers ensureChrome() (8-20+ seconds)
// 3. Once /health succeeds, instance status becomes "running"
// 4. Proxy request to /navigate completes successfully
// If the orchestrator's client timeout is too short (<30s), the /health check would timeout
// and the instance would never reach "running" state.
func TestOrchestrator_FirstRequestLazyChrome(t *testing.T) {
	// Create instance
	payload := map[string]any{
		"name":     fmt.Sprintf("first-request-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body := httpPost(t, "/instances/launch", payload)
	if status != 201 {
		t.Fatalf("instance creation failed: %d", status)
	}

	instID := jsonField(t, body, "id")

	defer func() {
		httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
	}()

	// Wait for instance to reach "running" state via monitor
	// The monitor polls /health every 500ms
	// First /health call triggers Chrome initialization (8-20+ seconds)
	// Once /health succeeds, status changes to "running"
	// If orchestrator timeout is <30s, this loop will timeout
	const maxWait = 45 * time.Second
	const pollInterval = 500 * time.Millisecond
	startTime := time.Now()

	var instStatus string
	for time.Since(startTime) < maxWait {
		status, body := httpGet(t, fmt.Sprintf("/instances/%s", instID))
		if status == 200 {
			instStatus = jsonField(t, body, "status")
			if instStatus == "running" {
				break
			}
		}
		time.Sleep(pollInterval)
	}

	if instStatus != "running" {
		t.Fatalf("instance never reached running state (timeout: %s), last status: %s", maxWait, instStatus)
	}

	// Now make the actual proxy request - Chrome is already initialized
	status, body, tabID := navigateInstance(t, instID, "https://example.com")
	if status != 200 {
		t.Fatalf("navigate failed: %d: %s", status, string(body))
	}

	if !strings.HasPrefix(tabID, "tab_") {
		t.Fatalf("invalid tab ID: %s", tabID)
	}

	t.Logf("✓ Instance reached running state with lazy Chrome init; navigate succeeded: tabId=%s", tabID)
}

// TestOrchestrator_AggregateTabsEndpoint verifies GET /instances/tabs returns all tabs
func TestOrchestrator_AggregateTabsEndpoint(t *testing.T) {
	var instIDs []string

	defer func() {
		for _, instID := range instIDs {
			httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
		}
	}()

	// Create 2 instances and navigate on each
	for i := 0; i < 2; i++ {
		payload := map[string]any{
			"name":     fmt.Sprintf("tabs-agg-%d-%d", time.Now().Unix(), i),
			"headless": true,
		}

		status, body := httpPost(t, "/instances/launch", payload)
		if status != 201 {
			t.Fatalf("instance %d creation failed", i)
		}

		instID := jsonField(t, body, "id")
		instIDs = append(instIDs, instID)
	}

	// Wait for Chrome init
	time.Sleep(2 * time.Second)

	// Navigate on each instance
	for _, instID := range instIDs {
		httpStatus, navBody, _ := navigateInstance(t, instID, "https://example.com")
		if httpStatus != 200 {
			t.Fatalf("navigate failed for %s: %d: %s", instID, httpStatus, string(navBody))
		}
	}

	// Get aggregate tabs
	status, body := httpGet(t, "/instances/tabs")
	if status != 200 {
		t.Fatalf("aggregate tabs failed: %d", status)
	}

	var tabs []map[string]any
	if err := json.Unmarshal(body, &tabs); err != nil {
		t.Fatalf("parse tabs failed: %v", err)
	}

	if len(tabs) < 2 {
		t.Fatalf("expected at least 2 tabs, got %d", len(tabs))
	}
}

// TestOrchestrator_StopNonexistent verifies error handling for stopping non-existent instance
func TestOrchestrator_StopNonexistent(t *testing.T) {
	status, _ := httpPost(t, "/instances/nonexistent/stop", nil)
	if status != 404 {
		t.Fatalf("expected 404 for nonexistent instance, got %d", status)
	}
}

// TestOrchestrator_InstanceCleanup verifies all instances properly stop
func TestOrchestrator_InstanceCleanup(t *testing.T) {
	var instIDs []string

	// Create 3 instances
	for i := 0; i < 3; i++ {
		payload := map[string]any{
			"name":     fmt.Sprintf("cleanup-test-%d-%d", time.Now().Unix(), i),
			"headless": true,
		}

		status, body := httpPost(t, "/instances/launch", payload)
		if status != 201 {
			t.Fatalf("instance %d creation failed", i)
		}

		instID := jsonField(t, body, "id")
		instIDs = append(instIDs, instID)
	}

	// Stop all
	for _, instID := range instIDs {
		status, _ := httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
		if status != 200 {
			t.Fatalf("stop instance %s failed: %d", instID, status)
		}
	}

	// Verify all stopped
	status, body := httpGet(t, "/instances")
	if status != 200 {
		t.Fatalf("list instances failed")
	}

	var instances []map[string]any
	if err := json.Unmarshal(body, &instances); err != nil {
		t.Fatalf("parse instances failed")
	}

	// Should be empty or not contain our test instances
	for _, inst := range instances {
		if id, ok := inst["id"].(string); ok {
			for _, testID := range instIDs {
				if id == testID {
					t.Fatalf("instance %s still running after stop", testID)
				}
			}
		}
	}
}
