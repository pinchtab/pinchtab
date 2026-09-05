package bench

import (
	"context"
	"fmt"
	"github.com/pinchtab/pinchtab/internal/readiness"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func validateAPIKeys(provider string) error {
	if provider == "fake" {
		return nil
	}
	hasOpenAI := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != ""
	hasAnthropic := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != ""
	if !hasOpenAI && !hasAnthropic {
		return fmt.Errorf("OPENAI_API_KEY or ANTHROPIC_API_KEY is required")
	}
	return nil
}

func setupLaneContainer(lane Lane, benchDir string, stdout, stderr io.Writer) error {
	if lane == LanePinchtab {
		return setupPinchtabContainer(benchDir, stdout, stderr)
	}
	return setupAgentBrowserContainer(benchDir, stdout, stderr)
}

func setupPinchtabContainer(benchDir string, stdout, stderr io.Writer) error {
	if os.Getenv("BENCHMARK_SKIP_PINCHTAB_RESTART") != "" {
		_, _ = fmt.Fprintln(stdout, "Skipping pinchtab container setup (BENCHMARK_SKIP_PINCHTAB_RESTART=1)")
		return nil
	}

	_, _ = fmt.Fprintln(stdout, "Rebuilding pinchtab with pinchtab-benchmark.json (wrapContent=false)...")

	cmd := exec.Command("docker", "compose",
		"-f", "docker-compose.yml",
		"-f", "docker-compose.benchmark.yml",
		"up", "-d", "--build", "--force-recreate", "--no-deps", "pinchtab")
	cmd.Dir = benchDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to rebuild pinchtab: %w", err)
	}

	if err := waitForPinchtabHealth(30 * time.Second); err != nil {
		return err
	}

	if err := verifyPinchtabConfig(); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stdout, "Verified: pinchtab running with wrapContent=false")
	return nil
}

func setupAgentBrowserContainer(benchDir string, stdout, stderr io.Writer) error {
	if os.Getenv("BENCHMARK_SKIP_AGENT_BROWSER_RESTART") != "" {
		_, _ = fmt.Fprintln(stdout, "Skipping agent-browser container setup (BENCHMARK_SKIP_AGENT_BROWSER_RESTART=1)")
		return nil
	}

	_, _ = fmt.Fprintln(stdout, "Force-recreating agent-browser and fixtures...")

	cmd := exec.Command("docker", "compose",
		"up", "-d", "--build", "--force-recreate", "--no-deps", "fixtures", "agent-browser")
	cmd.Dir = benchDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to recreate agent-browser: %w", err)
	}

	if err := waitForFixturesReachable(benchDir, 30*time.Second); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stdout, "Verified: agent-browser up and fixtures reachable")
	return nil
}

func waitForPinchtabHealth(timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	_, err := readiness.WaitUntil(context.Background(), timeout, time.Second, func() (struct{}, bool, error) {
		req, err := http.NewRequest("GET", "http://localhost:9867/health", nil)
		if err != nil {
			return struct{}{}, false, err
		}
		req.Header.Set("Authorization", "Bearer benchmark-token")
		resp, err := client.Do(req)
		if err != nil {
			return struct{}{}, false, nil
		}
		_ = resp.Body.Close()
		return struct{}{}, resp.StatusCode == 200, nil
	})
	if err != nil {
		return fmt.Errorf("pinchtab health check timed out after %v", timeout)
	}
	return nil
}

func verifyPinchtabConfig() error {
	cmd := exec.Command("docker", "exec", "tools-pinchtab-1",
		"sh", "-c", `echo "${PINCHTAB_CONFIG:-}"`)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check pinchtab config: %w", err)
	}

	configPath := strings.TrimSpace(string(output))
	if configPath != "/fixture-config/pinchtab-benchmark.json" {
		return fmt.Errorf("pinchtab not running benchmark config (got %s)", configPath)
	}

	cmd = exec.Command("docker", "exec", "tools-pinchtab-1",
		"cat", "/fixture-config/pinchtab-benchmark.json")
	output, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to read pinchtab config: %w", err)
	}

	if !strings.Contains(string(output), `"wrapContent": false`) &&
		!strings.Contains(string(output), `"wrapContent":false`) {
		return fmt.Errorf("pinchtab config does not have wrapContent=false")
	}

	return nil
}

func waitForFixturesReachable(benchDir string, timeout time.Duration) error {
	_, err := readiness.WaitUntil(context.Background(), timeout, time.Second, func() (struct{}, bool, error) {
		cmd := exec.Command("docker", "compose", "exec", "-T", "agent-browser",
			"curl", "-sf", "http://fixtures/")
		cmd.Dir = benchDir
		return struct{}{}, cmd.Run() == nil, nil
	})
	if err != nil {
		return fmt.Errorf("fixtures not reachable from agent-browser after %v", timeout)
	}
	return nil
}

func initializeLaneGo(resultsDir string, lane Lane, provider, model string, stdout, stderr io.Writer) (string, error) {
	_ = os.MkdirAll(resultsDir, 0755)

	timestamp := time.Now().Format("20060102_150405")

	var reportFile string
	var reportType string
	if lane == LanePinchtab {
		reportFile = filepath.Join(resultsDir, fmt.Sprintf("pinchtab_benchmark_%s.json", timestamp))
		reportType = "pinchtab"
	} else {
		reportFile = filepath.Join(resultsDir, fmt.Sprintf("agent_browser_benchmark_%s.json", timestamp))
		reportType = "agent-browser"
	}

	runNumber := 1 // Could count existing reports if needed

	report := fmt.Sprintf(`{
  "benchmark": {
    "type": "%s",
    "run_number": %d,
    "timestamp": "%s",
    "model": "%s",
    "runner": "%s"
  },
  "totals": {
    "input_tokens": 0,
    "output_tokens": 0,
    "total_tokens": 0,
    "estimated_cost_usd": 0,
    "tool_calls": 0,
    "steps_passed": 0,
    "steps_failed": 0,
    "steps_skipped": 0,
    "steps_answered": 0,
    "steps_verified_passed": 0,
    "steps_verified_failed": 0,
    "steps_verified_skipped": 0,
    "steps_pending_verification": 0
  },
  "run_usage": {
    "source": "none",
    "provider": "",
    "request_count": 0,
    "input_tokens": 0,
    "output_tokens": 0,
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "total_input_tokens": 0,
    "total_tokens": 0
  },
  "steps": []
}`, reportType, runNumber, timestamp, model, runnerSource(provider))

	if err := os.WriteFile(reportFile, []byte(report), 0644); err != nil {
		return "", fmt.Errorf("failed to create report: %w", err)
	}

	ptrFile := filepath.Join(resultsDir, "current_pinchtab_report.txt")
	if lane == LaneAgentBrowser {
		ptrFile = filepath.Join(resultsDir, "current_agent_browser_report.txt")
	}
	if err := os.WriteFile(ptrFile, []byte(reportFile+"\n"), 0644); err != nil {
		return "", fmt.Errorf("failed to write pointer: %w", err)
	}

	// Clear timing state from prior runs
	key := strings.ReplaceAll(reportType, "_", "-")
	_ = os.Remove(filepath.Join(resultsDir, fmt.Sprintf("run_start_%s.ms", key)))
	_ = os.Remove(filepath.Join(resultsDir, fmt.Sprintf("last_step_end_%s.ms", key)))

	_, _ = fmt.Fprintf(stdout, "=== PinchTab Benchmark Run #%d ===\n", runNumber)
	_, _ = fmt.Fprintf(stdout, "Timestamp: %s\n", timestamp)
	_, _ = fmt.Fprintf(stdout, "Report: %s\n\n", reportFile)

	return reportFile, nil
}
