package actions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pinchtab/pinchtab/internal/audit"
	auditreport "github.com/pinchtab/pinchtab/internal/audit/report"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// auditTimeout bounds a whole multi-page audit run; well above the default
// per-request client timeout, which a site audit easily exceeds.
const auditTimeout = 10 * time.Minute

func mustString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func mustBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

// Audit runs a multi-page site audit via POST /audit and writes artifacts.
func Audit(client *http.Client, base, token string, cmd *cobra.Command, target string) {
	body := map[string]any{}
	switch {
	case mustString(cmd, "seaportal-report") != "":
		path := mustString(cmd, "seaportal-report")
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read seaportal report: %v\n", err)
			os.Exit(1)
		}
		body["seaportalResults"] = json.RawMessage(data)
		body["seaportalFile"] = path
		if v, _ := cmd.Flags().GetBool("enrich-all"); v {
			body["enrichAll"] = true
		}
	case mustBool(cmd, "sitemap"):
		body["sitemapUrl"] = target
	default:
		body["urls"] = []string{target}
	}

	screenshot, _ := cmd.Flags().GetBool("screenshot")
	network, _ := cmd.Flags().GetBool("network-monitor")
	body["options"] = map[string]any{"screenshot": screenshot, "network": network}
	if v, _ := cmd.Flags().GetInt("concurrency"); v > 0 {
		body["concurrency"] = v
	}
	if v, _ := cmd.Flags().GetInt("sample-size"); v > 0 {
		body["sampleSize"] = v
	}

	longClient := &http.Client{Transport: client.Transport, Timeout: auditTimeout}
	raw := apiclient.DoPostRaw(longClient, base, token, "/audit", body)

	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		fmt.Fprintf(os.Stderr, "parse audit report: %v\n", err)
		os.Exit(1)
	}

	format := renderFormat(cmd)

	if dir, _ := cmd.Flags().GetString("output-dir"); dir != "" {
		if err := writeAuditArtifacts(dir, report); err != nil {
			fmt.Fprintf(os.Stderr, "write artifacts: %v\n", err)
			os.Exit(1)
		}
		if format != auditreport.FormatJSON {
			if err := renderAuditReportFile(dir, report, format); err != nil {
				fmt.Fprintf(os.Stderr, "render report: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", filepath.Join(dir, "report.json"))
	} else if format != auditreport.FormatJSON {
		rendered, err := auditreport.Render(typedAuditReport(report), format)
		if err != nil {
			fmt.Fprintf(os.Stderr, "render report: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(rendered))
		return
	}

	if v, _ := cmd.Flags().GetBool("json"); v {
		out, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(out))
		return
	}
	printAuditSummary(report)
}

// renderFormat reads the --format flag, defaulting to json.
func renderFormat(cmd *cobra.Command) string {
	f := mustString(cmd, "format")
	if f == "" {
		return auditreport.FormatJSON
	}
	return f
}

// typedAuditReport converts the generic report map (possibly mutated by
// artifact writing) back into the typed schema for rendering.
func typedAuditReport(report map[string]any) audit.AuditReport {
	data, _ := json.Marshal(report)
	var typed audit.AuditReport
	_ = json.Unmarshal(data, &typed)
	return typed
}

// renderAuditReportFile writes report.md / report.html next to report.json.
func renderAuditReportFile(dir string, report map[string]any, format string) error {
	rendered, err := auditreport.Render(typedAuditReport(report), format)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report."+format), rendered, 0o644)
}

// writeAuditArtifacts writes report.json and screenshots/ under dir. Inline
// base64 screenshots are moved to files and each page's browser entry gets
// the relative screenshotPath instead.
func writeAuditArtifacts(dir string, report map[string]any) error {
	shotsDir := filepath.Join(dir, "screenshots")
	if err := os.MkdirAll(shotsDir, 0o755); err != nil {
		return err
	}

	pages, _ := report["pages"].([]any)
	for i, p := range pages {
		page, ok := p.(map[string]any)
		if !ok {
			continue
		}
		b64, _ := page["screenshot"].(string)
		if b64 == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			continue
		}
		relPath := filepath.Join("screenshots", fmt.Sprintf("page-%03d.png", i+1))
		if err := os.WriteFile(filepath.Join(dir, relPath), data, 0o644); err != nil {
			return err
		}
		delete(page, "screenshot")
		if browser, ok := page["browser"].(map[string]any); ok {
			browser["screenshotPath"] = relPath
		}
	}

	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.json"), out, 0o644)
}

func printAuditSummary(report map[string]any) {
	pages, _ := report["pages"].([]any)
	failed := 0
	broken := 0
	for _, p := range pages {
		page, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if e, _ := page["error"].(string); e != "" {
			failed++
		}
		if browser, ok := page["browser"].(map[string]any); ok {
			if assets, ok := browser["brokenAssets"].([]any); ok {
				broken += len(assets)
			}
		}
	}
	fmt.Printf("Audited %d page(s) · summary score %v · %d broken asset(s) · %d failed page(s)\n",
		len(pages), report["summaryScore"], broken, failed)
	for _, p := range pages {
		page, ok := p.(map[string]any)
		if !ok {
			continue
		}
		status := "ok"
		if e, _ := page["error"].(string); e != "" {
			status = "error: " + e
		}
		fmt.Printf("  %s · %s\n", page["url"], status)
	}
}
