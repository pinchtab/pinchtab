//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// requireOrchestrator skips the test if the orchestrator is not reachable.
func requireOrchestrator(t *testing.T) {
	t.Helper()
	url := serverURL
	if url == "" {
		url = "http://localhost:9867"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/health", url))
	if err != nil {
		t.Skipf("Orchestrator not reachable at %s (skipping): %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
}

// httpPostWithRetry attempts a POST request with retries for flaky tests.
// Returns (statusCode, responseBody) matching httpPost signature.
func httpPostWithRetry(t *testing.T, path string, body map[string]any, maxRetries int) (int, []byte) {
	t.Helper()

	var lastCode int
	var lastBody []byte
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			t.Logf("Retry %d/%d for %s", i, maxRetries, path)
			time.Sleep(2 * time.Second)
		}

		code, respBody, err := doPost(serverURL+path, body)
		lastCode = code
		lastBody = respBody
		lastErr = err

		if err == nil && code == 200 {
			return code, respBody
		}

		// Integration suites can leak tabs across scenarios. If Chrome reports
		// max tabs reached, aggressively clean up and retry.
		if err == nil && strings.Contains(string(respBody), "tab limit reached") {
			t.Logf("Detected tab limit on %s, closing all tabs before retry", path)
			closeAllTabs(t)
		}

		if err != nil {
			t.Logf("Request failed with error: %v", err)
		} else {
			t.Logf("Request failed with status %d: %s", code, string(respBody))
		}
	}

	if lastErr != nil {
		t.Fatalf("http post %s: %v", path, lastErr)
	}
	return lastCode, lastBody
}

// doPost performs the actual HTTP POST.
func doPost(url string, body map[string]any) (int, []byte, error) {
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if token := getAuthToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{
		Timeout: 45 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	return resp.StatusCode, respBody, nil
}

// getAuthToken returns the auth token if configured
func getAuthToken() string {
	// This should match the token logic in main_test.go
	return "test-token"
}
