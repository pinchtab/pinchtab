package actions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/audit"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

const (
	compareLiveBase    = "http://localhost:18601"
	compareStagingBase = "http://localhost:18602"
)

// compareStub answers /audit with one page per requested URL, carrying the
// browser data of whichever side the URL belongs to.
func compareStub(t *testing.T, live, staging audit.BrowserPageData) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audit" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			URLs []string `json:"urls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode audit body: %v", err)
			return
		}
		report := audit.AuditReport{}
		for _, u := range body.URLs {
			data := live
			if strings.HasPrefix(u, compareStagingBase) {
				data = staging
			}
			report.Pages = append(report.Pages, audit.PageResult{URL: u, Browser: data})
		}
		_ = json.NewEncoder(w).Encode(report)
	}))
}

func uncaughtOn(base string) audit.BrowserPageData {
	return audit.BrowserPageData{JSErrors: []audit.JSError{{
		Message: "Uncaught: ReferenceError: brokenFunctionThatDoesNotExist is not defined\n    at " + base + "/index.html:3:9",
	}}}
}

func consoleErrorPage() audit.BrowserPageData {
	return audit.BrowserPageData{ConsoleLogs: []audit.ConsoleLogEntry{{Level: "error", Message: "staging regression"}}}
}

func runCompareGate(t *testing.T, live, staging audit.BrowserPageData) (*cobra.Command, error) {
	t.Helper()
	srv := compareStub(t, live, staging)
	defer srv.Close()
	cmd := newCompareTestCmd("--fail-on-diff")
	return cmd, Compare(apiclient.Conn{Client: http.DefaultClient, Base: srv.URL}, cmd, compareLiveBase, compareStagingBase)
}

func TestCompareGateFailsOnEitherErrorChannel(t *testing.T) {
	tests := []struct {
		name     string
		live     audit.BrowserPageData
		staging  audit.BrowserPageData
		wantFail bool
	}{
		{
			name:     "uncaught exception introduced on staging",
			staging:  uncaughtOn(compareStagingBase),
			wantFail: true,
		},
		{
			name:     "console error introduced on staging",
			staging:  consoleErrorPage(),
			wantFail: true,
		},
		{
			name:     "identical pages",
			wantFail: false,
		},
		{
			name:     "the same uncaught exception on both sides",
			live:     uncaughtOn(compareLiveBase),
			staging:  uncaughtOn(compareStagingBase),
			wantFail: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := runCompareGate(t, tc.live, tc.staging)
			if tc.wantFail {
				if err == nil || !strings.Contains(err.Error(), "--fail-on-diff") {
					t.Fatalf("Compare() error = %v, want the differences-found refusal", err)
				}
				if !cmd.SilenceUsage {
					t.Error("a gate rejection must not print the usage block")
				}
				return
			}
			if err != nil {
				t.Fatalf("Compare() error = %v, want the gate to pass", err)
			}
			if cmd.SilenceUsage {
				t.Error("usage suppression leaked onto a passing run")
			}
		})
	}
}

func TestCompareSummaryNamesTheDriftedFields(t *testing.T) {
	pc := audit.PageComparison{Drift: []audit.DataDrift{{Field: "jsErrors"}}}
	if got := driftLabel(pc.Drift); got != "drift 1 (jsErrors)" {
		t.Fatalf("driftLabel = %q, want the field named", got)
	}
	if got := driftLabel(nil); got != "drift 0" {
		t.Fatalf("driftLabel(nil) = %q", got)
	}
}
