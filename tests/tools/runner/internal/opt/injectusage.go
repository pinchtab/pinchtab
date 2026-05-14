package opt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func RunInjectUsage(argv []string, stdout, stderr io.Writer) int {
	var reportFile string
	var transcriptFiles []string

	i := 0
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		switch argv[i] {
		case "--report", "-r":
			if i+1 >= len(argv) {
				_, _ = fmt.Fprintf(stderr, "inject-usage: %s requires a value\n", argv[i])
				return 1
			}
			reportFile = argv[i+1]
			i += 2
		default:
			_, _ = fmt.Fprintf(stderr, "inject-usage: unknown option: %s\n", argv[i])
			return 1
		}
	}

	transcriptFiles = argv[i:]
	if len(transcriptFiles) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: inject-usage [-r report.json] <transcript1.jsonl> [transcript2.jsonl ...]")
		return 1
	}

	// Expand globs
	var files []string
	for _, pattern := range transcriptFiles {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			if _, statErr := os.Stat(pattern); statErr == nil {
				files = append(files, pattern)
			} else {
				_, _ = fmt.Fprintf(stderr, "inject-usage: no files matched %q\n", pattern)
				return 1
			}
		} else {
			files = append(files, matches...)
		}
	}

	var totals struct {
		InputTokens              int64 `json:"input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		RequestCount             int64 `json:"request_count"`
	}

	var agentDurations []float64
	var wallClockMs float64

	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "inject-usage: failed to open %s: %v\n", f, err)
			return 1
		}

		var fileRequests int64
		var firstTS, lastTS string
		scanner := bufio.NewScanner(fh)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			var entry struct {
				Timestamp string `json:"timestamp"`
				Message   struct {
					Role  string `json:"role"`
					Usage struct {
						InputTokens              int64 `json:"input_tokens"`
						CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
						CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
						OutputTokens             int64 `json:"output_tokens"`
					} `json:"usage"`
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
			u := entry.Message.Usage
			if u.InputTokens == 0 && u.CacheCreationInputTokens == 0 && u.CacheReadInputTokens == 0 && u.OutputTokens == 0 {
				continue
			}
			totals.InputTokens += u.InputTokens
			totals.CacheCreationInputTokens += u.CacheCreationInputTokens
			totals.CacheReadInputTokens += u.CacheReadInputTokens
			totals.OutputTokens += u.OutputTokens
			fileRequests++
		}
		_ = fh.Close()

		totals.RequestCount += fileRequests
		_, _ = fmt.Fprintf(stdout, "Parsed %d API responses from %s\n", fileRequests, filepath.Base(f))

		if firstTS != "" && lastTS != "" {
			t0, err0 := time.Parse(time.RFC3339Nano, firstTS)
			t1, err1 := time.Parse(time.RFC3339Nano, lastTS)
			if err0 == nil && err1 == nil {
				d := t1.Sub(t0).Seconds() * 1000
				agentDurations = append(agentDurations, d)
				if d > wallClockMs {
					wallClockMs = d
				}
			}
		}
	}

	totalInput := totals.InputTokens + totals.CacheCreationInputTokens + totals.CacheReadInputTokens
	totalTokens := totalInput + totals.OutputTokens

	usage := map[string]any{
		"source":                      "subagent-transcripts",
		"provider":                    "anthropic",
		"request_count":               totals.RequestCount,
		"input_tokens":                totals.InputTokens,
		"cache_creation_input_tokens": totals.CacheCreationInputTokens,
		"cache_read_input_tokens":     totals.CacheReadInputTokens,
		"total_input_tokens":          totalInput,
		"output_tokens":               totals.OutputTokens,
		"total_tokens":                totalTokens,
	}
	if wallClockMs > 0 {
		usage["wall_clock_ms"] = wallClockMs
		usage["agent_durations_ms"] = agentDurations
	}

	_, _ = fmt.Fprintf(stdout, "\nToken usage:\n")
	_, _ = fmt.Fprintf(stdout, "  API requests:   %d\n", totals.RequestCount)
	_, _ = fmt.Fprintf(stdout, "  Input (uncached): %d\n", totals.InputTokens)
	_, _ = fmt.Fprintf(stdout, "  Cache write:      %d\n", totals.CacheCreationInputTokens)
	_, _ = fmt.Fprintf(stdout, "  Cache read:       %d\n", totals.CacheReadInputTokens)
	_, _ = fmt.Fprintf(stdout, "  Total input:      %d\n", totalInput)
	_, _ = fmt.Fprintf(stdout, "  Output:           %d\n", totals.OutputTokens)
	_, _ = fmt.Fprintf(stdout, "  Total tokens:     %d\n", totalTokens)

	if reportFile != "" {
		data, err := os.ReadFile(reportFile)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "inject-usage: failed to read %s: %v\n", reportFile, err)
			return 1
		}
		var report map[string]any
		if err := json.Unmarshal(data, &report); err != nil {
			_, _ = fmt.Fprintf(stderr, "inject-usage: failed to parse %s: %v\n", reportFile, err)
			return 1
		}

		report["run_usage"] = usage

		output, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "inject-usage: failed to marshal: %v\n", err)
			return 1
		}
		if err := os.WriteFile(reportFile, output, 0644); err != nil {
			_, _ = fmt.Fprintf(stderr, "inject-usage: failed to write %s: %v\n", reportFile, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "\nInjected run_usage into %s\n", reportFile)
	}

	return 0
}
