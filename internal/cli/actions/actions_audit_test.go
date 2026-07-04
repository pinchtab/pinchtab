package actions

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

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

// The rejection must happen before any request: this re-execs the test
// binary to absorb Audit's os.Exit and fails if the fake instance receives
// a single hit — i.e. if the validation ever moves back below the POST.
func TestAuditPDFWithoutOutputDirExitsBeforePOST(t *testing.T) {
	if os.Getenv("AUDIT_PREFLIGHT_HELPER") == "1" {
		Audit(http.DefaultClient, os.Getenv("AUDIT_PREFLIGHT_BASE"), "", newAuditTestCmd("--format", "pdf"), "https://example.com")
		return
	}

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	cmd := exec.Command(os.Args[0], "-test.run", "TestAuditPDFWithoutOutputDirExitsBeforePOST")
	cmd.Env = append(os.Environ(), "AUDIT_PREFLIGHT_HELPER=1", "AUDIT_PREFLIGHT_BASE="+srv.URL)
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected non-zero exit, got success with output:\n%s", out)
	}
	if !strings.Contains(string(out), "--format pdf requires --output-dir") {
		t.Errorf("output should name both flags:\n%s", out)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("server received %d request(s); validation ran after server work", got)
	}
}
