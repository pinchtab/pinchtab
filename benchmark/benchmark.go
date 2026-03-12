// Package benchmark provides an automated benchmarking framework for the
// PinchTab IDPI (Indirect Prompt Injection) Shield.
//
// It loads labeled content samples, feeds them through the Shield's detection
// engine, and produces classification metrics (accuracy, precision, recall,
// F1, false positives, false negatives).
package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/idpi"
)

// Sample represents a single labeled content sample in the benchmark dataset.
type Sample struct {
	ID              string `json:"id"`
	Category        string `json:"category"`
	Label           string `json:"label"` // "malicious" or "safe"
	Severity        string `json:"severity"`
	Description     string `json:"description"`
	Content         string `json:"content"`
	ContentType     string `json:"content_type"`
	AttackVector    string `json:"attack_vector"`
	ExpectedPattern string `json:"expected_pattern"`
}

// Result holds the outcome of running a single sample through the Shield.
// This struct is used internally during evaluation and for the text report.
type Result struct {
	Sample         Sample `json:"sample"`
	ShieldDetected bool   `json:"shield_detected"`
	ShieldBlocked  bool   `json:"shield_blocked"`
	ShieldReason   string `json:"shield_reason"`
	ShieldPattern  string `json:"shield_pattern"`
	Correct        bool   `json:"correct"`
	Classification string `json:"classification"` // TP, TN, FP, FN
}

// FlatResult is a flattened representation of a single evaluation result,
// designed for machine-readable JSON reports. All fields from the sample
// are promoted to top level so an AI agent can parse results without
// navigating nested objects.
type FlatResult struct {
	SampleID       string `json:"sample_id"`
	Category       string `json:"category"`
	Label          string `json:"label"`
	ContentType    string `json:"content_type"`
	ShieldDetected bool   `json:"shield_detected"`
	MatchedPattern string `json:"matched_pattern"`
	Classification string `json:"classification"`
	Description    string `json:"description"`
	AttackVector   string `json:"attack_vector"`
	Severity       string `json:"severity"`
}

// Flatten converts an internal Result to a FlatResult for JSON report output.
func (r *Result) Flatten() FlatResult {
	return FlatResult{
		SampleID:       r.Sample.ID,
		Category:       r.Sample.Category,
		Label:          r.Sample.Label,
		ContentType:    r.Sample.ContentType,
		ShieldDetected: r.ShieldDetected,
		MatchedPattern: r.ShieldPattern,
		Classification: r.Classification,
		Description:    r.Sample.Description,
		AttackVector:   r.Sample.AttackVector,
		Severity:       r.Sample.Severity,
	}
}

// Metrics holds the aggregated evaluation metrics.
type Metrics struct {
	TotalSamples   int     `json:"total_samples"`
	MaliciousCount int     `json:"malicious_count"`
	SafeCount      int     `json:"safe_count"`
	TruePositives  int     `json:"true_positives"`
	TrueNegatives  int     `json:"true_negatives"`
	FalsePositives int     `json:"false_positives"`
	FalseNegatives int     `json:"false_negatives"`
	Accuracy       float64 `json:"accuracy"`
	Precision      float64 `json:"precision"`
	Recall         float64 `json:"recall"`
	F1Score        float64 `json:"f1_score"`
}

// Report is the full benchmark output.
// The JSON serialization uses FlatResults for machine-readability;
// the internal Results field is used by the text report generator.
type Report struct {
	Timestamp   string                      `json:"timestamp"`
	Duration    string                      `json:"duration"`
	Config      config.IDPIConfig           `json:"config"`
	Metrics     Metrics                     `json:"metrics"`
	FlatResults []FlatResult                `json:"results"`
	ByCategory  map[string]*CategoryMetrics `json:"by_category"`

	// Results is the full internal result set, used by the text report
	// generator but excluded from JSON output.
	Results []Result `json:"-"`
}

// CategoryMetrics tracks per-category performance.
type CategoryMetrics struct {
	Category       string `json:"category"`
	Total          int    `json:"total"`
	Correct        int    `json:"correct"`
	TruePositives  int    `json:"true_positives"`
	FalseNegatives int    `json:"false_negatives"`
	FalsePositives int    `json:"false_positives"`
	TrueNegatives  int    `json:"true_negatives"`
}

// LoadSamples reads all JSON sample files from a single directory.
// Each .json file is unmarshaled into a Sample struct. Non-JSON files
// and subdirectories are silently skipped. The returned slice is ordered
// by filename (os.ReadDir sorts lexicographically).
func LoadSamples(dir string) ([]Sample, error) {
	var samples []Sample

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		var s Sample
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		samples = append(samples, s)
	}

	return samples, nil
}

// LoadDataset loads the full benchmark dataset by reading samples from
// the malicious/ and safe/ subdirectories under datasetDir. It returns
// a combined slice with malicious samples first, followed by safe samples.
func LoadDataset(datasetDir string) ([]Sample, error) {
	malDir := filepath.Join(datasetDir, "malicious")
	safeDir := filepath.Join(datasetDir, "safe")

	mal, err := LoadSamples(malDir)
	if err != nil {
		return nil, fmt.Errorf("loading malicious samples: %w", err)
	}

	safe, err := LoadSamples(safeDir)
	if err != nil {
		return nil, fmt.Errorf("loading safe samples: %w", err)
	}

	return append(mal, safe...), nil
}

// RunBenchmark executes the full benchmark pipeline:
//  1. Load all samples from datasetDir (malicious/ + safe/)
//  2. Evaluate each sample against the IDPI Shield via idpi.ScanContent
//  3. Classify each result as TP, TN, FP, or FN
//  4. Compute aggregate metrics (accuracy, precision, recall, F1)
//  5. Group results by category for per-category analysis
//  6. Return a Report containing both internal Results and flattened FlatResults
//
// The pipeline is deterministic: identical input always produces identical output.
func RunBenchmark(datasetDir string, cfg config.IDPIConfig) (*Report, error) {
	start := time.Now()

	samples, err := LoadDataset(datasetDir)
	if err != nil {
		return nil, fmt.Errorf("loading dataset: %w", err)
	}

	if len(samples) == 0 {
		return nil, fmt.Errorf("no samples found in %s", datasetDir)
	}

	results := make([]Result, 0, len(samples))
	byCategory := make(map[string]*CategoryMetrics)

	for _, sample := range samples {
		result := evaluateSample(sample, cfg)
		results = append(results, result)

		cat, ok := byCategory[sample.Category]
		if !ok {
			cat = &CategoryMetrics{Category: sample.Category}
			byCategory[sample.Category] = cat
		}
		cat.Total++
		if result.Correct {
			cat.Correct++
		}
		switch result.Classification {
		case "TP":
			cat.TruePositives++
		case "TN":
			cat.TrueNegatives++
		case "FP":
			cat.FalsePositives++
		case "FN":
			cat.FalseNegatives++
		}
	}

	metrics := calculateMetrics(results)

	// Build flattened results for the machine-readable JSON report.
	flatResults := make([]FlatResult, len(results))
	for i := range results {
		flatResults[i] = results[i].Flatten()
	}

	return &Report{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Duration:    time.Since(start).String(),
		Config:      cfg,
		Metrics:     metrics,
		FlatResults: flatResults,
		ByCategory:  byCategory,
		Results:     results,
	}, nil
}

// evaluateSample runs one sample through the IDPI Shield and classifies
// the outcome into TP, TN, FP, or FN by comparing the Shield's detection
// result against the sample's ground-truth label.
func evaluateSample(sample Sample, cfg config.IDPIConfig) Result {
	check := idpi.ScanContent(sample.Content, cfg)

	isMalicious := sample.Label == "malicious"
	detected := check.Threat

	var classification string
	switch {
	case isMalicious && detected:
		classification = "TP" // True Positive: correctly detected threat
	case !isMalicious && !detected:
		classification = "TN" // True Negative: correctly passed safe content
	case !isMalicious && detected:
		classification = "FP" // False Positive: wrongly flagged safe content
	case isMalicious && !detected:
		classification = "FN" // False Negative: missed a real threat
	}

	return Result{
		Sample:         sample,
		ShieldDetected: check.Threat,
		ShieldBlocked:  check.Blocked,
		ShieldReason:   check.Reason,
		ShieldPattern:  check.Pattern,
		Correct:        (isMalicious == detected),
		Classification: classification,
	}
}

// calculateMetrics computes aggregate classification metrics from results.
func calculateMetrics(results []Result) Metrics {
	var m Metrics
	m.TotalSamples = len(results)

	for _, r := range results {
		if r.Sample.Label == "malicious" {
			m.MaliciousCount++
		} else {
			m.SafeCount++
		}
		switch r.Classification {
		case "TP":
			m.TruePositives++
		case "TN":
			m.TrueNegatives++
		case "FP":
			m.FalsePositives++
		case "FN":
			m.FalseNegatives++
		}
	}

	// Accuracy = (TP + TN) / Total
	if m.TotalSamples > 0 {
		m.Accuracy = float64(m.TruePositives+m.TrueNegatives) / float64(m.TotalSamples)
	}

	// Precision = TP / (TP + FP)
	if (m.TruePositives + m.FalsePositives) > 0 {
		m.Precision = float64(m.TruePositives) / float64(m.TruePositives+m.FalsePositives)
	}

	// Recall = TP / (TP + FN)
	if (m.TruePositives + m.FalseNegatives) > 0 {
		m.Recall = float64(m.TruePositives) / float64(m.TruePositives+m.FalseNegatives)
	}

	// F1 = 2 * (Precision * Recall) / (Precision + Recall)
	if (m.Precision + m.Recall) > 0 {
		m.F1Score = 2 * (m.Precision * m.Recall) / (m.Precision + m.Recall)
	}

	return m
}
