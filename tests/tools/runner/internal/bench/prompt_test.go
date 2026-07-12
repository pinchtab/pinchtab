package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemPromptContainsRules(t *testing.T) {
	cfg := PromptConfig{
		RepoRoot:     "/tmp/test",
		ToolsDir:     "/tmp/test/tests/tools",
		BenchmarkDir: "/tmp/test/tests/benchmark",
	}

	for _, lane := range []Lane{LanePinchtab, LaneAgentBrowser} {
		t.Run(string(lane), func(t *testing.T) {
			lc := LanePromptConfig(lane, cfg)
			got := SystemPrompt(lc)
			if !strings.Contains(got, "benchmark execution agent") {
				t.Error("missing base prompt content")
			}
			if !strings.Contains(got, "Never fabricate") {
				t.Error("missing rules")
			}
		})
	}
}

func TestLaneSubsetInstructionsFull(t *testing.T) {
	got := LaneSubsetInstructions(nil)
	want := "Execute the full benchmark task set."
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestLaneSubsetInstructionsSubset(t *testing.T) {
	got := LaneSubsetInstructions([]int{0, 1, 2})
	if !strings.Contains(got, "Execute only these benchmark groups: 0, 1, 2.") {
		t.Errorf("missing group list in %q", got)
	}
	if !strings.Contains(got, "Do not attempt groups outside this subset.") {
		t.Errorf("missing subset constraint in %q", got)
	}
}

func TestLanePromptConfigAgent(t *testing.T) {
	cfg := PromptConfig{
		RepoRoot:     "/repo",
		ToolsDir:     "/repo/tests/tools",
		BenchmarkDir: "/repo/tests/benchmark",
	}
	lc := LanePromptConfig(LanePinchtab, cfg)
	if lc.Label != "PinchTab" {
		t.Errorf("Label = %q; want 'PinchTab'", lc.Label)
	}
	if lc.Wrapper != "./scripts/pt" {
		t.Errorf("Wrapper = %q; want './scripts/pt'", lc.Wrapper)
	}
}

func TestLanePromptConfigAgentBrowser(t *testing.T) {
	cfg := PromptConfig{
		RepoRoot:     "/repo",
		ToolsDir:     "/repo/tests/tools",
		BenchmarkDir: "/repo/tests/benchmark",
	}
	lc := LanePromptConfig(LaneAgentBrowser, cfg)
	if lc.Label != "agent-browser" {
		t.Errorf("Label = %q; want 'agent-browser'", lc.Label)
	}
	if lc.Wrapper != "./scripts/ab" {
		t.Errorf("Wrapper = %q; want './scripts/ab'", lc.Wrapper)
	}
}

func TestLaneUserPromptStructure(t *testing.T) {
	cfg := DefaultPromptConfig()
	reportFile := cfg.BenchmarkDir + "/results/test_report.json"

	prompt := LaneUserPrompt(LanePinchtab, cfg, reportFile, nil)

	checks := []string{
		"Benchmark lane: PinchTab execution.",
		"shell working directory is tests/tools/",
		"./scripts/pt",
		"runner step-end",
		"bootstrap command sequence",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt missing %q", check)
		}
	}
}

func TestBenchmarkRunGroupFile(t *testing.T) {
	got := BenchmarkRunGroupFile("/dir", 5)
	want := filepath.Join("/dir", "group-05.md")
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestBenchmarkRunAllGroups(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"group-00.md", "group-01.md", "group-10.md", "index.md"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o644)
	}
	groups := BenchmarkRunAllGroups(dir)
	if len(groups) != 3 {
		t.Errorf("got %d groups; want 3", len(groups))
	}
}
