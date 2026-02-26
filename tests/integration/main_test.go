//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

var serverURL string

// removeEnvPrefix removes all environment variables starting with the given prefix
func removeEnvPrefix(env []string, prefix string) []string {
	result := []string{}
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

func TestMain(m *testing.M) {
	port := os.Getenv("PINCHTAB_TEST_PORT")
	if port == "" {
		port = "19867"
	}
	serverURL = fmt.Sprintf("http://localhost:%s", port)

	// Build the binary
	build := exec.Command("go", "build", "-o", "/tmp/pinchtab-test", "./cmd/pinchtab/")
	build.Dir = findRepoRoot()
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pinchtab: %v\n", err)
		os.Exit(1)
	}

	// Start server
	cmd := exec.Command("/tmp/pinchtab-test")

	// Build environment for subprocess
	// Start with a filtered set of inherited env vars, then add test-specific ones
	baseEnv := os.Environ()
	env := []string{}
	for _, e := range baseEnv {
		// Skip any pre-existing BRIDGE_* and PINCHTAB_* vars to avoid conflicts
		if !strings.HasPrefix(e, "BRIDGE_") && !strings.HasPrefix(e, "PINCHTAB_") {
			env = append(env, e)
		}
	}

	// Add test-specific environment
	env = append(env,
		"BRIDGE_PORT="+port,
		"BRIDGE_HEADLESS=true",
		"BRIDGE_NO_RESTORE=true",
		"BRIDGE_STEALTH=light",
		fmt.Sprintf("BRIDGE_STATE_DIR=%s", mustTempDir()),
		fmt.Sprintf("BRIDGE_PROFILE=%s", mustTempDir()),
	)

	// Pass CHROME_BINARY if set by CI workflow or environment
	if chromeBinary := os.Getenv("CHROME_BINARY"); chromeBinary != "" {
		env = append(env, "CHROME_BINARY="+chromeBinary)
	}

	// Pass BRIDGE_CHROME_VERSION if TEST_CHROME_VERSION is set
	if testChromeVersion := os.Getenv("TEST_CHROME_VERSION"); testChromeVersion != "" {
		fmt.Fprintf(os.Stderr, "TestMain: setting BRIDGE_CHROME_VERSION=%s from TEST_CHROME_VERSION\n", testChromeVersion)
		env = append(env, "BRIDGE_CHROME_VERSION="+testChromeVersion)
	}

	// Pass BRIDGE_PROFILE if TEST_PROFILE_DIR is set
	if testProfileDir := os.Getenv("TEST_PROFILE_DIR"); testProfileDir != "" {
		fmt.Fprintf(os.Stderr, "TestMain: setting BRIDGE_PROFILE=%s from TEST_PROFILE_DIR\n", testProfileDir)
		// Remove the default BRIDGE_PROFILE and use the test-specified one
		env = removeEnvPrefix(env, "BRIDGE_PROFILE=")
		env = append(env, "BRIDGE_PROFILE="+testProfileDir)
	}

	// Handle BRIDGE_NO_RESTORE - only disable if TEST_NO_RESTORE is explicitly set
	if testNoRestore := os.Getenv("TEST_NO_RESTORE"); testNoRestore != "" {
		fmt.Fprintf(os.Stderr, "TestMain: setting BRIDGE_NO_RESTORE=%s from TEST_NO_RESTORE\n", testNoRestore)
		env = removeEnvPrefix(env, "BRIDGE_NO_RESTORE=")
		env = append(env, "BRIDGE_NO_RESTORE="+testNoRestore)
	}

	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start pinchtab: %v\n", err)
		os.Exit(1)
	}

	// Wait for server to be ready (longer timeout in CI)
	healthTimeout := 30 * time.Second
	if os.Getenv("CI") == "true" {
		healthTimeout = 60 * time.Second
	}
	if !waitForHealth(serverURL, healthTimeout) {
		fmt.Fprintf(os.Stderr, "pinchtab did not become healthy within timeout (%v)\n", healthTimeout)
		_ = cmd.Process.Kill()
		os.Exit(1)
	}

	code := m.Run()

	// Shutdown
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
	}

	os.Exit(code)
}

func findRepoRoot() string {
	// Walk up from test dir to find go.mod
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			break
		}
		dir = parent
	}
	// fallback
	return "../.."
}

func mustTempDir() string {
	d, err := os.MkdirTemp("", "pinchtab-test-*")
	if err != nil {
		panic(err)
	}
	return d
}

func waitForHealth(base string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/health")
		if err == nil && resp.StatusCode == 200 {
			_ = resp.Body.Close()
			return true
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// helpers

func httpGet(t *testing.T, path string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(serverURL + path)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func httpPost(t *testing.T, path string, payload any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		reader = strings.NewReader(string(data))
	}
	resp, err := http.Post(serverURL+path, "application/json", reader)
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func httpPostRaw(t *testing.T, path string, body string) (int, []byte) {
	t.Helper()
	resp, err := http.Post(serverURL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func jsonField(t *testing.T, data []byte, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json parse failed: %v (body: %s)", err, string(data))
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}

func navigate(t *testing.T, url string) {
	t.Helper()
	// Use retry logic for better stability
	code, body := httpPostWithRetry(t, "/navigate", map[string]any{"url": url}, 2)
	if code != 200 {
		t.Fatalf("navigate to %s failed with %d: %s", url, code, string(body))
	}
}
