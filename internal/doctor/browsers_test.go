package doctor

import (
	"context"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestReportBrowsers_DefaultConfig(t *testing.T) {
	// nil BrowsersAvailable — report should still list known browsers from
	// the registry.
	cfg := &config.RuntimeConfig{}
	report := ReportBrowsers(context.Background(), cfg)

	if len(report.KnownBrowsers) == 0 {
		t.Fatal("expected at least one known browser from the registry")
	}
	if len(report.Browsers) == 0 {
		t.Fatal("expected at least one BrowserInfo entry")
	}
	// Every known browser should appear registered.
	for _, bi := range report.Browsers {
		if bi.Registered {
			continue
		}
		// A non-registered entry can only appear if it was configured.
		if !bi.Configured {
			t.Errorf("browser %q is neither registered nor configured", bi.Name)
		}
	}
}

func TestReportBrowsers_ConfiguredBrowsers(t *testing.T) {
	cfg := &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome", "cloak"},
	}
	report := ReportBrowsers(context.Background(), cfg)

	if len(report.ConfiguredBrowsers) != 2 {
		t.Fatalf("ConfiguredBrowsers = %v, want [chrome cloak]", report.ConfiguredBrowsers)
	}

	for _, name := range []string{"chrome", "cloak"} {
		found := false
		for _, bi := range report.Browsers {
			if bi.Name == name {
				found = true
				if !bi.Configured {
					t.Errorf("browser %q should be configured", name)
				}
				if !bi.Registered {
					t.Errorf("browser %q should be registered", name)
				}
			}
		}
		if !found {
			t.Errorf("browser %q missing from report", name)
		}
	}
}

func TestReportBrowsers_DefaultBrowser(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser:    "cloak",
		BrowsersAvailable: []string{"chrome", "cloak"},
	}
	report := ReportBrowsers(context.Background(), cfg)

	if report.DefaultBrowser != "cloak" {
		t.Fatalf("DefaultBrowser = %q, want cloak", report.DefaultBrowser)
	}

	for _, bi := range report.Browsers {
		switch bi.Name {
		case "cloak":
			if !bi.IsDefault {
				t.Error("cloak should be marked as default")
			}
		default:
			if bi.IsDefault {
				t.Errorf("browser %q should not be marked as default", bi.Name)
			}
		}
	}
}

func TestReportBrowsers_StatusReady(t *testing.T) {
	cfg := &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
	}
	report := ReportBrowsers(context.Background(), cfg)

	var chromeInfo *BrowserInfo
	for i := range report.Browsers {
		if report.Browsers[i].Name == "chrome" {
			chromeInfo = &report.Browsers[i]
			break
		}
	}
	if chromeInfo == nil {
		t.Fatal("chrome not found in report")
	}
	if chromeInfo.Status != "ready" {
		t.Errorf("chrome Status = %q, want %q", chromeInfo.Status, "ready")
	}
	if chromeInfo.StatusDetail == "" {
		t.Error("chrome StatusDetail should not be empty")
	}
}

func TestReportBrowsers_StatusReadyGhostChrome(t *testing.T) {
	cfg := &config.RuntimeConfig{
		BrowsersAvailable: []string{"ghost-chrome"},
	}
	report := ReportBrowsers(context.Background(), cfg)

	var ghostInfo *BrowserInfo
	for i := range report.Browsers {
		if report.Browsers[i].Name == "ghost-chrome" {
			ghostInfo = &report.Browsers[i]
			break
		}
	}
	if ghostInfo == nil {
		t.Fatal("ghost-chrome not found in report")
	}
	if ghostInfo.Status != "ready" {
		t.Errorf("ghost-chrome Status = %q, want %q", ghostInfo.Status, "ready")
	}
	// ghost-chrome may get its detail from a passing check or from the
	// special-case fallback; either way it must not be empty.
	if ghostInfo.StatusDetail == "" {
		t.Error("ghost-chrome StatusDetail should not be empty")
	}
}

func TestReportBrowsers_StatusNotRegistered(t *testing.T) {
	cfg := &config.RuntimeConfig{
		BrowsersAvailable: []string{"not-a-real-browser-xyz"},
	}
	report := ReportBrowsers(context.Background(), cfg)

	var unknownInfo *BrowserInfo
	for i := range report.Browsers {
		if report.Browsers[i].Name == "not-a-real-browser-xyz" {
			unknownInfo = &report.Browsers[i]
			break
		}
	}
	if unknownInfo == nil {
		t.Fatal("not-a-real-browser-xyz not found in report")
	}
	if unknownInfo.Status != "missing" {
		t.Errorf("unknown browser Status = %q, want %q", unknownInfo.Status, "missing")
	}
	if unknownInfo.StatusDetail != "not a known browser" {
		t.Errorf("unknown browser StatusDetail = %q, want %q", unknownInfo.StatusDetail, "not a known browser")
	}
}

func TestDeriveBrowserStatus(t *testing.T) {
	tests := []struct {
		name       string
		info       BrowserInfo
		wantStatus string
		wantDetail string
	}{
		{
			name:       "not registered",
			info:       BrowserInfo{Name: "fake", Registered: false},
			wantStatus: "missing",
			wantDetail: "not a known browser",
		},
		{
			name: "all pass",
			info: BrowserInfo{
				Name:       "test-browser",
				Registered: true,
				Checks: []CheckResult{
					{Name: "check1", Status: StatusPass, Detail: "found at /usr/bin/test"},
				},
			},
			wantStatus: "ready",
			wantDetail: "found at /usr/bin/test",
		},
		{
			name: "has failure",
			info: BrowserInfo{
				Name:       "test-browser",
				Registered: true,
				Checks: []CheckResult{
					{Name: "check1", Status: StatusPass, Detail: "ok"},
					{Name: "check2", Status: StatusFail, Detail: "binary not found"},
				},
			},
			wantStatus: "missing",
			wantDetail: "binary not found",
		},
		{
			name: "has failure with errMsg fallback",
			info: BrowserInfo{
				Name:       "test-browser",
				Registered: true,
				Checks: []CheckResult{
					{Name: "check1", Status: StatusFail, ErrMsg: "exec: not found"},
				},
			},
			wantStatus: "missing",
			wantDetail: "exec: not found",
		},
		{
			name: "has warning",
			info: BrowserInfo{
				Name:       "test-browser",
				Registered: true,
				Checks: []CheckResult{
					{Name: "check1", Status: StatusPass, Detail: "ok"},
					{Name: "check2", Status: StatusWarn, Detail: "version outdated"},
				},
			},
			wantStatus: "needs-config",
			wantDetail: "version outdated",
		},
		{
			name: "ghost-chrome no detail",
			info: BrowserInfo{
				Name:       "ghost-chrome",
				Registered: true,
				Checks: []CheckResult{
					{Name: "check1", Status: StatusSkip},
				},
			},
			wantStatus: "ready",
			wantDetail: "ghost -> chrome",
		},
		{
			name: "registered no checks",
			info: BrowserInfo{
				Name:       "other",
				Registered: true,
			},
			wantStatus: "ready",
			wantDetail: "registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := tt.info
			deriveBrowserStatus(&info)
			if info.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", info.Status, tt.wantStatus)
			}
			if info.StatusDetail != tt.wantDetail {
				t.Errorf("StatusDetail = %q, want %q", info.StatusDetail, tt.wantDetail)
			}
		})
	}
}

func TestReportBrowsers_HandlesReported(t *testing.T) {
	cfg := &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome", "ghost-chrome"},
	}
	report := ReportBrowsers(context.Background(), cfg)

	var chromeInfo, ghostInfo *BrowserInfo
	for i := range report.Browsers {
		switch report.Browsers[i].Name {
		case "chrome":
			chromeInfo = &report.Browsers[i]
		case "ghost-chrome":
			ghostInfo = &report.Browsers[i]
		}
	}

	if chromeInfo == nil {
		t.Fatal("chrome not found in report")
	}
	if ghostInfo == nil {
		t.Fatal("ghost-chrome not found in report")
	}

	// Chrome handles all shapes.
	if len(chromeInfo.Handles) != len(allShapes) {
		t.Errorf("chrome handles %d shapes, want %d", len(chromeInfo.Handles), len(allShapes))
	}
	if len(chromeInfo.SkipsOrFails) != 0 {
		t.Errorf("chrome skipsOrFails = %v, want empty", chromeInfo.SkipsOrFails)
	}

	// Ghost-chrome only handles static shapes.
	if len(ghostInfo.Handles) < 1 {
		t.Fatal("ghost-chrome should handle at least one shape")
	}
	if len(ghostInfo.SkipsOrFails) < 1 {
		t.Fatal("ghost-chrome should skip at least one shape")
	}

	// Static shapes should be handled.
	for _, shape := range []string{"static-read", "static-snapshot"} {
		if !contains(ghostInfo.Handles, shape) {
			t.Errorf("ghost-chrome should handle %q", shape)
		}
	}
	// Interactive shapes should be skipped.
	for _, shape := range []string{"interaction", "session-state", "network-control", "download-upload"} {
		if !contains(ghostInfo.SkipsOrFails, shape) {
			t.Errorf("ghost-chrome should skip %q", shape)
		}
	}
}

func TestReportBrowsers_NeverModifiesConfig(t *testing.T) {
	cfg := &config.RuntimeConfig{
		BrowsersAvailable: []string{"chrome"},
		DefaultBrowser:    "chrome",
	}

	// Snapshot key fields before calling ReportBrowsers.
	origAvailable := make([]string, len(cfg.BrowsersAvailable))
	copy(origAvailable, cfg.BrowsersAvailable)
	origDefault := cfg.DefaultBrowser

	_ = ReportBrowsers(context.Background(), cfg)

	// BrowsersAvailable must be unchanged.
	if len(cfg.BrowsersAvailable) != len(origAvailable) {
		t.Fatalf("BrowsersAvailable length changed: %v -> %v", origAvailable, cfg.BrowsersAvailable)
	}
	for i, v := range cfg.BrowsersAvailable {
		if v != origAvailable[i] {
			t.Fatalf("BrowsersAvailable[%d] changed: %q -> %q", i, origAvailable[i], v)
		}
	}

	// DefaultBrowser must be unchanged.
	if cfg.DefaultBrowser != origDefault {
		t.Errorf("DefaultBrowser changed: %q -> %q", origDefault, cfg.DefaultBrowser)
	}
}
