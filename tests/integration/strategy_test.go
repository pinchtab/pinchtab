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

// strategyServer manages a separate pinchtab process with a strategy configured.
type strategyServer struct {
	cmd     *exec.Cmd
	baseURL string
}

func startStrategyServer(t *testing.T, strategy, port string) *strategyServer {
	t.Helper()

	binary := "/tmp/pinchtab-test" // Built by TestMain
	if _, err := os.Stat(binary); err != nil {
		t.Skip("pinchtab binary not built — run full integration suite")
	}

	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(),
		"BRIDGE_PORT="+port,
		"BRIDGE_HEADLESS=true",
		"BRIDGE_NO_RESTORE=true",
		"PINCHTAB_STRATEGY="+strategy,
		"PINCHTAB_ALLOCATION_POLICY=fcfs",
		fmt.Sprintf("BRIDGE_STATE_DIR=%s", t.TempDir()),
		fmt.Sprintf("BRIDGE_PROFILE=%s", t.TempDir()),
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start strategy server: %v", err)
	}

	baseURL := fmt.Sprintf("http://localhost:%s", port)

	// Wait for health.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			_ = resp.Body.Close()
			return &strategyServer{cmd: cmd, baseURL: baseURL}
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}

	_ = cmd.Process.Kill()
	t.Fatal("strategy server did not become healthy within 30 seconds")
	return nil
}

func (s *strategyServer) stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- s.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = s.cmd.Process.Kill()
		}
	}
}

func (s *strategyServer) get(t *testing.T, path string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(s.baseURL + path)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func (s *strategyServer) post(t *testing.T, path string, payload any) (int, []byte) {
	t.Helper()
	data, _ := json.Marshal(payload)
	resp, err := http.Post(s.baseURL+path, "application/json", strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// TestStrategy_SimpleMode starts a separate server with PINCHTAB_STRATEGY=simple
// and verifies the shorthand endpoints work through the strategy layer.
func TestStrategy_SimpleMode(t *testing.T) {
	srv := startStrategyServer(t, "simple", "19877")
	defer srv.stop()

	// Health check should report strategy.
	code, body := srv.get(t, "/")
	if code != 200 {
		t.Fatalf("root health check failed: %d", code)
	}
	var health map[string]any
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if health["strategy"] != "simple" {
		t.Errorf("strategy = %v, want 'simple'", health["strategy"])
	}

	// Simple strategy auto-launches an instance. Wait for it to be ready.
	deadline := time.Now().Add(30 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		c, _ := srv.post(t, "/navigate", map[string]any{"url": "about:blank"})
		if c == 200 {
			ready = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !ready {
		t.Skip("auto-launched instance did not become ready in time")
	}

	// Navigate via shorthand (strategy-provided endpoint).
	code, body = srv.post(t, "/navigate", map[string]any{"url": "https://example.com"})
	if code != 200 {
		t.Fatalf("navigate failed: %d: %s", code, string(body))
	}
	t.Logf("navigate response: %s", string(body))

	// Snapshot via shorthand.
	code, body = srv.get(t, "/snapshot")
	if code != 200 {
		t.Fatalf("snapshot failed: %d: %s", code, string(body))
	}
	if len(body) < 10 {
		t.Error("snapshot response too short")
	}

	// Text via shorthand.
	code, body = srv.get(t, "/text")
	if code != 200 {
		t.Fatalf("text failed: %d: %s", code, string(body))
	}

	// List instances (available in all strategies).
	code, body = srv.get(t, "/instances")
	if code != 200 {
		t.Fatalf("list instances failed: %d", code)
	}

	// List tabs.
	code, body = srv.get(t, "/tabs")
	if code != 200 {
		t.Fatalf("list tabs failed: %d: %s", code, string(body))
	}
}
