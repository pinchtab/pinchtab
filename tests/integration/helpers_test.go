//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// httpPostWithRetry attempts a POST request with retries for flaky tests
func httpPostWithRetry(t *testing.T, path string, body any, maxRetries int) (int, []byte) {
	t.Helper()

	var lastCode int
	var lastBody []byte
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			t.Logf("Retry %d/%d for %s", i, maxRetries, path)
			time.Sleep(2 * time.Second) // Wait before retry
		}

		code, respBody, err := doPost(serverURL+path, body)
		lastCode = code
		lastBody = respBody
		lastErr = err

		if err == nil && code == 200 {
			return code, respBody
		}

		// Log the failure for debugging
		if err != nil {
			t.Logf("Request failed with error: %v", err)
		} else {
			t.Logf("Request failed with status %d: %s", code, string(respBody))
		}
	}

	// If all retries failed, use the original httpPost behavior
	if lastErr != nil {
		t.Fatalf("http post %s: %v", path, lastErr)
	}
	return lastCode, lastBody
}

// doPost performs the actual HTTP POST
func doPost(url string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		reader = strings.NewReader(string(jsonBody))
	}

	req, err := http.NewRequest("POST", url, reader)
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if token := getAuthToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{
		Timeout: 45 * time.Second, // Increase timeout for Chrome startup
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

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
