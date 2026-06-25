package bench

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoopWithFakeRunner(t *testing.T) {
	requireUnixShell(t)
	dir := t.TempDir()
	reportFile := filepath.Join(dir, "report.json")
	commandLog := filepath.Join(dir, "commands.ndjson")

	_ = os.WriteFile(reportFile, []byte(`{"totals":{"steps_answered":0},"steps":[]}`), 0o644)

	responses := []FakeResponse{
		{
			ToolCalls: []ToolCall{
				{ID: "call_1", Command: "echo hello", TimeoutSeconds: 10},
			},
		},
		{
			FinalText: "Benchmark complete",
		},
	}

	runner := NewFakeRunner("fake-model", responses)
	shell, err := NewPersistentShell(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer shell.Close(false)

	var stdout, stderr bytes.Buffer
	cfg := LoopConfig{
		Lane:           LanePinchtab,
		Provider:       "fake",
		Model:          "fake-model",
		Groups:         []int{0},
		ReportFile:     reportFile,
		MaxTurns:       10,
		MaxIdleTurns:   5,
		TimeoutSeconds: 30,
		TurnDelayMs:    0,
		ToolsDir:       dir,
		BenchmarkDir:   dir,
		CommandLogFile: commandLog,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	result := RunLoop(cfg, runner, shell)

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", result.ExitCode)
	}
	if result.FinalText != "Benchmark complete" {
		t.Errorf("FinalText = %q; want 'Benchmark complete'", result.FinalText)
	}
	if result.Usage.RequestCount != 2 {
		t.Errorf("RequestCount = %d; want 2", result.Usage.RequestCount)
	}

	logData, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "echo hello") {
		t.Error("command log missing 'echo hello'")
	}

	output := stdout.String()
	if !strings.Contains(output, "Benchmark complete") {
		t.Errorf("stdout missing final text: %s", output)
	}
	if !strings.Contains(output, "[run-usage]") {
		t.Errorf("stdout missing usage line: %s", output)
	}
}

func TestLoopIdleTurnLimit(t *testing.T) {
	requireUnixShell(t)
	dir := t.TempDir()
	reportFile := filepath.Join(dir, "report.json")
	commandLog := filepath.Join(dir, "commands.ndjson")

	_ = os.WriteFile(reportFile, []byte(`{"totals":{"steps_answered":0},"steps":[]}`), 0o644)

	responses := make([]FakeResponse, 10)
	for i := range responses {
		responses[i] = FakeResponse{
			ToolCalls: []ToolCall{
				{ID: "call", Command: "echo noop", TimeoutSeconds: 10},
			},
		}
	}

	runner := NewFakeRunner("fake-model", responses)
	shell, err := NewPersistentShell(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer shell.Close(false)

	var stdout bytes.Buffer
	cfg := LoopConfig{
		Lane:           LanePinchtab,
		Provider:       "fake",
		Model:          "fake-model",
		ReportFile:     reportFile,
		MaxTurns:       20,
		MaxIdleTurns:   3,
		TimeoutSeconds: 30,
		ToolsDir:       dir,
		BenchmarkDir:   dir,
		CommandLogFile: commandLog,
		Stdout:         &stdout,
		Stderr:         &bytes.Buffer{},
	}

	result := RunLoop(cfg, runner, shell)

	if result.ExitCode != 3 {
		t.Errorf("ExitCode = %d; want 3 (idle limit)", result.ExitCode)
	}
	if !strings.Contains(result.FinalText, "consecutive turns") {
		t.Errorf("FinalText should mention consecutive turns: %s", result.FinalText)
	}
}

func TestLoopMaxTurns(t *testing.T) {
	requireUnixShell(t)
	dir := t.TempDir()
	reportFile := filepath.Join(dir, "report.json")

	_ = os.WriteFile(reportFile, []byte(`{"totals":{"steps_answered":0},"steps":[]}`), 0o644)

	responses := make([]FakeResponse, 100)
	for i := range responses {
		responses[i] = FakeResponse{
			ToolCalls: []ToolCall{
				{ID: "call", Command: "echo turn", TimeoutSeconds: 10},
			},
		}
	}

	runner := NewFakeRunner("fake-model", responses)
	shell, err := NewPersistentShell(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer shell.Close(false)

	var stdout bytes.Buffer
	cfg := LoopConfig{
		Lane:           LanePinchtab,
		Provider:       "fake",
		Model:          "fake-model",
		ReportFile:     reportFile,
		MaxTurns:       3,
		MaxIdleTurns:   100,
		TimeoutSeconds: 30,
		ToolsDir:       dir,
		BenchmarkDir:   dir,
		Stdout:         &stdout,
		Stderr:         &bytes.Buffer{},
	}

	result := RunLoop(cfg, runner, shell)

	if result.ExitCode != 2 {
		t.Errorf("ExitCode = %d; want 2 (max turns)", result.ExitCode)
	}
	if !strings.Contains(result.FinalText, "max turns") {
		t.Errorf("FinalText should mention max turns: %s", result.FinalText)
	}
}

func TestAppendCommandLog(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log.ndjson")

	appendCommandLog(logFile, "ls -la", 0, "output here")
	appendCommandLog(logFile, "cat file", 1, "error")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines; want 2", len(lines))
	}

	var entry map[string]interface{}
	_ = json.Unmarshal([]byte(lines[0]), &entry)
	if entry["command"] != "ls -la" {
		t.Errorf("first command = %v; want 'ls -la'", entry["command"])
	}
}

func TestLoopBudgetLimit(t *testing.T) {
	requireUnixShell(t)
	dir := t.TempDir()
	reportFile := filepath.Join(dir, "report.json")

	_ = os.WriteFile(reportFile, []byte(`{"totals":{"steps_answered":0},"steps":[]}`), 0o644)

	responses := make([]FakeResponse, 10)
	for i := range responses {
		responses[i] = FakeResponse{
			ToolCalls: []ToolCall{
				{ID: "call", Command: "echo test", TimeoutSeconds: 10},
			},
		}
	}

	runner := NewFakeRunner("fake-model", responses)
	shell, err := NewPersistentShell(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer shell.Close(false)

	var stdout bytes.Buffer
	cfg := LoopConfig{
		Lane:           LanePinchtab,
		Provider:       "fake",
		Model:          "fake-model",
		ReportFile:     reportFile,
		MaxTurns:       20,
		MaxIdleTurns:   100,
		TimeoutSeconds: 30,
		MaxInputTokens: 150,
		ToolsDir:       dir,
		BenchmarkDir:   dir,
		Stdout:         &stdout,
		Stderr:         &bytes.Buffer{},
	}

	result := RunLoop(cfg, runner, shell)

	if result.ExitCode != 4 {
		t.Errorf("ExitCode = %d; want 4 (budget exceeded)", result.ExitCode)
	}
	if !strings.Contains(result.FinalText, "Budget exceeded") {
		t.Errorf("FinalText should mention budget: %s", result.FinalText)
	}
}

func TestFakeRunner(t *testing.T) {
	responses := []FakeResponse{
		{ToolCalls: []ToolCall{{ID: "1", Command: "echo test"}}},
		{FinalText: "done"},
	}
	runner := NewFakeRunner("test-model", responses)

	if runner.Provider() != "fake" {
		t.Errorf("Provider = %s; want 'fake'", runner.Provider())
	}

	conv := runner.InitialConversation("hello")
	if len(conv) != 1 {
		t.Fatalf("initial conv length = %d; want 1", len(conv))
	}

	resp1, _ := runner.Send("system", conv)
	calls := runner.ExtractToolCalls(resp1, 60)
	if len(calls) != 1 {
		t.Fatalf("turn 1 calls = %d; want 1", len(calls))
	}

	resp2, _ := runner.Send("system", conv)
	text := runner.ExtractFinalText(resp2)
	if text != "done" {
		t.Errorf("final text = %q; want 'done'", text)
	}

	if runner.Usage().RequestCount != 2 {
		t.Errorf("RequestCount = %d; want 2", runner.Usage().RequestCount)
	}
}
