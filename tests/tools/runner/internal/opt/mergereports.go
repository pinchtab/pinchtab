package opt

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MergeReportsArgs struct {
	OutputFile string
	InputFiles []string
}

func ParseMergeReportsArgs(argv []string) (MergeReportsArgs, error) {
	var args MergeReportsArgs

	i := 0
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		switch argv[i] {
		case "--output", "-o":
			if i+1 >= len(argv) {
				return args, fmt.Errorf("%s requires a value", argv[i])
			}
			args.OutputFile = argv[i+1]
			i += 2
		default:
			return args, fmt.Errorf("unknown option: %s", argv[i])
		}
	}

	args.InputFiles = argv[i:]
	if len(args.InputFiles) == 0 {
		return args, fmt.Errorf("usage: merge-reports [-o output.json] <report1.json> <report2.json>")
	}

	return args, nil
}

func RunMergeReports(argv []string, stdout, stderr io.Writer) int {
	args, err := ParseMergeReportsArgs(argv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "merge-reports: %v\n", err)
		return 1
	}

	// Also support glob patterns (e.g. agent*_20260429.json)
	var files []string
	for _, pattern := range args.InputFiles {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			if _, statErr := os.Stat(pattern); statErr == nil {
				files = append(files, pattern)
			} else {
				_, _ = fmt.Fprintf(stderr, "merge-reports: no files matched %q\n", pattern)
				return 1
			}
		} else {
			files = append(files, matches...)
		}
	}

	seenSteps := make(map[string]map[string]any)

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "merge-reports: failed to read %s: %v\n", f, err)
			return 1
		}
		var report map[string]any
		if err := json.Unmarshal(data, &report); err != nil {
			_, _ = fmt.Fprintf(stderr, "merge-reports: failed to parse %s: %v\n", f, err)
			return 1
		}

		steps, _ := report["steps"].([]any)
		for _, s := range steps {
			step, ok := s.(map[string]any)
			if !ok {
				continue
			}
			id, _ := step["id"].(string)
			if id == "" {
				g, _ := step["group"].(float64)
				s, _ := step["step"].(float64)
				id = fmt.Sprintf("%d.%d", int(g), int(s))
			}
			seenSteps[id] = step
		}
		_, _ = fmt.Fprintf(stdout, "Loaded %d steps from %s\n", len(steps), filepath.Base(f))
	}

	allSteps := make([]map[string]any, 0, len(seenSteps))
	for _, step := range seenSteps {
		allSteps = append(allSteps, step)
	}

	// Sort by group.step
	sort.Slice(allSteps, func(i, j int) bool {
		gi, _ := allSteps[i]["group"].(float64)
		si, _ := allSteps[i]["step"].(float64)
		gj, _ := allSteps[j]["group"].(float64)
		sj, _ := allSteps[j]["step"].(float64)
		if gi != gj {
			return gi < gj
		}
		return si < sj
	})

	// Build merged report
	merged := map[string]any{
		"benchmark": map[string]any{
			"type":      "pinchtab",
			"timestamp": time.Now().UTC().Format("20060102_150405"),
			"merged":    true,
			"sources":   len(files),
		},
		"totals": map[string]any{
			"steps_answered": len(allSteps),
		},
		"steps": allSteps,
	}

	output, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "merge-reports: failed to marshal: %v\n", err)
		return 1
	}

	if args.OutputFile != "" {
		if err := os.WriteFile(args.OutputFile, output, 0644); err != nil {
			_, _ = fmt.Fprintf(stderr, "merge-reports: failed to write %s: %v\n", args.OutputFile, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "Merged %d unique steps into %s\n", len(allSteps), args.OutputFile)
	} else {
		_, _ = fmt.Fprintf(stdout, "Merged %d unique steps (use -o to write to file)\n", len(allSteps))
		_, _ = stdout.Write(output)
		_, _ = fmt.Fprintln(stdout)
	}

	return 0
}
