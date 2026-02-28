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
	navPayload := map[string]any{
		"url": "https://example.com",
	}

	navStatus, navBody := httpPost(t, fmt.Sprintf("/instances/%s/navigate", instID), navPayload)
	if navStatus != 200 {
		t.Fatalf("navigate failed: %d: %s", navStatus, string(navBody))
	}

	tabID := jsonField(t, navBody, "tabId")
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
	payload1 := map[string]any{
		"name":     fmt.Sprintf("reuse-test-1-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body := httpPost(t, "/instances/launch", payload1)
	if status != 201 {
		t.Fatalf("instance 1 creation failed: %d", status)
	}

	instID1 := jsonField(t, body, "id")
	port1 := jsonField(t, body, "port")

	// Stop instance 1
	status, _ = httpPost(t, fmt.Sprintf("/instances/%s/stop", instID1), nil)
	if status != 200 {
		t.Fatalf("stop instance 1 failed: %d", status)
	}

	time.Sleep(500 * time.Millisecond)

	// Create instance 2
	payload2 := map[string]any{
		"name":     fmt.Sprintf("reuse-test-2-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body = httpPost(t, "/instances/launch", payload2)
	if status != 201 {
		t.Fatalf("instance 2 creation failed: %d", status)
	}

	instID2 := jsonField(t, body, "id")
	port2 := jsonField(t, body, "port")

	// Verify port2 == port1 (reused)
	if port1 != port2 {
		t.Fatalf("port not reused: old=%s, new=%s", port1, port2)
	}

	// Cleanup
	httpPost(t, fmt.Sprintf("/instances/%s/stop", instID2), nil)
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
	nav1Payload := map[string]any{"url": "https://example.com"}
	status, body := httpPost(t, fmt.Sprintf("/instances/%s/navigate", instIDs[0]), nav1Payload)
	if status != 200 {
		t.Fatalf("navigate instance 1 failed: %d", status)
	}
	tabID1 := jsonField(t, body, "tabId")

	// Navigate on instance 2
	nav2Payload := map[string]any{"url": "https://github.com"}
	status, body = httpPost(t, fmt.Sprintf("/instances/%s/navigate", instIDs[1]), nav2Payload)
	if status != 200 {
		t.Fatalf("navigate instance 2 failed: %d", status)
	}
	tabID2 := jsonField(t, body, "tabId")

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
	navPayload := map[string]any{"url": "https://example.com"}
	status, body = httpPost(t, fmt.Sprintf("/instances/%s/navigate", instID), navPayload)
	if status != 200 {
		t.Fatalf("proxy navigate failed: %d: %s", status, string(body))
	}

	tabID := jsonField(t, body, "tabId")
	if !strings.HasPrefix(tabID, "tab_") {
		t.Fatalf("invalid tab ID from proxy: %s", tabID)
	}

	// Test snapshot via orchestrator proxy
	status, body = httpGet(t, fmt.Sprintf("/instances/%s/snapshot", instID))
	if status != 200 {
		t.Fatalf("proxy snapshot failed: %d", status)
	}

	// Verify it's valid JSON with nodes
	var snapshot map[string]any
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("parse snapshot failed: %v", err)
	}
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
		navPayload := map[string]any{"url": "https://example.com"}
		httpPost(t, fmt.Sprintf("/instances/%s/navigate", instID), navPayload)
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
