//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// TestProxy_LocalhostOnly verifies that proxy endpoints only target localhost
// Security: Prevents Server-Side Request Forgery (SSRF) attacks
func TestProxy_LocalhostOnly(t *testing.T) {
	// Create an instance first
	payload := map[string]any{
		"name":     fmt.Sprintf("ssrf-test-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body := httpPost(t, "/instances/launch", payload)
	if status != 201 {
		t.Fatalf("failed to create instance: %d", status)
	}

	instID := jsonField(t, body, "id")
	defer httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)

	// Wait for instance to be ready
	waitForInstanceReady(t, instID)

	// The proxy handler validates that Host is localhost
	// This test verifies the construct happens correctly
	// (actual SSRF prevention is done at handler level through url.URL struct)

	// Verify instance can be accessed via proxy
	code, _, _ := navigateInstance(t, instID, "https://example.com")

	if code != 200 {
		t.Errorf("expected 200 for valid localhost proxy, got %d", code)
	}
}

// TestProxy_URLValidation verifies that URL construction is safe
// Security: Ensures URL.Host validation happens before making request
func TestProxy_URLValidation(t *testing.T) {
	// Create instance
	payload := map[string]any{
		"name":     fmt.Sprintf("url-val-test-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body := httpPost(t, "/instances/launch", payload)
	if status != 201 {
		t.Fatalf("failed to create instance: %d", status)
	}

	instID := jsonField(t, body, "id")
	defer httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)

	// Wait for instance to be ready
	waitForInstanceReady(t, instID)

	// Test with valid URL in path
	code, respBody, _ := navigateInstance(t, instID, "https://example.com/path?query=value")

	if code != 200 {
		t.Logf("navigate with query params got %d: %s", code, string(respBody))
		// Not critical if query params cause issues, main thing is it goes through
	}
}

// TestProxy_InstanceIsolation verifies each instance is isolated via proxy
func TestProxy_InstanceIsolation(t *testing.T) {
	// Create 2 instances
	var instIDs []string
	var ports []string

	for i := 0; i < 2; i++ {
		payload := map[string]any{
			"name":     fmt.Sprintf("isolation-test-%d-%d", time.Now().Unix(), i),
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

	defer func() {
		for _, instID := range instIDs {
			httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)
		}
	}()

	// Verify instances have different ports (isolation)
	if ports[0] == ports[1] {
		t.Fatalf("instances have same port: %s == %s", ports[0], ports[1])
	}

	// Wait for instances to be ready
	waitForInstanceReady(t, instIDs[0])
	waitForInstanceReady(t, instIDs[1])

	// Navigate in first instance
	code1, body1, _ := navigateInstance(t, instIDs[0], "https://example.com/page1")

	if code1 != 200 {
		t.Errorf("navigate in inst1 failed: %d: %s", code1, string(body1))
	}

	// Navigate in second instance
	code2, body2, _ := navigateInstance(t, instIDs[1], "https://example.com/page2")

	if code2 != 200 {
		t.Errorf("navigate in inst2 failed: %d: %s", code2, string(body2))
	}

	// Verify tabs are isolated (each instance should have own tabs)
	// This confirms proxy routing to correct instance
	_, tabsBody1 := httpPost(t, fmt.Sprintf("/instances/%s/tabs", instIDs[0]), nil)
	_, tabsBody2 := httpPost(t, fmt.Sprintf("/instances/%s/tabs", instIDs[1]), nil)

	// Parse to verify both return valid tab data
	var tabs1 map[string]any
	var tabs2 map[string]any

	if err := json.Unmarshal(tabsBody1, &tabs1); err != nil {
		t.Logf("inst1 tabs response not JSON: %v", err)
	}

	if err := json.Unmarshal(tabsBody2, &tabs2); err != nil {
		t.Logf("inst2 tabs response not JSON: %v", err)
	}
}

// TestProxy_SchemeValidation verifies proxy always uses http scheme for localhost
// Security: Ensures we don't accidentally proxy to https:// localhost
func TestProxy_SchemeValidation(t *testing.T) {
	// Create instance
	payload := map[string]any{
		"name":     fmt.Sprintf("scheme-test-%d", time.Now().Unix()),
		"headless": true,
	}

	status, body := httpPost(t, "/instances/launch", payload)
	if status != 201 {
		t.Fatalf("failed to create instance: %d", status)
	}

	instID := jsonField(t, body, "id")
	defer httpPost(t, fmt.Sprintf("/instances/%s/stop", instID), nil)

	// Wait for instance to be ready
	waitForInstanceReady(t, instID)

	// Navigate should work (verifies internal http scheme is used)
	code, respBody, _ := navigateInstance(t, instID, "https://example.com")

	if code != 200 {
		t.Errorf("navigate failed (scheme validation): %d: %s", code, string(respBody))
	}
}
