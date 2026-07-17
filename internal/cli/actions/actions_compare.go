package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pinchtab/pinchtab/internal/audit"
	auditreport "github.com/pinchtab/pinchtab/internal/audit/report"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// Compare audits the same pages on two base URLs and reports visual and
// data differences per page.
func Compare(client *http.Client, base, token string, cmd *cobra.Command, liveBase, stagingBase string) (err error) {
	paths := comparePaths(cmd)
	visual := mustBool(cmd, "visual-diff")
	concurrency, _ := cmd.Flags().GetInt("concurrency")

	base, cleanup, err := applyRunAuth(client, base, token, cmd, liveBase)
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			if err == nil {
				err = cleanupErr
			} else {
				err = fmt.Errorf("%w (also %v)", err, cleanupErr)
			}
		}
	}()

	longClient := &http.Client{Transport: client.Transport, Timeout: auditTimeout}
	runSide := func(siteBase string) (audit.AuditReport, error) {
		body := map[string]any{
			"urls":    joinPaths(siteBase, paths),
			"options": map[string]any{"screenshot": visual},
		}
		if concurrency > 0 {
			body["concurrency"] = concurrency
		}
		raw, err := apiclient.DoPostRawE(longClient, base, token, "/audit", body)
		if err != nil {
			return audit.AuditReport{}, err
		}
		var report audit.AuditReport
		if err := json.Unmarshal(raw, &report); err != nil {
			return audit.AuditReport{}, fmt.Errorf("parse audit report for %s: %w", siteBase, err)
		}
		return report, nil
	}

	liveReport, err := runSide(liveBase)
	if err != nil {
		return err
	}
	stagingReport, err := runSide(stagingBase)
	if err != nil {
		return err
	}

	outcome, err := audit.ComparePages(liveBase, stagingBase, liveReport, stagingReport)
	if err != nil {
		return fmt.Errorf("compare: %w", err)
	}

	if dir, _ := cmd.Flags().GetString("output-dir"); dir != "" {
		if err := writeCompareArtifacts(dir, &outcome); err != nil {
			return fmt.Errorf("write artifacts: %w", err)
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", filepath.Join(dir, "report.json"))
	}

	format := renderFormat(cmd)
	if format != auditreport.FormatJSON {
		rendered, err := auditreport.RenderComparison(outcome.Report, format)
		if err != nil {
			return fmt.Errorf("render report: %w", err)
		}
		if dir, _ := cmd.Flags().GetString("output-dir"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, "report."+format), rendered, 0o644); err != nil {
				return fmt.Errorf("write rendered report: %w", err)
			}
		} else {
			fmt.Println(string(rendered))
		}
	}

	if mustBool(cmd, "json") {
		out, _ := json.MarshalIndent(outcome.Report, "", "  ")
		fmt.Println(string(out))
	} else if format == auditreport.FormatJSON {
		printCompareSummary(outcome.Report)
	}

	if mustBool(cmd, "fail-on-diff") && outcome.Report.HasDiffs {
		return fmt.Errorf("differences found (--fail-on-diff)")
	}
	return nil
}

func comparePaths(cmd *cobra.Command) []string {
	raw := mustString(cmd, "pages")
	if raw == "" {
		return []string{""}
	}
	var paths []string
	for _, p := range strings.Split(raw, ",") {
		paths = append(paths, strings.TrimSpace(p))
	}
	return paths
}

func joinPaths(siteBase string, paths []string) []string {
	urls := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			urls = append(urls, siteBase)
			continue
		}
		urls = append(urls, strings.TrimSuffix(siteBase, "/")+"/"+strings.TrimPrefix(p, "/"))
	}
	return urls
}

// writeCompareArtifacts writes report.json and diffs/<path>.png under dir,
// filling each page's diffImagePath before the report is serialized.
func writeCompareArtifacts(dir string, outcome *audit.CompareOutcome) error {
	diffsDir := filepath.Join(dir, "diffs")
	if err := os.MkdirAll(diffsDir, 0o755); err != nil {
		return err
	}

	for i := range outcome.Report.Pages {
		pc := &outcome.Report.Pages[i]
		data, ok := outcome.DiffImages[pc.Path]
		if !ok {
			continue
		}
		relPath := filepath.Join("diffs", diffImageName(pc.Path))
		if err := os.WriteFile(filepath.Join(dir, relPath), data, 0o644); err != nil {
			return err
		}
		pc.DiffImagePath = relPath
	}

	out, err := json.MarshalIndent(outcome.Report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.json"), out, 0o644)
}

func diffImageName(path string) string {
	if path == "" {
		path = "index"
	}
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "?", "_", "&", "_").Replace(path)
	return safe + ".diff.png"
}

func printCompareSummary(report audit.ComparisonReport) {
	fmt.Printf("Compared %d page(s) · diffs: %v\n", len(report.Pages), report.HasDiffs)
	for _, p := range report.Pages {
		label := p.Path
		if label == "" {
			label = "(base)"
		}
		switch {
		case p.Status != audit.CompareStatusCompared:
			fmt.Printf("  %s · %s\n", label, p.Status)
		case p.DiffPercentage != nil:
			fmt.Printf("  %s · visual %.2f%% · drift %d\n", label, *p.DiffPercentage, len(p.Drift))
		default:
			fmt.Printf("  %s · drift %d\n", label, len(p.Drift))
		}
	}
}
