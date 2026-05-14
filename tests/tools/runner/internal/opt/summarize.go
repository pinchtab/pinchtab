package opt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func RunSummarize(argv []string, stdout, stderr io.Writer) int {
	var reportFile string
	var transcriptFiles []string

	i := 0
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		switch argv[i] {
		case "--report", "-r":
			if i+1 >= len(argv) {
				_, _ = fmt.Fprintf(stderr, "summarize: %s requires a value\n", argv[i])
				return 1
			}
			reportFile = argv[i+1]
			i += 2
		default:
			_, _ = fmt.Fprintf(stderr, "summarize: unknown option: %s\n", argv[i])
			return 1
		}
	}
	transcriptFiles = argv[i:]

	if reportFile == "" {
		_, _ = fmt.Fprintln(stderr, "usage: summarize -r <merged-report.json> [transcript1.jsonl ...]")
		return 1
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "summarize: %v\n", err)
		return 1
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		_, _ = fmt.Fprintf(stderr, "summarize: %v\n", err)
		return 1
	}

	steps, _ := report["steps"].([]any)
	var answered, failed, skipped int
	var totalDurationMs float64
	var stepsWithDuration int
	seen := make(map[string]string)
	for _, s := range steps {
		step, _ := s.(map[string]any)
		status, _ := step["status"].(string)
		switch status {
		case "answer":
			answered++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
		if d, ok := step["duration_ms"].(float64); ok && d > 0 {
			totalDurationMs += d
			stepsWithDuration++
		}
		id, _ := step["id"].(string)
		if id == "" {
			g, _ := step["group"].(float64)
			st, _ := step["step"].(float64)
			id = fmt.Sprintf("%d.%d", int(g), int(st))
		}
		answer, _ := step["answer"].(string)
		seen[id] = answer
	}
	totalSteps := answered + failed + skipped

	var verifyPass, verifyFail int
	var verifyFailures []string
	for id, answer := range seen {
		pattern, has := expectPatterns[id]
		if !has {
			continue
		}
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil || !re.MatchString(answer) {
			verifyFail++
			truncated := answer
			if len(truncated) > 80 {
				truncated = truncated[:77] + "..."
			}
			verifyFailures = append(verifyFailures, fmt.Sprintf("%s: [%s]", id, truncated))
		} else {
			verifyPass++
		}
	}

	var missing []string
	for g := 0; g <= maxGroup(); g++ {
		count := groupSizes[g]
		for s := 1; s <= count; s++ {
			id := fmt.Sprintf("%d.%d", g, s)
			if _, ok := seen[id]; !ok {
				missing = append(missing, id)
			}
		}
	}

	// Count baseline ops from baseline.sh
	baselineOps := countBaselineOps()

	// Parse browser ops and agent durations from transcripts
	ptRe := regexp.MustCompile(`\./scripts/pt\s+(\w+)`)
	var totalOps int
	cmdTypes := make(map[string]int)
	var hasTranscripts bool
	var totalAgentDurationMs float64

	for _, f := range expandGlobs(transcriptFiles, stderr) {
		fh, err := os.Open(f)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "summarize: warning: cannot open %s: %v\n", f, err)
			continue
		}
		scanner := bufio.NewScanner(fh)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		var firstTS, lastTS string
		for scanner.Scan() {
			line := scanner.Bytes()
			var entry struct {
				Timestamp string `json:"timestamp"`
				Message   struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			if entry.Timestamp != "" {
				if firstTS == "" {
					firstTS = entry.Timestamp
				}
				lastTS = entry.Timestamp
			}
			var blocks []struct {
				Type  string `json:"type"`
				Name  string `json:"name"`
				Input struct {
					Command string `json:"command"`
				} `json:"input"`
			}
			if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
				continue
			}
			for _, b := range blocks {
				if b.Type == "tool_use" && b.Name == "Bash" {
					for _, m := range ptRe.FindAllStringSubmatch(b.Input.Command, -1) {
						totalOps++
						cmdTypes[m[1]]++
					}
				}
			}
		}
		_ = fh.Close()
		hasTranscripts = true

		if firstTS != "" && lastTS != "" {
			t0, err0 := time.Parse(time.RFC3339Nano, firstTS)
			t1, err1 := time.Parse(time.RFC3339Nano, lastTS)
			if err0 == nil && err1 == nil {
				totalAgentDurationMs += t1.Sub(t0).Seconds() * 1000
			}
		}
	}

	usage, _ := report["run_usage"].(map[string]any)
	getInt := func(key string) int64 {
		if v, ok := usage[key].(float64); ok {
			return int64(v)
		}
		return 0
	}

	// Build rows for a single combined table
	type row struct {
		metric, baseline, agent string
	}
	blSteps := totalBaselineSteps()
	blLabel := fmt.Sprintf("%d/%d", blSteps, blSteps)
	rows := []row{
		{"Steps completed", blLabel, fmt.Sprintf("%d/%d", totalSteps, blSteps)},
		{"Verified pass", blLabel, fmt.Sprintf("%d/%d", verifyPass, blSteps)},
	}
	if hasTranscripts {
		blOpsStr := ""
		blOpsRatio := ""
		if baselineOps > 0 {
			blOpsStr = fmtInt(int64(baselineOps))
			blOpsRatio = fmt.Sprintf("%.1f", float64(baselineOps)/float64(blSteps))
		}
		rows = append(rows,
			row{"Browser ops", blOpsStr, fmtInt(int64(totalOps))},
			row{"Ops/step", blOpsRatio, fmt.Sprintf("%.1f", float64(totalOps)/float64(blSteps))},
		)
	}
	rows = append(rows, row{"Errors", "0", fmt.Sprintf("%d", failed)})

	// Prefer transcript-based timing, then run_usage wall clock, then per-step sum.
	var displayDurationMs float64
	if totalAgentDurationMs > 0 {
		displayDurationMs = totalAgentDurationMs
	} else if wc, ok := usage["wall_clock_ms"].(float64); ok && wc > 0 {
		displayDurationMs = wc
	} else if stepsWithDuration > 0 {
		displayDurationMs = totalDurationMs
	}
	if displayDurationMs > 0 {
		avgMs := displayDurationMs / float64(blSteps)
		rows = append(rows,
			row{"", "", ""},
			row{"Avg time/step", "", fmt.Sprintf("%.1fs", avgMs/1000)},
			row{"Total time", "", fmtDuration(displayDurationMs)},
		)
	}

	if usage != nil {
		rows = append(rows,
			row{"", "", ""},
			row{"API requests", "", fmtInt(getInt("request_count"))},
			row{"Input (uncached)", "", fmtInt(getInt("input_tokens"))},
			row{"Cache write", "", fmtInt(getInt("cache_creation_input_tokens"))},
			row{"Cache read", "", fmtInt(getInt("cache_read_input_tokens"))},
			row{"Total input", "", fmtInt(getInt("total_input_tokens"))},
			row{"Output", "", fmtInt(getInt("output_tokens"))},
			row{"Total tokens", "", fmtInt(getInt("total_tokens"))},
		)
	}

	// Compute column widths
	colW := [3]int{len("Metric"), len("Baseline"), len("Agent")}
	for _, r := range rows {
		if r.metric == "" {
			continue
		}
		if len(r.metric) > colW[0] {
			colW[0] = len(r.metric)
		}
		if len(r.baseline) > colW[1] {
			colW[1] = len(r.baseline)
		}
		if len(r.agent) > colW[2] {
			colW[2] = len(r.agent)
		}
	}

	hLine := fmt.Sprintf("+-%-s-+-%-s-+-%-s-+", strings.Repeat("-", colW[0]), strings.Repeat("-", colW[1]), strings.Repeat("-", colW[2]))
	fmtRow := func(a, b, c string) string {
		return fmt.Sprintf("| %-*s | %-*s | %-*s |", colW[0], a, colW[1], b, colW[2], c)
	}

	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintln(stdout, hLine)
	_, _ = fmt.Fprintln(stdout, fmtRow("Metric", "Baseline", "Agent"))
	_, _ = fmt.Fprintln(stdout, hLine)
	for _, r := range rows {
		if r.metric == "" {
			_, _ = fmt.Fprintln(stdout, hLine)
			continue
		}
		_, _ = fmt.Fprintln(stdout, fmtRow(r.metric, r.baseline, r.agent))
	}
	_, _ = fmt.Fprintln(stdout, hLine)

	// Ops breakdown (compact, one line)
	if hasTranscripts {
		type kv struct {
			k string
			v int
		}
		var sortedOps []kv
		for k, v := range cmdTypes {
			sortedOps = append(sortedOps, kv{k, v})
		}
		sort.Slice(sortedOps, func(i, j int) bool { return sortedOps[i].v > sortedOps[j].v })
		_, _ = fmt.Fprintln(stdout)
		_, _ = fmt.Fprint(stdout, "Ops: ")
		for i, s := range sortedOps {
			if i > 0 {
				_, _ = fmt.Fprint(stdout, ", ")
			}
			_, _ = fmt.Fprintf(stdout, "%s %d", s.k, s.v)
		}
		_, _ = fmt.Fprintln(stdout)
	}

	if len(missing) > 0 {
		_, _ = fmt.Fprintf(stdout, "\nMissing: %s\n", strings.Join(missing, ", "))
	}

	if len(verifyFailures) > 0 {
		_, _ = fmt.Fprintln(stdout, "\nVerification failures:")
		for _, f := range verifyFailures {
			_, _ = fmt.Fprintf(stdout, "  %s\n", f)
		}
	}

	_, _ = fmt.Fprintln(stdout)
	return 0
}

func fmtDuration(ms float64) string {
	totalSec := int(ms / 1000)
	if totalSec < 60 {
		return fmt.Sprintf("%ds", totalSec)
	}
	m := totalSec / 60
	s := totalSec % 60
	if m < 60 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	h := m / 60
	m = m % 60
	return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
}

func fmtInt(n int64) string {
	if n == 0 {
		return "0"
	}
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// countBaselineOps counts browser operations in scripts/baseline.sh.
// Each invocation of a wrapper (ACT, NAV, SNAP, EV) or a direct curl call
// to $BASE counts as one operation. Function definitions (lines containing
// "() {") are excluded to avoid counting the wrapper bodies.
func countBaselineOps() int {
	candidates := []string{
		"scripts/baseline.sh",
		"tests/tools/scripts/baseline.sh",
	}
	var path string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			path = c
			break
		}
	}
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	wrapperRe := regexp.MustCompile(`\b(ACT|NAV|SNAP|EV|FRAME)\b[\s'"{(]`)
	curlRe := regexp.MustCompile(`\bcurl `)
	funcDefRe := regexp.MustCompile(`\(\)\s*\{`)

	var total int
	for _, line := range strings.Split(string(data), "\n") {
		if funcDefRe.MatchString(line) {
			continue
		}
		total += len(wrapperRe.FindAllString(line, -1))
		total += len(curlRe.FindAllString(line, -1))
	}
	return total
}

func expandGlobs(patterns []string, stderr io.Writer) []string {
	var out []string
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil || len(matches) == 0 {
			if _, statErr := os.Stat(p); statErr == nil {
				out = append(out, p)
			}
		} else {
			out = append(out, matches...)
		}
	}
	return out
}
