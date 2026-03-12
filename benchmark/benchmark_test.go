package benchmark

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestRunBenchmark(t *testing.T) {
	// Find the dataset directory relative to this test file.
	datasetDir := filepath.Join("dataset")
	if _, err := os.Stat(filepath.Join(datasetDir, "malicious")); err != nil {
		t.Skipf("dataset not found at %s, skipping benchmark test", datasetDir)
	}

	cfg := config.IDPIConfig{
		Enabled:     true,
		StrictMode:  true,
		ScanContent: true,
		WrapContent: false,
	}

	report, err := RunBenchmark(datasetDir, cfg)
	if err != nil {
		t.Fatalf("RunBenchmark failed: %v", err)
	}

	if report.Metrics.TotalSamples == 0 {
		t.Fatal("expected samples, got 0")
	}

	t.Logf("Benchmark complete: %d samples", report.Metrics.TotalSamples)
	t.Logf("Accuracy:  %.1f%%", report.Metrics.Accuracy*100)
	t.Logf("Precision: %.1f%%", report.Metrics.Precision*100)
	t.Logf("Recall:    %.1f%%", report.Metrics.Recall*100)
	t.Logf("F1 Score:  %.1f%%", report.Metrics.F1Score*100)
	t.Logf("TP=%d TN=%d FP=%d FN=%d",
		report.Metrics.TruePositives, report.Metrics.TrueNegatives,
		report.Metrics.FalsePositives, report.Metrics.FalseNegatives)

	// Print text report
	text := GenerateReport(report)
	t.Log("\n" + text)

	// The shield should have high accuracy on the curated dataset
	if report.Metrics.Accuracy < 0.8 {
		t.Errorf("accuracy %.1f%% is below 80%% threshold", report.Metrics.Accuracy*100)
	}
}

func TestLoadSamples(t *testing.T) {
	malDir := filepath.Join("dataset", "malicious")
	if _, err := os.Stat(malDir); err != nil {
		t.Skipf("malicious dataset not found, skipping")
	}

	samples, err := LoadSamples(malDir)
	if err != nil {
		t.Fatalf("LoadSamples failed: %v", err)
	}

	if len(samples) == 0 {
		t.Fatal("expected malicious samples, got 0")
	}

	for _, s := range samples {
		if s.Label != "malicious" {
			t.Errorf("sample %s: expected label 'malicious', got %q", s.ID, s.Label)
		}
		if s.Content == "" {
			t.Errorf("sample %s: content is empty", s.ID)
		}
	}
}
