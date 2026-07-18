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
// --cookies-file cookies before the run. Cookie-authenticated runs receive a
// disposable instance so they never alter a persistent browser profile.
func applyRunAuth(client *http.Client, base, token string, cmd *cobra.Command, scopeURL string) (string, func() error, error) {
	cookies, err := loadRunCookies(cmd)
	if err != nil {
		return "", nil, err
	}
	if len(cookies) == 0 {
		if profile := mustString(cmd, "profile"); profile != "" {
			base, err = resolveProfileBase(client, base, token, profile)
			if err != nil {
				return "", nil, err
			}
		}
		return base, func() error { return nil }, nil
	}
	if mustString(cmd, "profile") != "" {
		return "", nil, fmt.Errorf("--profile cannot be combined with --cookie or --cookies-file; cookie-authenticated runs use an isolated temporary instance")
	}
	if scopeURL == "" {
		return "", nil, fmt.Errorf("cookie injection requires a URL target")
	}

	instanceBase, cleanup, err := startIsolatedInstance(client, base, token)
	if err != nil {
		return "", nil, err
	}
	if err := setRunCookies(client, instanceBase, token, scopeURL, cookies); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return "", nil, fmt.Errorf("set run cookies: %w (also failed to stop isolated instance: %v)", err, cleanupErr)
		}
		return "", nil, fmt.Errorf("set run cookies: %w", err)
	}
	return instanceBase, cleanup, nil
}

func loadRunCookies(cmd *cobra.Command) ([]audit.Cookie, error) {
	flags, _ := cmd.Flags().GetStringArray("cookie")
	var fileData []byte
	if path := mustString(cmd, "cookies-file"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read cookies file: %w", err)
		}
		fileData = data
	}
	cookies, err := audit.CollectCookies(flags, fileData)
	if err != nil {
		return nil, err
	}
	return cookies, nil
}

// setRunCookies injects cookies through POST /cookies, using a throwaway
// blank tab for the required tab context so it works on a fresh browser.
func setRunCookies(client *http.Client, base, token, url string, cookies []audit.Cookie) error {
	raw, err := apiclient.DoPostRawE(client, base, token, "/tab", map[string]any{"action": "new"})
	if err != nil {
		return err
	}
	var tab struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(raw, &tab); err != nil {
		return fmt.Errorf("parse temporary tab: %w", err)
	}
	if tab.TabID != "" {
		defer func() { _, _ = apiclient.DoPostRawE(client, base, token, "/close", map[string]any{"tabId": tab.TabID}) }()
	}
	body := map[string]any{"url": url, "cookies": cookies}
	if tab.TabID != "" {
		body["tabId"] = tab.TabID
	}
	if _, err := apiclient.DoPostRawE(client, base, token, "/cookies", body); err != nil {
		return err
	}
	return nil
}

// startIsolatedInstance creates an unnamed instance. The orchestrator assigns
// these an instance-* profile and removes its profile directory on stop.
func startIsolatedInstance(client *http.Client, base, token string) (string, func() error, error) {
	raw, err := apiclient.DoPostRawE(client, base, token, "/instances/start", map[string]any{})
	if err != nil {
		return "", nil, fmt.Errorf("start isolated instance: %w", err)
	}
	var instance struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &instance); err != nil {
		return "", nil, fmt.Errorf("parse isolated instance: %w", err)
	}
	if instance.ID == "" || instance.URL == "" {
		return "", nil, fmt.Errorf("start isolated instance: response omitted id or url")
	}
	cleanup := func() error {
		_, err := apiclient.DoPostRawE(client, base, token, "/instances/"+url.PathEscape(instance.ID)+"/stop", nil)
		if err != nil {
			return fmt.Errorf("stop isolated instance: %w", err)
		}
		return nil
	}
	if err := waitForIsolatedInstance(client, base, token, instance.ID); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return "", nil, fmt.Errorf("wait for isolated instance: %w (also failed to stop isolated instance: %v)", err, cleanupErr)
		}
		return "", nil, fmt.Errorf("wait for isolated instance: %w", err)
	}
	// Child instance URLs bind to the orchestrator's local interface. They are
	// therefore not generally reachable by a remote CLI client (including the
	// E2E runner container). Use the orchestrator's per-instance proxy instead.
	return strings.TrimSuffix(base, "/") + "/instances/" + url.PathEscape(instance.ID), cleanup, nil
}

func waitForIsolatedInstance(client *http.Client, base, token, instanceID string) error {
	const timeout = 30 * time.Second
	deadline := time.Now().Add(timeout)
	path := "/instances/" + url.PathEscape(instanceID)

	for {
		raw, err := apiclient.DoGetRawE(client, base, token, path, nil)
		if err == nil {
			var instance struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(raw, &instance); err != nil {
				return fmt.Errorf("parse instance status: %w", err)
			}
			switch instance.Status {
			case "running":
				return nil
			case "error", "stopped":
				return fmt.Errorf("instance %q entered %s state", instanceID, instance.Status)
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("instance %q did not become ready within %s: %w", instanceID, timeout, err)
			}
			return fmt.Errorf("instance %q did not become ready within %s", instanceID, timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// resolveProfileBase routes the run at the instance owning the named
// profile: the orchestrator base for the default-routed instance, or the
// instance's own URL otherwise.
func resolveProfileBase(client *http.Client, base, token, profile string) (string, error) {
	raw, err := apiclient.DoGetRawE(client, base, token, "/instances", nil)
	if err != nil {
		return "", err
	}
	var instances []struct {
		ID          string `json:"id"`
		ProfileName string `json:"profileName"`
		Status      string `json:"status"`
		URL         string `json:"url"`
	}
	if err := json.Unmarshal(raw, &instances); err != nil {
		return "", fmt.Errorf("parse instances: %w", err)
	}
	for _, inst := range instances {
		if inst.ProfileName != profile || inst.Status != "running" {
			continue
		}
		if inst.ID == defaultInstanceID(client, base, token) {
			return base, nil
		}
		return strings.TrimSuffix(inst.URL, "/"), nil
	}
	return "", fmt.Errorf("no running instance for profile %q (start one: pinchtab instance start --profile %s)", profile, profile)
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
func Audit(client *http.Client, base, token string, cmd *cobra.Command, target string) (err error) {
	if err := validateAuditFlags(cmd); err != nil {
		return err
	}

	base, cleanup, err := applyRunAuth(client, base, token, cmd, target)
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

	body := map[string]any{}
	switch {
	case mustString(cmd, "seaportal-report") != "":
		path := mustString(cmd, "seaportal-report")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read seaportal report: %w", err)
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
	raw, err := apiclient.DoPostRawE(longClient, base, token, "/audit", body)
	if err != nil {
		return err
	}

	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		return fmt.Errorf("parse audit report: %w", err)
	}

	format := renderFormat(cmd)

	if dir, _ := cmd.Flags().GetString("output-dir"); dir != "" {
		var typedForPDF audit.AuditReport
		if format == auditreport.FormatPDF {
			// Captured before artifact writing strips the inline screenshots.
			typedForPDF = typedAuditReport(report)
		}
		if err := writeAuditArtifacts(dir, report); err != nil {
			return fmt.Errorf("write artifacts: %w", err)
		}
		switch {
		case format == auditreport.FormatPDF:
			if err := exportAuditPDF(client, base, token, dir, typedForPDF); err != nil {
				return err
			}
		case format != auditreport.FormatJSON:
			if err := renderAuditReportFile(dir, report, format); err != nil {
				return fmt.Errorf("render report: %w", err)
			}
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", filepath.Join(dir, "report.json"))
	} else if format != auditreport.FormatJSON {
		rendered, err := auditreport.Render(typedAuditReport(report), format)
		if err != nil {
			return fmt.Errorf("render report: %w", err)
		}
		fmt.Println(string(rendered))
		return nil
	}

	if v, _ := cmd.Flags().GetBool("json"); v {
		out, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(out))
		return nil
	}
	printAuditSummary(report)
	return nil
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
func exportAuditPDF(client *http.Client, base, token, dir string, report audit.AuditReport) error {
	pdf, err := auditreport.ExportPDF(report, serverPDFPrinter(client, base, token))
	if err != nil {
		return fmt.Errorf("PDF export failed: %w (report.json was still written)", err)
	}
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		return fmt.Errorf("write %s: %w (report.json was still written)", path, err)
	}
	return nil
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
