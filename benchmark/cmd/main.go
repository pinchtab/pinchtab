// Command benchmark-runner executes the IDPI Shield benchmark and generates
// a report. This is a standalone CLI tool for running the benchmark.
//
// It always produces benchmark/reports/latest.json so that automated agents
// can read results from a stable path without discovering timestamped filenames.
//
// Usage:
//
//	go run benchmark/cmd/main.go [flags]
//	  -dataset    Path to dataset directory (default: benchmark/dataset)
//	  -output     Path to reports directory (default: benchmark/reports)
//	  -strict     Enable strict mode (default: true)
//	  -custom     Comma-separated custom patterns to add
//	  -json       Output JSON only (no text to stdout)
//
// Exit codes:
//
//	0  Benchmark passed (accuracy >= 80%)
//	1  Runtime error
//	2  Accuracy below 80% threshold
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/benchmark"
	"github.com/pinchtab/pinchtab/internal/config"
)

func main() {
	datasetDir := flag.String("dataset", "benchmark/dataset", "path to the benchmark dataset directory")
	outputDir := flag.String("output", "benchmark/reports", "path to write reports")
	strict := flag.Bool("strict", true, "enable strict mode (block on detection)")
	customPat := flag.String("custom", "", "comma-separated custom patterns to add")
	jsonOnly := flag.Bool("json", false, "output JSON only (no text report)")
	flag.Parse()

	cfg := config.IDPIConfig{
		Enabled:     true,
		StrictMode:  *strict,
		ScanContent: true,
		WrapContent: false,
	}

	if *customPat != "" {
		for _, p := range strings.Split(*customPat, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.CustomPatterns = append(cfg.CustomPatterns, p)
			}
		}
	}

	fmt.Println("🔬 IDPI Shield Benchmark Runner")
	fmt.Println("================================")
	fmt.Printf("Dataset : %s\n", *datasetDir)
	fmt.Printf("Output  : %s\n", *outputDir)
	fmt.Printf("Strict  : %v\n", *strict)
	if len(cfg.CustomPatterns) > 0 {
		fmt.Printf("Custom  : %v\n", cfg.CustomPatterns)
	}
	fmt.Println()

	report, err := benchmark.RunBenchmark(*datasetDir, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	if !*jsonOnly {
		fmt.Print(benchmark.GenerateReport(report))
	}

	if err := benchmark.SaveReport(report, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR saving report: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nReports saved to: %s\n", *outputDir)
	fmt.Printf("Machine-readable: %s/latest.json\n", *outputDir)

	// Exit with non-zero if accuracy is below threshold
	if report.Metrics.Accuracy < 0.8 {
		fmt.Fprintf(os.Stderr, "\n⚠️  Accuracy %.1f%% is below 80%% threshold\n", report.Metrics.Accuracy*100)
		os.Exit(2)
	}
}
