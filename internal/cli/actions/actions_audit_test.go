package actions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pinchtab/pinchtab/internal/audit"
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

	err := Audit(http.DefaultClient, srv.URL, "", newAuditTestCmd("--format", "pdf"), "https://example.com")
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

	err := Audit(http.DefaultClient, srv.URL, "", newAuditTestCmd("--cookie", "session=temporary"), "https://example.com")
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
	_, _, err := applyRunAuth(http.DefaultClient, "http://example.invalid", "", newAuditTestCmd("--profile", "work", "--cookie", "session=temporary"), "https://example.com")
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

	base, err := resolveProfileBase(http.DefaultClient, srv.URL, "", "work")
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

	err := setRunCookies(http.DefaultClient, srv.URL, "", "https://example.com", []audit.Cookie{{Name: "session", Value: "temporary"}})
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

	err := Compare(http.DefaultClient, srv.URL, "", newCompareTestCmd("--cookie", "session=temporary"), "https://live.example", "https://staging.example")
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	if want := []string{"https://live.example", "https://staging.example"}; strings.Join(cookieURLs, ",") != strings.Join(want, ",") {
		t.Fatalf("cookie URLs = %v, want %v", cookieURLs, want)
	}
}
