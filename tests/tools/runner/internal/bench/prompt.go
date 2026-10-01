package bench

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type PromptConfig struct {
	RepoRoot     string
	ToolsDir     string
	BenchmarkDir string
	TerseSummary bool
}

func DefaultPromptConfig() PromptConfig {
	cwd, _ := os.Getwd()
	repoRoot := findRepoRoot(cwd)
	return PromptConfig{
		RepoRoot:     repoRoot,
		ToolsDir:     filepath.Join(repoRoot, "tests", "tools"),
		BenchmarkDir: filepath.Join(repoRoot, "tests", "benchmark"),
	}
}

func findRepoRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}

type LaneConfig struct {
	Label             string
	Wrapper           string
	RecordType        string
	SkillFile         string
	SetupContent      string
	WorkflowSummary   []string
	AdapterNotes      []string
	BootstrapCommands []string
	ExampleStep       string
}

func workflowSummary(wrapper, snapCmd string) []string {
	return []string{
		fmt.Sprintf("use only %s for browser actions", wrapper),
		"keep the shared session across commands",
		fmt.Sprintf("use %s %s for actionable refs (compact interactive is default)", wrapper, snapCmd),
		"refresh refs after any navigation or DOM change",
		"STOP after each step completion and run ./scripts/runner step-end (records + verifies in one call) before continuing",
		"if 3 consecutive attempts show the same content or error, record the step as fail and move on",
	}
}

func adapterNotes() []string {
	return []string{
		"after navigation-triggering clicks, verify with a fresh snapshot",
		"do not assume every command returns JSON; parse only when the command actually returned JSON",
		`this environment uses BSD/macOS userland tools; avoid GNU-only flags such as "head -n -1"`,
	}
}

func LanePromptConfig(lane Lane, cfg PromptConfig) LaneConfig {
	repoRoot := cfg.RepoRoot
	if lane == LanePinchtab {
		wrapper := "./scripts/pt"
		return LaneConfig{
			Label:           "PinchTab",
			Wrapper:         wrapper,
			RecordType:      "pinchtab",
			SkillFile:       filepath.Join(repoRoot, "skills", "pinchtab", "SKILL.md"),
			SetupContent:    setupPinchtab,
			WorkflowSummary: workflowSummary(wrapper, "snap"),
			AdapterNotes:    adapterNotes(),
			BootstrapCommands: []string{
				wrapper + " nav http://fixtures/ --snap",
			},
			ExampleStep: fmt.Sprintf(`Step 0.1 (Server reachable):
  1. Run: %s health
  2. See output: {"status":"ok",...}
  3. Record + verify in one call: ./scripts/runner step-end 0 1 answer "status: ok" pass "health check passed"
  4. Move to step 0.2`, wrapper),
		}
	}
	wrapper := "./scripts/ab"
	return LaneConfig{
		Label:           "agent-browser",
		Wrapper:         wrapper,
		RecordType:      "agent-browser",
		SkillFile:       "", // Downloaded at runtime
		SetupContent:    setupAgentBrowser,
		WorkflowSummary: workflowSummary(wrapper, "snapshot"),
		AdapterNotes:    adapterNotes(),
		BootstrapCommands: []string{
			wrapper + " open http://fixtures/",
			wrapper + " snapshot -i -c",
		},
		ExampleStep: fmt.Sprintf(`Step 0.1 (Open fixtures home):
  1. Run: %s open http://fixtures/
  2. See output: page loaded successfully
  3. Record + verify in one call: ./scripts/runner step-end 0 1 answer "page opened with title Home" pass "confirmed page loaded"
  4. Move to step 0.2`, wrapper),
	}
}

func SystemPrompt(lc LaneConfig) string {
	base := `You are a precise benchmark execution agent. Use tools to inspect the repo and run the benchmark lane exactly as instructed.

Rules:
- Never fabricate command output or task results.
- Use the shell tool for all file reads and command execution.
- Do not use destructive commands such as rm -rf, git reset, or checkout.
- After recording an answer, verify it immediately against the task oracle.
- Prefer factual command output over long reasoning.`

	parts := []string{base}
	if skill := LoadSkillContent(lc.SkillFile); skill != "" {
		parts = append(parts, skill)
	}
	if setup := strings.TrimSpace(lc.SetupContent); setup != "" {
		// The setup file is inlined here so it lives in the cached prefix and
		// the agent does not spend turns running `cat setup-*.md`. Keep this
		// in sync with LaneUserPrompt, which now tells the agent the setup is
		// already in context rather than asking it to read the file.
		parts = append(parts, "# Lane Setup (already loaded — do not re-read)\n\n"+setup)
	}
	return strings.Join(parts, "\n\n")
}

func LoadSkillContent(skillFile string) string {
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return ""
	}
	return stripFrontmatter(string(data))
}

func stripFrontmatter(content string) string {
	if strings.HasPrefix(content, "---") {
		if idx := strings.Index(content[3:], "---"); idx >= 0 {
			return strings.TrimSpace(content[idx+6:])
		}
	}
	return content
}

// DownloadAgentBrowserSkill refreshes skills/agent-browser/SKILL.md from the
// live `agent-browser skills get agent-browser --full` CLI output every time
// the lane starts. The file is overwritten on every run so drift between the
// checked-in copy and the installed CLI shows up as a git diff.
//
// Only the `--- references/commands.md ---` section is kept. Other sections
// (snapshot-refs, authentication, session-management, templates, etc.) are
// dropped so the agent-browser skill stays byte-close to
// skills/pinchtab/SKILL.md (~14 KB), preserving the benchmark's cost-fairness
// assumption between lanes.
func DownloadAgentBrowserSkill(repoRoot, toolsDir string) (string, error) {
	skillDir := filepath.Join(repoRoot, "skills", "agent-browser")
	skillFile := filepath.Join(skillDir, "SKILL.md")

	wrapper := filepath.Join(toolsDir, "scripts", "ab")
	cmd := exec.Command(wrapper, "skills", "get", "agent-browser", "--full") // #nosec G204 -- wrapper path is constructed from known toolsDir
	cmd.Dir = toolsDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("download skill: %w", err)
	}

	// Commands-only extraction. If the CLI ever stops emitting a
	// `--- references/commands.md ---` section, the result is empty and
	// we fail loudly rather than silently writing a blank skill file.
	content := string(output)
	extracted := extractSkillSections(content, []string{
		"references/commands.md",
	})
	if strings.TrimSpace(extracted) == "" {
		return "", fmt.Errorf("download skill: no `--- references/commands.md ---` section in CLI output")
	}

	banner := "<!--\n" +
		"  GENERATED FILE — do not edit by hand.\n" +
		"  Regenerated by tests/tools/runner (DownloadAgentBrowserSkill)\n" +
		"  from `agent-browser skills get agent-browser --full`.\n" +
		"  Only the commands section is kept; other references are dropped\n" +
		"  to preserve cost-fairness parity with skills/pinchtab/SKILL.md.\n" +
		"-->\n\n"

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("create skill dir: %w", err)
	}

	if err := os.WriteFile(skillFile, []byte(banner+extracted), 0644); err != nil {
		return "", fmt.Errorf("write skill: %w", err)
	}

	return skillFile, nil
}

func extractSkillSections(content string, includeSections []string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inHeader := true
	inIncludedSection := false
	currentSection := ""

	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") && strings.HasSuffix(line, " ---") {
			inHeader = false
			currentSection = strings.TrimPrefix(strings.TrimSuffix(line, " ---"), "--- ")
			inIncludedSection = false
			if slices.Contains(includeSections, currentSection) {
				inIncludedSection = true
				result = append(result, "", line)
			}
			continue
		}

		if inHeader || inIncludedSection {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

func LaneSubsetInstructions(groups []int) string {
	if len(groups) == 0 {
		return "Execute the full benchmark task set."
	}
	parts := make([]string, len(groups))
	for i, g := range groups {
		parts[i] = fmt.Sprintf("%d", g)
	}
	return fmt.Sprintf(`Execute only these benchmark groups: %s.
Do not attempt groups outside this subset.
Treat all other groups as out of scope for this run rather than as failures.
For the selected groups, execute every step in the group unless blocked.`, strings.Join(parts, ", "))
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func BenchmarkRunGroupFile(dir string, group int) string {
	return filepath.Join(dir, fmt.Sprintf("group-%02d.md", group))
}

func BenchmarkRunAllGroups(dir string) []int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var groups []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if m := groupFileRegex.FindStringSubmatch(e.Name()); m != nil {
			var n int
			_, _ = fmt.Sscanf(m[1], "%d", &n)
			groups = append(groups, n)
		}
	}
	return groups
}

func BenchmarkRunSelectedText(cfg PromptConfig, groups []int) string {
	indexPath := filepath.Join(cfg.BenchmarkDir, "index.md")
	selected := groups
	if len(selected) == 0 {
		selected = BenchmarkRunAllGroups(cfg.BenchmarkDir)
	}
	chunks := []string{readText(indexPath)}
	for _, g := range selected {
		path := BenchmarkRunGroupFile(cfg.BenchmarkDir, g)
		if content := readText(path); content != "" {
			chunks = append(chunks, content)
		}
	}
	return strings.Join(chunks, "\n\n")
}

func LaneTaskSourceInstructions(lane Lane, cfg PromptConfig, groups []int) string {
	indexPath := filepath.Join(cfg.BenchmarkDir, "index.md")

	if len(groups) == 0 {
		return fmt.Sprintf("Use the benchmark run index at %s plus all group files in %s.\n\n%s",
			indexPath, cfg.BenchmarkDir, BenchmarkRunSelectedText(cfg, nil))
	}

	if lane == LanePinchtab {
		subset := BenchmarkRunSelectedText(cfg, groups)
		return fmt.Sprintf("Use only this selected task subset from %s:\n\n%s", cfg.BenchmarkDir, subset)
	}

	var sections []string
	for _, g := range groups {
		path := BenchmarkRunGroupFile(cfg.BenchmarkDir, g)
		if content := readText(path); content != "" {
			sections = append(sections, content)
		}
	}

	return fmt.Sprintf("Use only this selected task subset for agent-browser: Group 0 (embedded), Groups 1+ from %s.\n\n%s\n\n%s",
		cfg.BenchmarkDir, readText(indexPath), strings.Join(sections, "\n\n"))
}

func LaneUserPrompt(lane Lane, cfg PromptConfig, reportFile string, groups []int) string {
	lc := LanePromptConfig(lane, cfg)
	firstGroup := 0
	if len(groups) > 0 {
		firstGroup = groups[0]
	}

	var bootstrapLines []string
	for i, cmd := range lc.BootstrapCommands {
		bootstrapLines = append(bootstrapLines, fmt.Sprintf("%d. %s", i+1, cmd))
	}
	bootstrap := strings.Join(bootstrapLines, "\n")

	taskInstr := LaneTaskSourceInstructions(lane, cfg, groups)
	taskInstr = strings.ReplaceAll(taskInstr, "\n", "\n  ")

	subsetInstr := LaneSubsetInstructions(groups)
	subsetInstr = strings.ReplaceAll(subsetInstr, "\n", "\n  - ")

	// Final-answer instruction. For benchmark runs (terse mode) we skip this
	// entirely — the "STOP immediately" instruction above is enough and the
	// verbose prohibition list was confusing the agent into looking for reports.
	// For optimization runs the prose summary is useful to a human reading the log.
	finalAnswerInstr := ""
	if !cfg.TerseSummary {
		finalAnswerInstr = "Your final answer should briefly summarize completion status and the main blockers, if any."
	}

	return fmt.Sprintf(`Benchmark lane: %s execution.
Your shell working directory is tests/tools/ within the repo. Commands like ./scripts/pt work directly.

Requirements:
- Follow a linear execution flow: execute, record, verify, continue.
- The lane setup notes are already inlined in the system prompt under "Lane Setup". Do NOT cat any setup files — their contents are already in context.
- The CLI skill and the selected task groups are also already in context. Do not cat index.md or group-*.md.
- Do not read README, browse directories, or inspect unrelated files unless a command path is missing.
- Tool wrapper:
  - %s
- Workflow summary:
  - %s
- Adapter notes:
  - %s
- Task scope:
  %s
- For each completed step you MUST record and verify immediately, in a single call:
     ./scripts/runner step-end <group> <step> answer "<what you saw>" <pass|fail|skip> "verification notes"
- CRITICAL: After EVERY action that completes a step, run step-end.sh before doing anything else.
- If a step cannot be completed, record fail or skip in the same.
- Do not leave answered steps pending verification.
- Example flow:
%s
- Keep commands concise. Prefer rg/sed/cat only when you must inspect a specific file.
- Start with this bootstrap command sequence before attempting the selected steps:
%s
- After the bootstrap, immediately execute Group %d step 1.
- Subset selection:
  - %s
- Finish when all selected steps are executed or when you are blocked. After the last step-end, STOP immediately — do not search for reports, run status commands, or read files.

%s`,
		lc.Label,
		lc.Wrapper,
		strings.Join(lc.WorkflowSummary, "\n  - "),
		strings.Join(lc.AdapterNotes, "\n  - "),
		taskInstr,
		lc.ExampleStep,
		bootstrap,
		firstGroup,
		subsetInstr,
		finalAnswerInstr)
}
