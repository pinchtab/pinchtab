package bench

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const defaultBrowserBenchCSV = "https://raw.githubusercontent.com/Halluminate/browserbench/main/browserbench.csv"

var errBrowserBenchHelp = errors.New("browserbench help")

type BrowserBenchArgs struct {
	Provider        Provider
	Model           string
	CSVFile         string
	Output          string
	Tasks           int
	TaskID          string
	SkipInit        bool
	Verbose         bool
	MaxTokens       int
	Temperature     float64
	MaxTurns        int
	TimeoutSeconds  int
	TurnDelayMs     int
	MaxInputTokens  int
	MaxOutputTokens int
}

type BrowserBenchTask struct {
	TaskID          string
	StartingURL     string
	TaskDescription string
	GroundTruthURL  string
	GroundTruth     string
}

type BrowserBenchRow struct {
	TaskID                   string
	StartingURL              string
	TaskDescription          string
	GroundTruthURL           string
	GroundTruth              string
	Provider                 string
	Timestamp                string
	Success                  bool
	ErrorMessage             string
	AgentResult              string
	SessionURL               string
	ExecutionTimeSeconds     string
	PinchTabVersion          string
	StealthLevel             string
	SolverUsed               string
	SolveAttempts            string
	HARPath                  string
	ScreenshotPath           string
	ConsoleLogPath           string
	CommandLogPath           string
	RequestCount             string
	InputTokens              string
	OutputTokens             string
	CacheCreationInputTokens string
	CacheReadInputTokens     string
	TotalInputTokens         string
	TotalTokens              string
}

func defaultBrowserBenchArgs() BrowserBenchArgs {
	return BrowserBenchArgs{
		CSVFile:         defaultBrowserBenchCSV,
		MaxTokens:       defaultMaxTokens,
		Temperature:     defaultTemperature,
		MaxTurns:        80,
		TimeoutSeconds:  defaultTimeoutSeconds,
		TurnDelayMs:     750,
		MaxInputTokens:  0,
		MaxOutputTokens: 0,
	}
}

func parseBrowserBenchArgs(argv []string) (BrowserBenchArgs, error) {
	a := defaultBrowserBenchArgs()
	next := func(i *int, name string) (string, error) {
		*i++
		if *i >= len(argv) {
			return "", fmt.Errorf("%s requires a value", name)
		}
		return argv[*i], nil
	}

	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--help", "-h":
			return a, errBrowserBenchHelp
		case "--provider":
			if err := parseStringFlag(next, &i, argv[i], func(v string) { a.Provider = Provider(v) }); err != nil {
				return a, err
			}
		case "--model":
			if err := parseStringFlag(next, &i, argv[i], func(v string) { a.Model = v }); err != nil {
				return a, err
			}
		case "--csv-file":
			if err := parseStringFlag(next, &i, argv[i], func(v string) { a.CSVFile = v }); err != nil {
				return a, err
			}
		case "--output":
			if err := parseStringFlag(next, &i, argv[i], func(v string) { a.Output = v }); err != nil {
				return a, err
			}
		case "--tasks":
			if err := parseIntFlag(next, &i, argv[i], func(n int) { a.Tasks = n }); err != nil {
				return a, err
			}
		case "--task-id":
			if err := parseStringFlag(next, &i, argv[i], func(v string) { a.TaskID = v }); err != nil {
				return a, err
			}
		case "--skip-init":
			a.SkipInit = true
		case "--verbose", "-v":
			a.Verbose = true
		case "--max-tokens":
			if err := parseIntFlag(next, &i, argv[i], func(n int) { a.MaxTokens = n }); err != nil {
				return a, err
			}
		case "--temperature":
			if err := parseFloatFlag(next, &i, argv[i], func(f float64) { a.Temperature = f }); err != nil {
				return a, err
			}
		case "--max-turns":
			if err := parseIntFlag(next, &i, argv[i], func(n int) { a.MaxTurns = n }); err != nil {
				return a, err
			}
		case "--timeout-seconds":
			if err := parseIntFlag(next, &i, argv[i], func(n int) { a.TimeoutSeconds = n }); err != nil {
				return a, err
			}
		case "--turn-delay-ms":
			if err := parseIntFlag(next, &i, argv[i], func(n int) { a.TurnDelayMs = n }); err != nil {
				return a, err
			}
		case "--max-input-tokens":
			if err := parseIntFlag(next, &i, argv[i], func(n int) { a.MaxInputTokens = n }); err != nil {
				return a, err
			}
		case "--max-output-tokens":
			if err := parseIntFlag(next, &i, argv[i], func(n int) { a.MaxOutputTokens = n }); err != nil {
				return a, err
			}
		default:
			return a, fmt.Errorf("unknown flag %q", argv[i])
		}
	}

	if a.Tasks < 0 {
		return a, fmt.Errorf("--tasks must be >= 0")
	}
	return a, nil
}

func writeBrowserBenchUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  runner browserbench [options]

Options:
`+usageLineProvider+
		usageLineModel+
		`  --csv-file PATH_OR_URL        BrowserBench CSV (default: Halluminate raw CSV)
  --output PATH                 Output CSV path (default: timestamped in tests/tools/browserbench/results)
  --tasks N                     Limit number of tasks
  --task-id ID                  Run only one task_id
  --skip-init                   Reuse existing benchmark container
`+usageLineMaxTokens+
		usageLineTemperature+
		usageLineMaxTurns+
		usageLineTimeoutSeconds+
		usageLineTurnDelayMs+
		`  --max-input-tokens N
  --max-output-tokens N
  --verbose, -v

Example:
  go run ./tests/tools/runner browserbench --provider anthropic --tasks 5 --verbose
`)
}

func RunBrowserBench(argv []string, stdout, stderr io.Writer) int {
	args, err := parseBrowserBenchArgs(argv)
	if err != nil {
		if errors.Is(err, errBrowserBenchHelp) {
			writeBrowserBenchUsage(stdout)
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "runner browserbench: %v\n\n", err)
		writeBrowserBenchUsage(stderr)
		return 1
	}

	provider := resolveProvider(Args{Provider: args.Provider})
	if err := validateAPIKeys(provider); err != nil {
		_, _ = fmt.Fprintf(stderr, "runner browserbench: %v\n", err)
		return 1
	}
	model := resolveModel(provider, args.Model)
	toolsDir := resolveToolsDir()

	if !args.SkipInit {
		if err := setupPinchtabContainer(toolsDir, stdout, stderr); err != nil {
			_, _ = fmt.Fprintf(stderr, "runner browserbench: %v\n", err)
			return 1
		}
	}

	tasks, err := loadBrowserBenchTasks(args.CSVFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "runner browserbench: load CSV: %v\n", err)
		return 1
	}
	tasks = selectBrowserBenchTasks(tasks, args.TaskID, args.Tasks)
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(stderr, "runner browserbench: no tasks selected")
		return 1
	}

	resultsDir := filepath.Join(toolsDir, "browserbench", "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		_, _ = fmt.Fprintf(stderr, "runner browserbench: create results dir: %v\n", err)
		return 1
	}

	outputPath := args.Output
	if outputPath == "" {
		outputPath = filepath.Join(resultsDir, fmt.Sprintf("pinchtab_browserbench_%s.csv", time.Now().Format("20060102_150405")))
	}
	runDir := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		_, _ = fmt.Fprintf(stderr, "runner browserbench: create run dir: %v\n", err)
		return 1
	}

	shell, err := NewPersistentShell(toolsDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "runner browserbench: shell init failed: %v\n", err)
		return 1
	}
	defer shell.Close(false)

	pinchtabVersion := detectPinchTabVersion(shell)
	rows := make([]BrowserBenchRow, 0, len(tasks))

	for i, task := range tasks {
		_, _ = fmt.Fprintf(stdout, "[browserbench] task %d/%d id=%s url=%s\n", i+1, len(tasks), task.TaskID, task.StartingURL)
		row, err := runBrowserBenchTask(args, provider, model, task, runDir, shell, pinchtabVersion, stdout, stderr)
		if err != nil {
			row.ErrorMessage = err.Error()
		}
		rows = append(rows, row)
		if err := writeBrowserBenchCSV(outputPath, rows); err != nil {
			_, _ = fmt.Fprintf(stderr, "runner browserbench: write CSV: %v\n", err)
			return 1
		}
	}

	passed := 0
	for _, row := range rows {
		if row.Success {
			passed++
		}
	}
	_, _ = fmt.Fprintf(stdout, "[browserbench] wrote %s (%d/%d passed)\n", outputPath, passed, len(rows))
	return 0
}

func loadBrowserBenchTasks(path string) ([]BrowserBenchTask, error) {
	data, err := readFileOrURL(path)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(strings.NewReader(string(data)))
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV has no data rows")
	}
	headers := make(map[string]int)
	for i, h := range records[0] {
		headers[strings.TrimSpace(strings.ToLower(h))] = i
	}
	required := []string{"task_id", "starting_url", "task_description", "ground_truth_url", "ground_truth"}
	for _, key := range required {
		if _, ok := headers[key]; !ok {
			return nil, fmt.Errorf("missing required column %q", key)
		}
	}
	var tasks []BrowserBenchTask
	for _, rec := range records[1:] {
		if len(rec) == 0 {
			continue
		}
		tasks = append(tasks, BrowserBenchTask{
			TaskID:          rec[headers["task_id"]],
			StartingURL:     rec[headers["starting_url"]],
			TaskDescription: rec[headers["task_description"]],
			GroundTruthURL:  rec[headers["ground_truth_url"]],
			GroundTruth:     rec[headers["ground_truth"]],
		})
	}
	return tasks, nil
}

func selectBrowserBenchTasks(tasks []BrowserBenchTask, taskID string, limit int) []BrowserBenchTask {
	selected := make([]BrowserBenchTask, 0, len(tasks))
	for _, task := range tasks {
		if taskID != "" && task.TaskID != taskID {
			continue
		}
		selected = append(selected, task)
		if limit > 0 && len(selected) >= limit {
			break
		}
	}
	return selected
}

func runBrowserBenchTask(args BrowserBenchArgs, provider, model string, task BrowserBenchTask, runDir string, shell *PersistentShell, pinchtabVersion string, stdout, stderr io.Writer) (BrowserBenchRow, error) {
	row := BrowserBenchRow{
		TaskID:          task.TaskID,
		StartingURL:     task.StartingURL,
		TaskDescription: task.TaskDescription,
		GroundTruthURL:  task.GroundTruthURL,
		GroundTruth:     task.GroundTruth,
		Provider:        provider,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		PinchTabVersion: pinchtabVersion,
	}

	taskDir := filepath.Join(runDir, fmt.Sprintf("task-%s", sanitizeTaskID(task.TaskID)))
	artifactsDir := filepath.Join(taskDir, "artifacts")
	_ = os.MkdirAll(artifactsDir, 0o755)
	commandLogPath := filepath.Join(taskDir, "commands.ndjson")
	row.CommandLogPath = commandLogPath

	if _, _, err := shell.Run(fmt.Sprintf("mkdir -p %q && ./scripts/pt console --clear >/dev/null 2>&1 || true", artifactsDir), 30*time.Second); err != nil {
		return row, err
	}
	_, _, _ = shell.Run(`curl -sS -X POST -H 'Authorization: Bearer benchmark-token' http://localhost:9867/network/clear >/dev/null 2>&1 || true`, 15*time.Second)
	if _, _, err := shell.Run(fmt.Sprintf("./scripts/pt nav %q >/dev/null", task.StartingURL), 45*time.Second); err != nil {
		return row, fmt.Errorf("pre-nav failed: %w", err)
	}

	runner := createRunner(provider, model, Args{
		Provider:        Provider(provider),
		Model:           model,
		MaxTokens:       args.MaxTokens,
		Temperature:     args.Temperature,
		MaxInputTokens:  args.MaxInputTokens,
		MaxOutputTokens: args.MaxOutputTokens,
	})

	before := runner.Usage()
	start := time.Now()
	finalText, loopErr := runBrowserBenchAgentLoop(task, provider, shell, runner, args, commandLogPath, stdout)
	elapsed := time.Since(start)
	after := runner.Usage()
	usage := diffUsage(after, before)

	row.AgentResult = extractFinalAnswer(finalText)
	row.ExecutionTimeSeconds = fmt.Sprintf("%.3f", elapsed.Seconds())
	row.RequestCount = strconv.Itoa(usage.RequestCount)
	row.InputTokens = strconv.Itoa(usage.InputTokens)
	row.OutputTokens = strconv.Itoa(usage.OutputTokens)
	row.CacheCreationInputTokens = strconv.Itoa(usage.CacheCreationInputTokens)
	row.CacheReadInputTokens = strconv.Itoa(usage.CacheReadInputTokens)
	row.TotalInputTokens = strconv.Itoa(usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens)
	row.TotalTokens = strconv.Itoa(usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens + usage.OutputTokens)
	if loopErr != nil {
		row.ErrorMessage = loopErr.Error()
	}

	stealthPath := filepath.Join(artifactsDir, "stealth-status.json")
	autosolverPath := filepath.Join(artifactsDir, "autosolver.json")
	screenshotPath := filepath.Join(artifactsDir, "final.png")
	harPath := filepath.Join(artifactsDir, "network.har")
	consolePath := filepath.Join(artifactsDir, "console.log")
	row.ScreenshotPath = screenshotPath
	row.HARPath = harPath
	row.ConsoleLogPath = consolePath

	_, _, _ = shell.Run(fmt.Sprintf("curl -sS -H 'Authorization: Bearer benchmark-token' http://localhost:9867/stealth/status > %q || true", stealthPath), 15*time.Second)
	_, _, _ = shell.Run(fmt.Sprintf("curl -sS -H 'Authorization: Bearer benchmark-token' http://localhost:9867/config/autosolver > %q || true", autosolverPath), 15*time.Second)
	_, _, _ = shell.Run(fmt.Sprintf("curl -sS -H 'Authorization: Bearer benchmark-token' 'http://localhost:9867/screenshot?format=png&raw=true' > %q || true", screenshotPath), 30*time.Second)
	_, _, _ = shell.Run(fmt.Sprintf("curl -sS -H 'Authorization: Bearer benchmark-token' 'http://localhost:9867/network/export?format=har' > %q || true", harPath), 30*time.Second)
	_, _, _ = shell.Run(fmt.Sprintf("./scripts/pt console > %q 2>/dev/null || true", consolePath), 15*time.Second)

	row.StealthLevel = readJSONField(stealthPath, "level")
	row.SolverUsed, row.SolveAttempts = deriveSolveMetadata(commandLogPath)
	row.Success = evaluateBrowserBenchAnswer(row.AgentResult, task.GroundTruth) && row.ErrorMessage == ""

	return row, nil
}

func runBrowserBenchAgentLoop(task BrowserBenchTask, provider string, shell *PersistentShell, runner Runner, args BrowserBenchArgs, commandLogPath string, stdout io.Writer) (string, error) {
	conversation := runner.InitialConversation(browserBenchUserPrompt(task))
	systemPrompt := browserBenchSystemPrompt(filepath.Dir(filepath.Dir(resolveToolsDir())))
	out := NewOutputWriter(stdout, args.Verbose)

	outcome := runTurnLoop(turnLoopConfig{
		maxTurns:        args.MaxTurns,
		turnDelayMs:     args.TurnDelayMs,
		timeoutSeconds:  args.TimeoutSeconds,
		maxInputTokens:  args.MaxInputTokens,
		maxOutputTokens: args.MaxOutputTokens,
		spinnerMessage:  "Solving BrowserBench task...",
		systemPrompt:    systemPrompt,
		commandLog:      commandLogPath,
		provider:        provider,
		compactionSummary: func() string {
			return "BrowserBench task in progress. Continue from the current browser state until you can answer with FINAL_ANSWER."
		},
		onToolResults: nil,
	}, runner, shell, out, conversation)

	switch outcome.stop {
	case stopFinal:
		return outcome.finalText, nil
	case stopBudgetInput:
		return "", fmt.Errorf("input token budget exceeded")
	case stopBudgetOutput:
		return "", fmt.Errorf("output token budget exceeded")
	case stopAPIError:
		return "", outcome.err
	default: // stopMaxTurns
		return "", fmt.Errorf("max turns reached (%d)", args.MaxTurns)
	}
}

func browserBenchSystemPrompt(repoRoot string) string {
	base := `You are a precise web-task execution agent running BrowserBench-style tasks through PinchTab.

Rules:
- Use shell tool calls only.
- Use only ./scripts/pt for browser actions.
- The browser is already pointed at the task's starting URL when the task begins.
- Do NOT create or manage PinchTab sessions yourself. Do not call ./scripts/pt session create, session login, or other session bootstrap commands.
- Prefer PinchTab primitives such as snap, text, click, fill, press, wait, back, eval, console, and solve.
- Do NOT invent convenience flags. "--snap" and "--snap-diff" are valid on some actions (for example nav, click, fill, select, scroll, back, forward, reload) but not on every command. In particular, do not use them with press. When unsure, run a separate "./scripts/pt snap -i -c" after the action.
- If a site presents a challenge, you may use ./scripts/pt solve.
- Stay on the starting website domain unless the task explicitly requires a page on the same site reached by navigation.
- Do not fabricate answers.
- When you are confident, end with a single line in this exact format: FINAL_ANSWER: <answer>
- Keep the final answer short and factual.`

	skill := LoadSkillContent(filepath.Join(repoRoot, "skills", "pinchtab", "SKILL.md"))
	if skill == "" {
		return base
	}
	return base + "\n\n# PinchTab Skill\n\n" + skill
}

func browserBenchUserPrompt(task BrowserBenchTask) string {
	return fmt.Sprintf(`Complete this BrowserBench task.

Starting URL: %s
Task ID: %s
Instruction: %s

Important:
- the browser is already opened on the starting URL
- do not use the ground truth in your reasoning
- return exactly one final line: FINAL_ANSWER: <answer>`, task.StartingURL, task.TaskID, task.TaskDescription)
}

func readFileOrURL(path string) ([]byte, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(path)
}

func writeBrowserBenchCSV(path string, rows []BrowserBenchRow) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	headers := []string{
		"task_id", "starting_url", "task_description", "ground_truth_url", "ground_truth",
		"provider", "timestamp", "success", "error_message", "agent_result", "session_url", "execution_time",
		"pinchtab_version", "stealth_level", "solver_used", "attempts",
		"har_path", "screenshot_path", "console_log_path", "command_log_path",
		"request_count", "input_tokens", "output_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "total_input_tokens", "total_tokens",
	}
	if err := w.Write(headers); err != nil {
		return err
	}
	for _, row := range rows {
		rec := []string{
			row.TaskID, row.StartingURL, row.TaskDescription, row.GroundTruthURL, row.GroundTruth,
			row.Provider, row.Timestamp, strconv.FormatBool(row.Success), row.ErrorMessage, row.AgentResult, row.SessionURL, row.ExecutionTimeSeconds,
			row.PinchTabVersion, row.StealthLevel, row.SolverUsed, row.SolveAttempts,
			row.HARPath, row.ScreenshotPath, row.ConsoleLogPath, row.CommandLogPath,
			row.RequestCount, row.InputTokens, row.OutputTokens, row.CacheCreationInputTokens, row.CacheReadInputTokens, row.TotalInputTokens, row.TotalTokens,
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func detectPinchTabVersion(shell *PersistentShell) string {
	out, _, err := shell.Run("docker exec tools-pinchtab-1 sh -lc 'pinchtab --version 2>/dev/null || true'", 15*time.Second)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		return strings.TrimSpace(out)
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

func sanitizeTaskID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	return re.ReplaceAllString(v, "-")
}

func extractFinalAnswer(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "FINAL_ANSWER:") {
			return strings.TrimSpace(line[len("FINAL_ANSWER:"):])
		}
	}
	return strings.TrimSpace(text)
}

func evaluateBrowserBenchAnswer(answer, truth string) bool {
	a := normalizeAnswer(answer)
	g := normalizeAnswer(truth)
	if a == "" || g == "" {
		return false
	}
	if g == "yes" || g == "no" {
		return a == g || strings.HasPrefix(a, g+" ") || strings.Contains(a, " "+g+" ")
	}
	if a == g || strings.Contains(a, g) || strings.Contains(g, a) {
		return true
	}
	return false
}

func normalizeAnswer(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "’", "'")
	s = strings.ReplaceAll(s, "“", `"`)
	s = strings.ReplaceAll(s, "”", `"`)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	s = strings.Trim(s, " .,!?:;\"'")
	return s
}

func diffUsage(after, before UsageCounters) UsageCounters {
	return UsageCounters{
		RequestCount:             after.RequestCount - before.RequestCount,
		InputTokens:              after.InputTokens - before.InputTokens,
		OutputTokens:             after.OutputTokens - before.OutputTokens,
		CacheCreationInputTokens: after.CacheCreationInputTokens - before.CacheCreationInputTokens,
		CacheReadInputTokens:     after.CacheReadInputTokens - before.CacheReadInputTokens,
	}
}

func readJSONField(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	if v, ok := m[key]; ok {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

func deriveSolveMetadata(commandLogPath string) (string, string) {
	data, err := os.ReadFile(commandLogPath)
	if err != nil {
		return "", ""
	}
	lines := strings.Split(string(data), "\n")
	solver := ""
	maxAttempt := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry struct {
			Output string `json:"output"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		var payload struct {
			Solver   string `json:"solver"`
			Attempts int    `json:"attempts"`
		}
		if err := json.Unmarshal([]byte(entry.Output), &payload); err != nil {
			continue
		}
		if payload.Solver != "" {
			solver = payload.Solver
		}
		if payload.Attempts > maxAttempt {
			maxAttempt = payload.Attempts
		}
	}
	if maxAttempt == 0 {
		return solver, ""
	}
	return solver, strconv.Itoa(maxAttempt)
}
