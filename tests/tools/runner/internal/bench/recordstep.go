package bench

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RecordStepArgs struct {
	ReportType    string
	ReportFile    string
	InputTokens   int
	OutputTokens  int
	ResponseBytes int
	ToolCalls     int
	ToolCallsSet  bool
	Answer        string
	Group         int
	Step          int
	Status        string
	Notes         string
}

func ParseRecordStepArgs(argv []string) (RecordStepArgs, error) {
	var args RecordStepArgs
	i := 0
	for i < len(argv) && strings.HasPrefix(argv[i], "--") {
		switch argv[i] {
		case "--type":
			if i+1 >= len(argv) {
				return args, errors.New("--type requires a value")
			}
			args.ReportType = argv[i+1]
			i += 2
		case "--report-file":
			if i+1 >= len(argv) {
				return args, errors.New("--report-file requires a value")
			}
			args.ReportFile = argv[i+1]
			i += 2
		case "--tokens":
			if i+2 >= len(argv) {
				return args, errors.New("--tokens requires two values")
			}
			var err error
			args.InputTokens, err = strconv.Atoi(argv[i+1])
			if err != nil {
				return args, fmt.Errorf("invalid input tokens: %w", err)
			}
			args.OutputTokens, err = strconv.Atoi(argv[i+2])
			if err != nil {
				return args, fmt.Errorf("invalid output tokens: %w", err)
			}
			i += 3
		case "--bytes":
			if i+1 >= len(argv) {
				return args, errors.New("--bytes requires a value")
			}
			var err error
			args.ResponseBytes, err = strconv.Atoi(argv[i+1])
			if err != nil {
				return args, fmt.Errorf("invalid bytes: %w", err)
			}
			i += 2
		case "--tool-calls":
			if i+1 >= len(argv) {
				return args, errors.New("--tool-calls requires a value")
			}
			var err error
			args.ToolCalls, err = strconv.Atoi(argv[i+1])
			if err != nil {
				return args, fmt.Errorf("invalid tool-calls: %w", err)
			}
			args.ToolCallsSet = true
			i += 2
		case "--observed", "--answer":
			if i+1 >= len(argv) {
				return args, fmt.Errorf("%s requires a value", argv[i])
			}
			args.Answer = argv[i+1]
			i += 2
		default:
			return args, fmt.Errorf("unknown option: %s", argv[i])
		}
	}

	positional := argv[i:]
	if len(positional) < 3 {
		return args, errors.New("usage: record-step [options] <group> <step> <pass|fail|skip|answer> [answer] [notes]")
	}

	var err error
	args.Group, err = parseInt(positional[0])
	if err != nil {
		return args, fmt.Errorf("invalid group: %w", err)
	}
	args.Step, err = parseInt(positional[1])
	if err != nil {
		return args, fmt.Errorf("invalid step: %w", err)
	}
	args.Status = positional[2]
	if args.Status == "observed" {
		args.Status = "answer"
	}

	switch args.Status {
	case "pass", "fail", "skip", "answer":
	default:
		return args, fmt.Errorf("status must be one of pass, fail, skip, answer")
	}

	positional = positional[3:]
	if args.Status == "answer" {
		if args.Answer == "" && len(positional) >= 1 {
			args.Answer = positional[0]
			positional = positional[1:]
		}
		if len(positional) >= 1 {
			args.Notes = positional[0]
		}
	} else {
		if len(positional) >= 1 {
			args.Notes = positional[0]
		}
	}

	return args, nil
}

func RunRecordStep(argv []string, stdout, stderr io.Writer) int {
	args, err := ParseRecordStepArgs(argv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "record-step: %v\n", err)
		return 1
	}

	benchmarkDir := resolveBenchmarkDir()
	resultsDir := filepath.Join(benchmarkDir, "results")
	_ = os.MkdirAll(resultsDir, 0755)

	reportFile := args.ReportFile
	if reportFile == "" {
		reportFile = resolveReportFile(resultsDir, args.ReportType)
	}

	if reportFile == "" || !fileExists(reportFile) {
		_, _ = fmt.Fprintln(stderr, "ERROR: No benchmark report found. Run ./run-optimization.sh first.")
		return 1
	}

	report, err := os.ReadFile(reportFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: failed to read report: %v\n", err)
		return 1
	}

	var data map[string]any
	if err := json.Unmarshal(report, &data); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: failed to parse report: %v\n", err)
		return 1
	}

	benchmark, _ := data["benchmark"].(map[string]any)
	benchmarkType, _ := benchmark["type"].(string)

	if err := validateStatus(benchmarkType, args.Status); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	stepID := fmt.Sprintf("%d.%d", args.Group, args.Step)

	// Reset stale timing files when this is the first step in the report.
	existingSteps, _ := data["steps"].([]any)
	if len(existingSteps) == 0 {
		resetTimingFiles(resultsDir, benchmarkType)
	}

	timing := computeTiming(resultsDir, benchmarkType)
	dockerStats := collectDockerStats(benchmarkType)
	cost := computeCost(args.InputTokens, args.OutputTokens, benchmark)
	toolCalls := computeToolCalls(args, data, resultsDir, benchmarkType)

	step := buildStepJSON(args, stepID, timestamp, timing, dockerStats, cost, toolCalls)

	steps, _ := data["steps"].([]any)
	steps = append(steps, step)
	data["steps"] = steps

	updateRecordTotals(data, args.InputTokens, args.OutputTokens, toolCalls, cost, timing.elapsed)

	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: failed to marshal report: %v\n", err)
		return 1
	}

	if err := os.WriteFile(reportFile, output, 0644); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: failed to write report: %v\n", err)
		return 1
	}

	persistTiming(resultsDir, benchmarkType, timing.stepEnd)

	if args.Status == "fail" {
		errLog := filepath.Join(resultsDir, "errors.log")
		f, err := os.OpenFile(errLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "[%s] Step %d.%d FAILED: %s\n", timestamp, args.Group, args.Step, args.Notes)
			_ = f.Close()
		}
	}

	_, _ = fmt.Fprintf(stdout, "Recorded %d.%d %s\n", args.Group, args.Step, args.Status)
	return 0
}

func validateStatus(benchmarkType, status string) error {
	switch benchmarkType {
	case "baseline":
		switch status {
		case "pass", "fail", "skip", "answer":
			return nil
		}
		return fmt.Errorf("baseline steps must use answer, fail, skip, or legacy pass")
	case "pinchtab", "agent", "agent-browser":
		switch status {
		case "answer", "fail", "skip":
			return nil
		}
		return fmt.Errorf("pinchtab/agent steps must use answer, fail, or skip")
	}
	return nil
}

type timingInfo struct {
	stepEnd  int64
	duration int64
	elapsed  int64
}

func computeTiming(resultsDir, benchmarkType string) timingInfo {
	key := strings.ReplaceAll(benchmarkType, "_", "-")
	runStartFile := filepath.Join(resultsDir, fmt.Sprintf("run_start_%s.ms", key))
	lastStepEndFile := filepath.Join(resultsDir, fmt.Sprintf("last_step_end_%s.ms", key))

	stepEndMs := nowMs()

	runStartMs := readMsFile(runStartFile)
	if runStartMs == 0 {
		runStartMs = stepEndMs
		writeMsFile(runStartFile, runStartMs)
	}

	lastStepEndMs := readMsFile(lastStepEndFile)

	// Discard stale last_step_end from a previous run.
	if lastStepEndMs > 0 && lastStepEndMs < runStartMs {
		lastStepEndMs = runStartMs
	}

	var durationMs int64
	if lastStepEndMs > 0 && stepEndMs >= lastStepEndMs {
		durationMs = stepEndMs - lastStepEndMs
	}

	var elapsedMs int64
	if stepEndMs >= runStartMs {
		elapsedMs = stepEndMs - runStartMs
	}

	return timingInfo{
		stepEnd:  stepEndMs,
		duration: durationMs,
		elapsed:  elapsedMs,
	}
}

func persistTiming(resultsDir, benchmarkType string, stepEndMs int64) {
	key := strings.ReplaceAll(benchmarkType, "_", "-")
	lastStepEndFile := filepath.Join(resultsDir, fmt.Sprintf("last_step_end_%s.ms", key))
	writeMsFile(lastStepEndFile, stepEndMs)
}

func resetTimingFiles(resultsDir, benchmarkType string) {
	key := strings.ReplaceAll(benchmarkType, "_", "-")
	_ = os.Remove(filepath.Join(resultsDir, fmt.Sprintf("run_start_%s.ms", key)))
	_ = os.Remove(filepath.Join(resultsDir, fmt.Sprintf("last_step_end_%s.ms", key)))
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}

func readMsFile(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(data))
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func writeMsFile(path string, ms int64) {
	_ = os.WriteFile(path, []byte(fmt.Sprintf("%d", ms)), 0644)
}

func collectDockerStats(benchmarkType string) map[string]any {
	if os.Getenv("BENCHMARK_NO_DOCKER_STATS") != "" {
		return nil
	}

	var container string
	switch benchmarkType {
	case "pinchtab", "agent":
		container = "tools-pinchtab-1"
	case "agent-browser", "agent_browser":
		container = "tools-agent-browser-1"
	default:
		return nil
	}

	cmd := exec.Command("docker", "stats", "--no-stream", "--no-trunc", // #nosec G204 -- container name from test config
		"--format", "{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}\t{{.PIDs}}",
		container)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return nil
	}

	parts := strings.Split(line, "\t")
	if len(parts) < 6 {
		return nil
	}

	cpu := strings.TrimSuffix(strings.TrimSpace(parts[0]), "%")
	memPair := strings.Split(parts[1], " / ")
	memUsed := strings.TrimSpace(memPair[0])
	memLimit := ""
	if len(memPair) > 1 {
		memLimit = strings.TrimSpace(memPair[1])
	}
	memPct := strings.TrimSuffix(strings.TrimSpace(parts[2]), "%")
	netPair := strings.Split(parts[3], " / ")
	netRx := strings.TrimSpace(netPair[0])
	netTx := ""
	if len(netPair) > 1 {
		netTx = strings.TrimSpace(netPair[1])
	}
	blockPair := strings.Split(parts[4], " / ")
	blockRead := strings.TrimSpace(blockPair[0])
	blockWrite := ""
	if len(blockPair) > 1 {
		blockWrite = strings.TrimSpace(blockPair[1])
	}
	pids := strings.TrimSpace(parts[5])

	return map[string]any{
		"cpu_pct":     cpu,
		"mem_used":    memUsed,
		"mem_limit":   memLimit,
		"mem_pct":     memPct,
		"net_rx":      netRx,
		"net_tx":      netTx,
		"block_read":  blockRead,
		"block_write": blockWrite,
		"pids":        pids,
	}
}

func computeCost(inputTokens, outputTokens int, benchmark map[string]any) float64 {
	totalTokens := inputTokens + outputTokens
	if totalTokens == 0 {
		return 0
	}

	model, _ := benchmark["model"].(string)
	model = strings.ToLower(model)

	var inputRate, outputRate float64
	switch {
	case strings.Contains(model, "haiku"):
		inputRate, outputRate = 0.25, 1.25
	case strings.Contains(model, "sonnet"):
		inputRate, outputRate = 3.0, 15.0
	case strings.Contains(model, "opus"):
		inputRate, outputRate = 15.0, 75.0
	case strings.Contains(model, "gpt-4o-mini"):
		inputRate, outputRate = 0.15, 0.60
	case strings.Contains(model, "gpt-4o"):
		inputRate, outputRate = 2.50, 10.0
	case strings.Contains(model, "gpt-4"):
		inputRate, outputRate = 10.0, 30.0
	case regexp.MustCompile(`gemini.*flash`).MatchString(model):
		inputRate, outputRate = 0.075, 0.30
	case regexp.MustCompile(`gemini.*pro`).MatchString(model):
		inputRate, outputRate = 1.25, 5.0
	default:
		inputRate, outputRate = 1.0, 3.0
	}

	return (float64(inputTokens)/1000000)*inputRate + (float64(outputTokens)/1000000)*outputRate
}

func computeToolCalls(args RecordStepArgs, data map[string]any, resultsDir, benchmarkType string) int {
	if args.ToolCallsSet {
		return args.ToolCalls
	}

	if benchmarkType != "agent-browser" {
		return 0
	}

	logFile := filepath.Join(resultsDir, "agent_browser_commands.ndjson")
	currentToolCalls := countNDJSONLines(logFile)

	totals, _ := data["totals"].(map[string]any)
	prevToolCalls := 0
	if v, ok := totals["tool_calls"].(float64); ok {
		prevToolCalls = int(v)
	}

	if currentToolCalls >= prevToolCalls {
		return currentToolCalls - prevToolCalls
	}
	return 0
}

func countNDJSONLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) == nil {
			count++
		}
	}
	return count
}

func buildStepJSON(args RecordStepArgs, stepID, timestamp string, timing timingInfo, dockerStats map[string]any, cost float64, toolCalls int) map[string]any {
	step := map[string]any{
		"group":     args.Group,
		"step":      args.Step,
		"id":        stepID,
		"status":    args.Status,
		"timestamp": timestamp,
	}

	if args.Answer != "" {
		step["answer"] = args.Answer
	}
	if args.Notes != "" {
		step["notes"] = args.Notes
	}
	if args.InputTokens > 0 {
		step["input_tokens"] = args.InputTokens
	}
	if args.OutputTokens > 0 {
		step["output_tokens"] = args.OutputTokens
	}
	totalTokens := args.InputTokens + args.OutputTokens
	if totalTokens > 0 {
		step["total_tokens"] = totalTokens
	}
	if toolCalls > 0 {
		step["tool_calls"] = toolCalls
	}
	if cost > 0 {
		step["cost_usd"] = cost
	}
	if args.ResponseBytes > 0 {
		step["response_bytes"] = args.ResponseBytes
	}
	if dockerStats != nil {
		step["docker_stats"] = dockerStats
	}
	if timing.duration > 0 {
		step["duration_ms"] = timing.duration
	}
	if args.Status == "answer" {
		step["verification"] = map[string]any{"status": "pending"}
	}

	return step
}

func updateRecordTotals(data map[string]any, inputTokens, outputTokens, toolCalls int, cost float64, elapsedMs int64) {
	totals, ok := data["totals"].(map[string]any)
	if !ok {
		totals = make(map[string]any)
		data["totals"] = totals
	}

	addInt(totals, "input_tokens", inputTokens)
	addInt(totals, "output_tokens", outputTokens)
	addInt(totals, "total_tokens", inputTokens+outputTokens)
	addInt(totals, "tool_calls", toolCalls)
	addFloat(totals, "estimated_cost_usd", cost)
	totals["elapsed_ms"] = elapsedMs

	steps, _ := data["steps"].([]any)
	var passed, failed, skipped, answered int
	var verifiedPassed, verifiedFailed, verifiedSkipped, pending int
	var stepDurationSum int64

	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		status, _ := step["status"].(string)
		switch status {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		case "answer":
			answered++
			verification, _ := step["verification"].(map[string]any)
			vstatus, _ := verification["status"].(string)
			switch vstatus {
			case "pass":
				verifiedPassed++
			case "fail":
				verifiedFailed++
			case "skip":
				verifiedSkipped++
			case "pending", "":
				pending++
			}
		}
		if d, ok := step["duration_ms"].(float64); ok {
			stepDurationSum += int64(d)
		}
	}

	totals["steps_passed"] = passed
	totals["steps_failed"] = failed
	totals["steps_skipped"] = skipped
	totals["steps_answered"] = answered
	totals["steps_verified_passed"] = verifiedPassed
	totals["steps_verified_failed"] = verifiedFailed
	totals["steps_verified_skipped"] = verifiedSkipped
	totals["steps_pending_verification"] = pending
	totals["step_duration_sum_ms"] = stepDurationSum
}

func addInt(m map[string]any, key string, delta int) {
	current := 0
	if v, ok := m[key].(float64); ok {
		current = int(v)
	} else if v, ok := m[key].(int); ok {
		current = v
	}
	m[key] = current + delta
}

func addFloat(m map[string]any, key string, delta float64) {
	current := 0.0
	if v, ok := m[key].(float64); ok {
		current = v
	}
	m[key] = current + delta
}
