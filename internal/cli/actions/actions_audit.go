package actions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// applyRunAuth resolves --profile routing and injects --cookie /
// --cookies-file cookies before the run. Isolation approach: injected
// cookies live in the shared browser jar, so the returned cleanup clears
// the jar after the run (documented on the audit/compare help).
func applyRunAuth(client *http.Client, base, token string, cmd *cobra.Command, scopeURL string) (string, func()) {
	if profile := mustString(cmd, "profile"); profile != "" {
		base = resolveProfileBase(client, base, token, profile)
	}
	cookies := loadRunCookies(cmd)
	if len(cookies) == 0 {
		return base, func() {}
	}
	if scopeURL == "" {
		fmt.Fprintln(os.Stderr, "cookie injection requires a URL target")
		os.Exit(1)
	}
	setRunCookies(client, base, token, scopeURL, cookies)
	return base, func() { clearRunCookies(client, base, token) }
}

func loadRunCookies(cmd *cobra.Command) []audit.Cookie {
	flags, _ := cmd.Flags().GetStringArray("cookie")
	var fileData []byte
	if path := mustString(cmd, "cookies-file"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read cookies file: %v\n", err)
			os.Exit(1)
		}
		fileData = data
	}
	cookies, err := audit.CollectCookies(flags, fileData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	return cookies
}

// setRunCookies injects cookies through POST /cookies, using a throwaway
// blank tab for the required tab context so it works on a fresh browser.
func setRunCookies(client *http.Client, base, token, url string, cookies []audit.Cookie) {
	tab := apiclient.DoPostQuiet(client, base, token, "/tab", map[string]any{"action": "new"})
	tabID, _ := tab["tabId"].(string)
	body := map[string]any{"url": url, "cookies": cookies}
	if tabID != "" {
		body["tabId"] = tabID
	}
	apiclient.DoPostQuiet(client, base, token, "/cookies", body)
	if tabID != "" {
		apiclient.DoPostQuiet(client, base, token, "/close", map[string]any{"tabId": tabID})
	}
}

// clearRunCookies clears the browser cookie jar, quietly and best-effort.
func clearRunCookies(client *http.Client, base, token string) {
	req, err := http.NewRequest(http.MethodDelete, base+"/cookies", nil)
	if err != nil {
		return
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if resp, err := client.Do(req); err == nil {
		_ = resp.Body.Close()
	}
}

// resolveProfileBase routes the run at the instance owning the named
// profile: the orchestrator base for the default-routed instance, or the
// instance's own URL otherwise.
func resolveProfileBase(client *http.Client, base, token, profile string) string {
	raw := apiclient.DoGetRaw(client, base, token, "/instances", nil)
	var instances []struct {
		ID          string `json:"id"`
		ProfileName string `json:"profileName"`
		Status      string `json:"status"`
		URL         string `json:"url"`
	}
	if err := json.Unmarshal(raw, &instances); err != nil {
		fmt.Fprintf(os.Stderr, "parse instances: %v\n", err)
		os.Exit(1)
	}
	for _, inst := range instances {
		if inst.ProfileName != profile || inst.Status != "running" {
			continue
		}
		if inst.ID == defaultInstanceID(client, base, token) {
			return base
		}
		return strings.TrimSuffix(inst.URL, "/")
	}
	fmt.Fprintf(os.Stderr, "no running instance for profile %q (start one: pinchtab instance start --profile %s)\n", profile, profile)
	os.Exit(1)
	return ""
}

func defaultInstanceID(client *http.Client, base, token string) string {
	raw := apiclient.DoGetRaw(client, base, token, "/health", nil)
	var health struct {
		DefaultInstance struct {
			ID string `json:"id"`
		} `json:"defaultInstance"`
	}
	_ = json.Unmarshal(raw, &health)
	return health.DefaultInstance.ID
}

// validateAuditFlags rejects invalid flag combinations pre-flight, before
// any server work starts — a full audit must never run just to report a bad
// invocation.
func validateAuditFlags(cmd *cobra.Command) error {
	format := renderFormat(cmd)
	switch format {
	case auditreport.FormatJSON, auditreport.FormatMarkdown, auditreport.FormatHTML, auditreport.FormatPDF:
	default:
		return fmt.Errorf("unsupported --format %q (json, md, html, or pdf)", format)
	}
	if format == auditreport.FormatPDF && mustString(cmd, "output-dir") == "" {
		return fmt.Errorf("--format pdf requires --output-dir")
	}
	return nil
}

// Audit runs a multi-page site audit via POST /audit and writes artifacts.
func Audit(client *http.Client, base, token string, cmd *cobra.Command, target string) {
	if err := validateAuditFlags(cmd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	base, cleanupCookies := applyRunAuth(client, base, token, cmd, target)

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
	cleanupCookies()

	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		fmt.Fprintf(os.Stderr, "parse audit report: %v\n", err)
		os.Exit(1)
	}

	format := renderFormat(cmd)

	if dir, _ := cmd.Flags().GetString("output-dir"); dir != "" {
		var typedForPDF audit.AuditReport
		if format == auditreport.FormatPDF {
			// Captured before artifact writing strips the inline screenshots.
			typedForPDF = typedAuditReport(report)
		}
		if err := writeAuditArtifacts(dir, report); err != nil {
			fmt.Fprintf(os.Stderr, "write artifacts: %v\n", err)
			os.Exit(1)
		}
		switch {
		case format == auditreport.FormatPDF:
			exportAuditPDF(client, base, token, dir, typedForPDF)
		case format != auditreport.FormatJSON:
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

// exportAuditPDF prints the report to <dir>/report.pdf through the server.
// Graceful-degradation contract: report.json is already on disk by the time
// this runs; a print failure surfaces as a warning and a non-zero exit
// (pdf being the sole requested format).
func exportAuditPDF(client *http.Client, base, token, dir string, report audit.AuditReport) {
	pdf, err := auditreport.ExportPDF(report, serverPDFPrinter(client, base, token))
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: PDF export failed: %v (report.json was still written)\n", err)
		os.Exit(1)
	}
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: write %s: %v (report.json was still written)\n", path, err)
		os.Exit(1)
	}
}

// serverPDFPrinter prints self-contained HTML through the running server:
// a fresh tab, content injected via the evaluate capability, exported with
// GET /pdf.
func serverPDFPrinter(client *http.Client, base, token string) auditreport.PDFPrinter {
	return func(html []byte) ([]byte, error) {
		status, _, tab := apiclient.DoPostQuietWithStatus(client, base, token, "/tab", map[string]any{"action": "new"})
		tabID, _ := tab["tabId"].(string)
		if status >= 400 || tabID == "" {
			return nil, fmt.Errorf("create print tab: HTTP %d", status)
		}
		defer func() {
			_, _, _ = apiclient.DoPostQuietWithStatus(client, base, token, "/close", map[string]any{"tabId": tabID})
		}()

		doc, _ := json.Marshal(string(html))
		status, body, _ := apiclient.DoPostQuietWithStatus(client, base, token, "/evaluate", map[string]any{
			"tabId":      tabID,
			"expression": fmt.Sprintf("document.open(); document.write(%s); document.close(); true", doc),
		})
		if status >= 400 {
			return nil, fmt.Errorf("inject report HTML (requires the evaluate capability): HTTP %d: %s", status, strings.TrimSpace(string(body)))
		}
		return fetchRawPDF(client, base, token, tabID)
	}
}

func fetchRawPDF(client *http.Client, base, token, tabID string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, base+"/pdf?raw=true&tabId="+url.QueryEscape(tabID), nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("print pdf: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("print pdf: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
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
