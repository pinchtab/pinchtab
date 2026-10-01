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
	"github.com/pinchtab/pinchtab/internal/authn"
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
func applyRunAuth(conn apiclient.Conn, cmd *cobra.Command, scopeURLs ...string) (apiclient.Conn, func() error, error) {
	cookies, err := loadRunCookies(cmd)
	if err != nil {
		return apiclient.Conn{}, nil, err
	}
	if len(cookies) == 0 {
		if profile := mustString(cmd, "profile"); profile != "" {
			conn.Base, err = resolveProfileBase(conn, profile)
			if err != nil {
				return apiclient.Conn{}, nil, err
			}
		}
		return conn, func() error { return nil }, nil
	}
	if mustString(cmd, "profile") != "" {
		return apiclient.Conn{}, nil, fmt.Errorf("--profile cannot be combined with --cookie or --cookies-file; cookie-authenticated runs use an isolated temporary instance")
	}
	scopes := cookieScopeURLs(scopeURLs)
	if len(scopes) == 0 {
		return apiclient.Conn{}, nil, fmt.Errorf("cookie injection requires a URL target")
	}

	instanceBase, cleanup, err := startIsolatedInstance(conn)
	if err != nil {
		return apiclient.Conn{}, nil, err
	}
	conn.Base = instanceBase
	for _, scope := range scopes {
		if err := setRunCookies(conn, scope, cookies); err != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				return apiclient.Conn{}, nil, fmt.Errorf("set run cookies: %w (also failed to stop isolated instance: %w)", err, cleanupErr)
			}
			return apiclient.Conn{}, nil, fmt.Errorf("set run cookies: %w", err)
		}
	}
	return conn, cleanup, nil
}

func cookieScopeURLs(urls []string) []string {
	seen := make(map[string]bool, len(urls))
	scopes := make([]string, 0, len(urls))
	for _, raw := range urls {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		scope := (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/"}).String()
		if !seen[scope] {
			seen[scope] = true
			scopes = append(scopes, scope)
		}
	}
	return scopes
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
func setRunCookies(conn apiclient.Conn, url string, cookies []audit.Cookie) error {
	raw, err := conn.Raw(http.MethodPost, "/tab", apiclient.WithBody(map[string]any{"action": "new"}))
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
		defer func() {
			_, _ = conn.Raw(http.MethodPost, "/close", apiclient.WithBody(map[string]any{"tabId": tab.TabID}))
		}()
	}
	body := map[string]any{"url": url, "cookies": cookies}
	if tab.TabID != "" {
		body["tabId"] = tab.TabID
	}
	raw, err = conn.Raw(http.MethodPost, "/cookies", apiclient.WithBody(body))
	if err != nil {
		return err
	}
	var result struct {
		Set    int `json:"set"`
		Failed int `json:"failed"`
		Total  int `json:"total"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parse cookie injection result: %w", err)
	}
	if result.Failed != 0 || result.Set != len(cookies) {
		return fmt.Errorf("cookie injection incomplete: set %d of %d cookies (%d failed)", result.Set, len(cookies), result.Failed)
	}
	return nil
}

// startIsolatedInstance creates an unnamed instance. The orchestrator assigns
// these an instance-* profile and removes its profile directory on stop.
func startIsolatedInstance(conn apiclient.Conn) (string, func() error, error) {
	raw, err := conn.Raw(http.MethodPost, "/instances/start", apiclient.WithBody(map[string]any{}))
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
		_, err := conn.Raw(http.MethodPost, "/instances/"+url.PathEscape(instance.ID)+"/stop")
		if err != nil {
			return fmt.Errorf("stop isolated instance: %w", err)
		}
		return nil
	}
	if err := waitForIsolatedInstance(conn, instance.ID); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return "", nil, fmt.Errorf("wait for isolated instance: %w (also failed to stop isolated instance: %w)", err, cleanupErr)
		}
		return "", nil, fmt.Errorf("wait for isolated instance: %w", err)
	}
	// Child instance URLs bind to the orchestrator's local interface. They are
	// therefore not generally reachable by a remote CLI client (including the
	// E2E runner container). Use the orchestrator's per-instance proxy instead.
	return strings.TrimSuffix(conn.Base, "/") + "/instances/" + url.PathEscape(instance.ID), cleanup, nil
}

func waitForIsolatedInstance(conn apiclient.Conn, instanceID string) error {
	const timeout = 30 * time.Second
	deadline := time.Now().Add(timeout)
	path := "/instances/" + url.PathEscape(instanceID)

	for {
		raw, err := conn.Raw(http.MethodGet, path)
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

// resolveProfileBase routes the run at the instance owning the named profile
// through the orchestrator proxy. Child URLs normally bind to localhost and
// are therefore not reachable by remote CLI clients.
func resolveProfileBase(conn apiclient.Conn, profile string) (string, error) {
	raw, err := conn.Raw(http.MethodGet, "/instances")
	if err != nil {
		return "", err
	}
	var instances []struct {
		ID          string `json:"id"`
		ProfileName string `json:"profileName"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(raw, &instances); err != nil {
		return "", fmt.Errorf("parse instances: %w", err)
	}
	for _, inst := range instances {
		if inst.ProfileName != profile || inst.Status != "running" {
			continue
		}
		return strings.TrimSuffix(conn.Base, "/") + "/instances/" + url.PathEscape(inst.ID), nil
	}
	return "", fmt.Errorf("no running instance for profile %q (start one: pinchtab instance start --profile %s)", profile, profile)
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
func Audit(conn apiclient.Conn, cmd *cobra.Command, target string) (err error) {
	if err := validateAuditFlags(cmd); err != nil {
		return err
	}

	body := map[string]any{}
	authScopes := []string{target}
	switch {
	case mustString(cmd, "seaportal-report") != "":
		path := mustString(cmd, "seaportal-report")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read seaportal report: %w", err)
		}
		pages, err := audit.ParseSeaportalReport(data)
		if err != nil {
			return err
		}
		authScopes = make([]string, 0, len(pages))
		for _, page := range pages {
			authScopes = append(authScopes, page.URL)
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

	conn, cleanup, err := applyRunAuth(conn, cmd, authScopes...)
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			if err == nil {
				err = cleanupErr
			} else {
				err = fmt.Errorf("%w (also %w)", err, cleanupErr)
			}
		}
	}()

	screenshot, _ := cmd.Flags().GetBool("screenshot")
	network, _ := cmd.Flags().GetBool("network-monitor")
	body["options"] = map[string]any{"screenshot": screenshot, "network": network}
	if v, _ := cmd.Flags().GetInt("concurrency"); v > 0 {
		body["concurrency"] = v
	}
	if v, _ := cmd.Flags().GetInt("sample-size"); v > 0 {
		body["sampleSize"] = v
	}

	longClient := &http.Client{Transport: conn.Client.Transport, Timeout: auditTimeout}
	raw, err := conn.WithClient(longClient).Raw(http.MethodPost, "/audit", apiclient.WithBody(body))
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
			if err := exportAuditPDF(conn, dir, typedForPDF); err != nil {
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
func exportAuditPDF(conn apiclient.Conn, dir string, report audit.AuditReport) error {
	pdf, err := auditreport.ExportPDF(report, serverPDFPrinter(conn))
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
func serverPDFPrinter(conn apiclient.Conn) auditreport.PDFPrinter {
	return func(html []byte) ([]byte, error) {
		status, _, tab := conn.PostQuietWithStatus("/tab", map[string]any{"action": "new"})
		tabID, _ := tab["tabId"].(string)
		if status >= 400 || tabID == "" {
			return nil, fmt.Errorf("create print tab: HTTP %d", status)
		}
		defer func() {
			_, _, _ = conn.PostQuietWithStatus("/close", map[string]any{"tabId": tabID})
		}()

		doc, _ := json.Marshal(string(html))
		status, body, _ := conn.PostQuietWithStatus("/evaluate", map[string]any{
			"tabId":      tabID,
			"expression": fmt.Sprintf("document.open(); document.write(%s); document.close(); true", doc),
		})
		if status >= 400 {
			return nil, fmt.Errorf("inject report HTML (requires the evaluate capability): HTTP %d: %s", status, strings.TrimSpace(string(body)))
		}
		return fetchRawPDF(conn, tabID)
	}
}

func fetchRawPDF(conn apiclient.Conn, tabID string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, conn.Base+"/pdf?raw=true&tabId="+url.QueryEscape(tabID), nil)
	if err != nil {
		return nil, err
	}
	authn.SetAuthorization(req.Header, conn.Token)
	resp, err := conn.Client.Do(req)
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
	for _, line := range auditSummaryLines(typedAuditReport(report)) {
		fmt.Println(line)
	}
}

// auditSummaryLines renders the human summary of an audit run: the headline
// counts followed by one status line per page. The headline names the score
// for what it measures, since it cannot move for the failures printed beside it.
func auditSummaryLines(report audit.AuditReport) []string {
	failed, broken, jsErrors := 0, 0, 0
	for _, p := range report.Pages {
		// A page failed when the audit could not collect it (transport Error) OR
		// when its own document returned 4xx/5xx — the latter has no Error, so it
		// used to read as ok. The main-document failure is not in BrokenAssets
		// (dropped in ToPageResult), so broken counts only failed sub-resources.
		if p.Error != "" || p.StatusCode >= 400 {
			failed++
		}
		broken += len(p.Browser.BrokenAssets)
		jsErrors += len(p.Browser.JSErrors)
	}
	lines := []string{fmt.Sprintf(
		"Audited %d page(s) · mean accessibility score %d · %d broken asset(s) · %d uncaught JS error(s) · %d failed page(s)",
		len(report.Pages), report.SummaryScore, broken, jsErrors, failed)}
	for _, p := range report.Pages {
		lines = append(lines, fmt.Sprintf("  %s · %s", p.URL, audit.PageStatus(p)))
	}
	return lines
}
