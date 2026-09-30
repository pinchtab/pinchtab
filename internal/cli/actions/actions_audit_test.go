package actions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pinchtab/pinchtab/internal/audit"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newAuditTestCmd(args ...string) *cobra.Command {
	cmd := &cobra.Command{Use: "audit"}
	cmd.Flags().Bool("sitemap", false, "")
	cmd.Flags().Int("sample-size", 0, "")
	cmd.Flags().Bool("screenshot", true, "")
	cmd.Flags().Bool("network-monitor", true, "")
	cmd.Flags().String("output-dir", "", "")
	cmd.Flags().Int("concurrency", 0, "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("seaportal-report", "", "")
	cmd.Flags().Bool("enrich-all", false, "")
	cmd.Flags().String("format", "json", "")
	cmd.Flags().StringArray("cookie", nil, "")
	cmd.Flags().String("cookies-file", "", "")
	cmd.Flags().String("profile", "", "")
	if err := cmd.Flags().Parse(args); err != nil {
		panic(err)
	}
	return cmd
}

func newCompareTestCmd(args ...string) *cobra.Command {
	cmd := &cobra.Command{Use: "compare"}
	cmd.Flags().String("pages", "", "")
	cmd.Flags().Bool("visual-diff", true, "")
	cmd.Flags().String("output-dir", "", "")
	cmd.Flags().Int("concurrency", 0, "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("fail-on-diff", false, "")
	cmd.Flags().String("format", "json", "")
	cmd.Flags().StringArray("cookie", nil, "")
	cmd.Flags().String("cookies-file", "", "")
	cmd.Flags().String("profile", "", "")
	if err := cmd.Flags().Parse(args); err != nil {
		panic(err)
	}
	return cmd
}

func TestValidateAuditFlags(t *testing.T) {
	if err := validateAuditFlags(newAuditTestCmd("--format", "pdf")); err == nil {
		t.Error("pdf without output-dir should error")
	} else {
		for _, want := range []string{"--format pdf", "--output-dir"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q should name %q", err, want)
			}
		}
	}

	if err := validateAuditFlags(newAuditTestCmd("--format", "docx")); err == nil {
		t.Error("unknown format should error pre-flight")
	}

	for _, args := range [][]string{
		{},
		{"--format", "md"},
		{"--format", "html"},
		{"--format", "pdf", "--output-dir", "/tmp/x"},
	} {
		if err := validateAuditFlags(newAuditTestCmd(args...)); err != nil {
			t.Errorf("valid flags %v rejected: %v", args, err)
		}
	}
}

func TestAuditPDFWithoutOutputDirReturnsBeforePOST(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	err := Audit(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newAuditTestCmd("--format", "pdf"), "https://example.com")
	if err == nil || !strings.Contains(err.Error(), "--format pdf requires --output-dir") {
		t.Fatalf("Audit() error = %v, want missing output-dir error", err)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("server received %d request(s); validation ran after server work", got)
	}
}

func TestAuditCookieRunStopsIsolatedInstanceOnFailure(t *testing.T) {
	var stopped atomic.Bool
	var deletedCookies atomic.Bool
	var instancePaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/instances/start":
			_, _ = w.Write([]byte(`{"id":"isolated","url":"http://localhost:9870"}`))
		case "/instances/isolated":
			_, _ = w.Write([]byte(`{"id":"isolated","status":"running"}`))
		case "/instances/isolated/tab":
			instancePaths = append(instancePaths, r.URL.Path)
			_, _ = w.Write([]byte(`{"tabId":"temporary-tab"}`))
		case "/instances/isolated/cookies":
			instancePaths = append(instancePaths, r.URL.Path)
			if r.Method == http.MethodDelete {
				deletedCookies.Store(true)
			}
			_, _ = w.Write([]byte(`{"set":1}`))
		case "/instances/isolated/close":
			instancePaths = append(instancePaths, r.URL.Path)
			_, _ = w.Write([]byte(`{}`))
		case "/instances/isolated/audit":
			instancePaths = append(instancePaths, r.URL.Path)
			http.Error(w, `{"error":"upstream failed"}`, http.StatusBadGateway)
		case "/instances/isolated/stop":
			stopped.Store(true)
			_, _ = w.Write([]byte(`{"status":"stopped"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	err := Audit(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newAuditTestCmd("--cookie", "session=temporary"), "https://example.com")
	if err == nil || !strings.Contains(err.Error(), "upstream failed") {
		t.Fatalf("Audit() error = %v, want upstream failure", err)
	}
	if !stopped.Load() {
		t.Fatal("isolated instance was not stopped after audit failure")
	}
	if deletedCookies.Load() {
		t.Fatal("cookie-authenticated audit cleared cookies instead of discarding its isolated instance")
	}
	wantPaths := []string{
		"/instances/isolated/tab",
		"/instances/isolated/cookies",
		"/instances/isolated/close",
		"/instances/isolated/audit",
	}
	if strings.Join(instancePaths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("isolated instance paths = %v, want %v", instancePaths, wantPaths)
	}
}

func TestApplyRunAuthRejectsProfileWithCookies(t *testing.T) {
	_, _, err := applyRunAuth(apiclient.Conn{Client: http.DefaultClient, Base: "http://example.invalid"}, newAuditTestCmd("--profile", "work", "--cookie", "session=temporary"), "https://example.com")
	if err == nil || !strings.Contains(err.Error(), "--profile cannot be combined") {
		t.Fatalf("applyRunAuth() error = %v, want profile/cookie conflict", err)
	}
}

func TestResolveProfileBaseUsesOrchestratorProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/instances" {
			t.Fatalf("path = %s, want /instances", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"work-instance","profileName":"work","status":"running","url":"http://localhost:9870"}]`))
	}))
	defer srv.Close()

	base, err := resolveProfileBase(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, "work")
	if err != nil {
		t.Fatalf("resolveProfileBase() error = %v", err)
	}
	if want := srv.URL + "/instances/work-instance"; base != want {
		t.Fatalf("resolveProfileBase() = %q, want %q", base, want)
	}
}

func TestSetRunCookiesRejectsPartialWrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tab":
			_, _ = w.Write([]byte(`{"tabId":"temporary-tab"}`))
		case "/cookies":
			_, _ = w.Write([]byte(`{"set":0,"failed":1,"total":1}`))
		case "/close":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := setRunCookies(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, "https://example.com", []audit.Cookie{{Name: "session", Value: "temporary"}})
	if err == nil || !strings.Contains(err.Error(), "cookie injection incomplete") {
		t.Fatalf("setRunCookies() error = %v, want incomplete cookie injection", err)
	}
}

func TestCompareInjectsShorthandCookiesForBothSites(t *testing.T) {
	var cookieURLs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/instances/start":
			_, _ = w.Write([]byte(`{"id":"isolated","url":"http://localhost:9870"}`))
		case "/instances/isolated":
			_, _ = w.Write([]byte(`{"id":"isolated","status":"running"}`))
		case "/instances/isolated/tab":
			_, _ = w.Write([]byte(`{"tabId":"temporary-tab"}`))
		case "/instances/isolated/cookies":
			var body struct {
				URL string `json:"url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode cookie body: %v", err)
			}
			cookieURLs = append(cookieURLs, body.URL)
			_, _ = w.Write([]byte(`{"set":1,"failed":0,"total":1}`))
		case "/instances/isolated/close", "/instances/isolated/stop":
			_, _ = w.Write([]byte(`{}`))
		case "/instances/isolated/audit":
			_, _ = w.Write([]byte(`{"pages":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := Compare(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newCompareTestCmd("--cookie", "session=temporary"), "https://live.example", "https://staging.example")
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	if want := []string{"https://live.example/", "https://staging.example"}; strings.Join(cookieURLs, ",") != strings.Join(want, ",") {
		t.Fatalf("cookie URLs = %v, want %v", cookieURLs, want)
	}
}

func TestAuditSeaportalReportInjectsCookiesWithoutPositionalURL(t *testing.T) {
	reportPath := filepath.Join(t.TempDir(), "seaportal.json")
	if err := os.WriteFile(reportPath, []byte(`[{"url":"https://example.com/a","profile":{"browserRecommended":false}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	var cookieURLs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/instances/start":
			_, _ = w.Write([]byte(`{"id":"isolated","url":"http://localhost:9870"}`))
		case "/instances/isolated":
			_, _ = w.Write([]byte(`{"id":"isolated","status":"running"}`))
		case "/instances/isolated/tab":
			_, _ = w.Write([]byte(`{"tabId":"temporary-tab"}`))
		case "/instances/isolated/cookies":
			var body struct {
				URL string `json:"url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode cookie body: %v", err)
			}
			cookieURLs = append(cookieURLs, body.URL)
			_, _ = w.Write([]byte(`{"set":1,"failed":0,"total":1}`))
		case "/instances/isolated/close", "/instances/isolated/stop":
			_, _ = w.Write([]byte(`{}`))
		case "/instances/isolated/audit":
			_, _ = w.Write([]byte(`{"pages":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := Audit(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, newAuditTestCmd("--seaportal-report", reportPath, "--cookie", "session=temporary"), "")
	if err != nil {
		t.Fatalf("Audit() error = %v", err)
	}
	if want := []string{"https://example.com/"}; strings.Join(cookieURLs, ",") != strings.Join(want, ",") {
		t.Fatalf("cookie URLs = %v, want %v", cookieURLs, want)
	}
}

func TestAuditSummaryLines(t *testing.T) {
	report := audit.NewAuditReport()
	report.SummaryScore = 95
	report.Pages = []audit.PageResult{
		{
			URL: "http://fixtures/audit.html",
			Browser: audit.BrowserPageData{
				BrokenAssets: []audit.BrokenAsset{{URL: "/missing-image.png", Status: 404}, {URL: "/missing-script.js", Status: 404}},
				ConsoleLogs:  []audit.ConsoleLogEntry{{Level: "error", Message: "boom"}},
				JSErrors:     []audit.JSError{{Message: "Uncaught: ReferenceError: undefinedFn is not defined"}},
			},
		},
		{URL: "http://fixtures/clean.html"},
		{URL: "http://fixtures/down.html", Error: "connection refused"},
	}

	lines := auditSummaryLines(report)
	want := []string{
		"Audited 3 page(s) · mean accessibility score 95 · 2 broken asset(s) · 1 uncaught JS error(s) · 1 failed page(s)",
		"  http://fixtures/audit.html · 1 uncaught JS error(s)",
		"  http://fixtures/clean.html · ok",
		"  http://fixtures/down.html · error: connection refused",
	}
	if len(lines) != len(want) {
		t.Fatalf("auditSummaryLines = %q", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
	if strings.Contains(lines[0], "summary score") {
		t.Errorf("headline still calls the accessibility mean a summary score: %q", lines[0])
	}
}

// A page whose own document is 4xx/5xx used to read `· ok` with 0 failed pages
// (only p.Error was counted) while the document doubled as a broken asset. This
// audits a 404 URL, a plain 200 URL, and a 200 URL with a broken sub-resource
// through the real audit path and asserts the summary now distinguishes them.
func TestAuditSummaryDistinguishesA4xxDocumentFromA200(t *testing.T) {
	const url404 = "http://x/missing404.html"
	const url200 = "http://x/ok.html"
	const urlBrokenImg = "http://x/broken-image.html"

	auditor := func(u string, _ audit.PageOptions) audit.PageAudit {
		pa := audit.PageAudit{URL: u}
		switch u {
		case url404:
			pa.BrowserPageData = audit.BrowserPageData{
				NetworkRequests: []audit.NetworkRequest{{URL: u, ResourceType: "Document", Status: 404, Failed: true}},
				// The document itself is captured as a broken asset today; the fix
				// must drop it so the page failure is not also counted as broken.
				BrokenAssets: []audit.BrokenAsset{{URL: u, ResourceType: "document", Status: 404}},
			}
		case urlBrokenImg:
			img := u + "?img"
			pa.BrowserPageData = audit.BrowserPageData{
				NetworkRequests: []audit.NetworkRequest{
					{URL: u, ResourceType: "Document", Status: 200},
					{URL: img, ResourceType: "Image", Status: 404, Failed: true},
				},
				BrokenAssets: []audit.BrokenAsset{{URL: img, ResourceType: "image", Status: 404}},
			}
		default:
			pa.BrowserPageData = audit.BrowserPageData{
				NetworkRequests: []audit.NetworkRequest{{URL: u, ResourceType: "Document", Status: 200}},
			}
		}
		return pa
	}

	input := audit.AuditInput{URLs: []string{url404, url200, urlBrokenImg}}
	report, err := audit.RunAudit(input, nil, audit.RunOptions{Page: audit.PageOptions{Network: true}}, nil, auditor)
	if err != nil {
		t.Fatalf("RunAudit: %v", err)
	}

	lines := auditSummaryLines(report)
	// One failed page (the 404), one broken asset (the image on the 200 page —
	// the 404 document must NOT be counted as broken).
	if !strings.Contains(lines[0], "1 failed page(s)") {
		t.Errorf("headline failed count wrong: %q", lines[0])
	}
	if !strings.Contains(lines[0], "1 broken asset(s)") {
		t.Errorf("headline broken count wrong (document double-counted?): %q", lines[0])
	}

	status := map[string]audit.PageResult{}
	for _, p := range report.Pages {
		status[p.URL] = p
	}
	if got := audit.PageStatus(status[url404]); got != "HTTP 404" {
		t.Errorf("404 page status = %q, want HTTP 404", got)
	}
	if got := status[url404].StatusCode; got != 404 {
		t.Errorf("404 page StatusCode = %d, want 404", got)
	}
	if n := len(status[url404].Browser.BrokenAssets); n != 0 {
		t.Errorf("404 page still lists its own document as a broken asset: %+v", status[url404].Browser.BrokenAssets)
	}
	if got := audit.PageStatus(status[url200]); got != "ok" {
		t.Errorf("200 page status = %q, want ok", got)
	}
	if got := audit.PageStatus(status[urlBrokenImg]); got != "ok" {
		t.Errorf("200 page with a broken image status = %q, want ok (a sub-resource 404 is not a page failure)", got)
	}
	if n := len(status[urlBrokenImg].Browser.BrokenAssets); n != 1 {
		t.Errorf("the 200 page's broken image was dropped: %+v", status[urlBrokenImg].Browser.BrokenAssets)
	}
}
