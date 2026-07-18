package audit

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

var enrichTS = time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

func fullCollectors() Collectors {
	return Collectors{
		Title:      func() (string, error) { return "Fixture", nil },
		Screenshot: func() ([]byte, error) { return []byte{0x89, 0x50}, nil },
		Console: func() ([]bridge.LogEntry, error) {
			return []bridge.LogEntry{{Timestamp: enrichTS, Level: "error", Message: "boom", Source: "app.js"}}, nil
		},
		Network: func() ([]observe.NetworkEntry, error) {
			return []observe.NetworkEntry{
				{URL: "http://fixtures/page.html", Method: "GET", Status: 200, ResourceType: "Document", Finished: true},
				{URL: "http://fixtures/missing.png", Method: "GET", Status: 404, ResourceType: "Image", Finished: true},
			}, nil
		},
		Snapshot: func() ([]observe.A11yNode, error) {
			return []observe.A11yNode{
				{Ref: "e1", Role: "link", Name: "Home", Tag: "a"},
				{Ref: "e2", Role: "image"},
				{Role: "paragraph", Name: "text"},
				{Ref: "e3", Role: "button", Name: "Hidden", Hidden: true},
			}, nil
		},
		PageFacts: func() (PageFacts, error) {
			return PageFacts{Title: "Fixture", Lang: "en", HeadingLevels: []int{1}}, nil
		},
		Timing: func() (*observe.TimingMetrics, error) {
			return &observe.TimingMetrics{TTFBMs: 5, LoadMs: 120, FCPMs: 40, LCPMs: 80, CLS: 0.01, DOMContentLoadedMs: 90}, nil
		},
	}
}

func TestEnrichPageAllCollectors(t *testing.T) {
	pa := EnrichPage("http://fixtures/page.html", DefaultPageOptions(), fullCollectors())
	if pa.Error != "" {
		t.Fatalf("Error = %q, want empty", pa.Error)
	}
	if pa.Title != "Fixture" {
		t.Errorf("Title = %q", pa.Title)
	}
	if len(pa.ConsoleLogs) != 1 || pa.ConsoleLogs[0].Level != "error" {
		t.Errorf("ConsoleLogs = %+v", pa.ConsoleLogs)
	}
	if len(pa.NetworkRequests) != 2 {
		t.Errorf("NetworkRequests = %+v", pa.NetworkRequests)
	}
	if len(pa.BrokenAssets) != 1 || pa.BrokenAssets[0].URL != "http://fixtures/missing.png" || pa.BrokenAssets[0].Status != 404 {
		t.Errorf("BrokenAssets = %+v", pa.BrokenAssets)
	}
	if len(pa.InteractiveElements) != 1 || pa.InteractiveElements[0].Ref != "e1" {
		t.Errorf("InteractiveElements = %+v (want only the visible link)", pa.InteractiveElements)
	}
	if pa.AccessibilityScore != 90 {
		t.Errorf("AccessibilityScore = %d, want 90 (one missing-alt)", pa.AccessibilityScore)
	}
	if len(pa.A11yFindings) != 1 || pa.A11yFindings[0].Rule != "missing-alt" {
		t.Errorf("A11yFindings = %+v", pa.A11yFindings)
	}
	if pa.TimingMetrics.Load != 120 || pa.TimingMetrics.TimeToFirstByte != 5 {
		t.Errorf("TimingMetrics = %+v", pa.TimingMetrics)
	}
	if pa.Screenshot == "" {
		t.Error("Screenshot should be populated")
	}
}

func TestEnrichPageOptionToggles(t *testing.T) {
	opts := DefaultPageOptions()
	opts.Screenshot = false
	opts.Network = false
	pa := EnrichPage("http://fixtures/page.html", opts, fullCollectors())
	if pa.Screenshot != "" {
		t.Error("Screenshot should be empty when disabled")
	}
	if len(pa.NetworkRequests) != 0 {
		t.Errorf("NetworkRequests = %+v, want none", pa.NetworkRequests)
	}
	if len(pa.BrokenAssets) != 0 {
		t.Errorf("BrokenAssets = %+v, want none", pa.BrokenAssets)
	}

	data, err := json.Marshal(pa)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)
	for _, absent := range []string{"screenshot", "networkRequests", "brokenAssets"} {
		if _, ok := fields[absent]; ok {
			t.Errorf("field %q should be omitted when disabled", absent)
		}
	}
}

func TestEnrichPageCollectorFailureIsData(t *testing.T) {
	c := fullCollectors()
	c.Timing = func() (*observe.TimingMetrics, error) { return nil, errors.New("timing exploded") }
	pa := EnrichPage("http://fixtures/page.html", DefaultPageOptions(), c)
	if !strings.Contains(pa.Error, "timing: timing exploded") {
		t.Errorf("Error = %q, want timing failure recorded", pa.Error)
	}
	if len(pa.ConsoleLogs) != 1 {
		t.Error("other collectors should still populate")
	}
}

func TestNewPageAuditError(t *testing.T) {
	pa := NewPageAuditError("http://down.invalid/", errors.New("connection refused"))
	if pa.URL != "http://down.invalid/" || pa.Error != "connection refused" {
		t.Errorf("PageAudit = %+v", pa)
	}
	data, _ := json.Marshal(pa)
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)
	if fields["error"] != "connection refused" {
		t.Errorf("error JSON field = %v", fields["error"])
	}
}

func TestMapConsoleLogs(t *testing.T) {
	got := MapConsoleLogs([]bridge.LogEntry{{Timestamp: enrichTS, Level: "warn", Message: "careful", Source: "s"}})
	want := []ConsoleLogEntry{{Timestamp: enrichTS, Level: "warn", Message: "careful", Source: "s"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("MapConsoleLogs = %+v, want %+v", got, want)
	}
}

func TestMapNetworkRequestsMarksHTTPErrorsFailed(t *testing.T) {
	got := MapNetworkRequests([]observe.NetworkEntry{
		{URL: "a", Status: 200, Finished: true},
		{URL: "b", Status: 500, Finished: true},
		{URL: "c", Failed: true, Error: "net::ERR_FAILED"},
	})
	if got[0].Failed || !got[1].Failed || !got[2].Failed {
		t.Errorf("Failed flags = %v %v %v", got[0].Failed, got[1].Failed, got[2].Failed)
	}
	if got[2].Error != "net::ERR_FAILED" {
		t.Errorf("Error = %q", got[2].Error)
	}
}

func TestMapInteractiveElementsFiltersRolesAndHidden(t *testing.T) {
	got := MapInteractiveElements([]observe.A11yNode{
		{Ref: "e1", Role: "button", Name: "Go"},
		{Ref: "e2", Role: "paragraph", Name: "text"},
		{Ref: "e3", Role: "link", Name: "x", Hidden: true},
	})
	if len(got) != 1 || got[0].Ref != "e1" || !got[0].Visible {
		t.Errorf("MapInteractiveElements = %+v", got)
	}
}

func TestMapTimingMetrics(t *testing.T) {
	got := MapTimingMetrics(observe.TimingMetrics{TTFBMs: 1, FCPMs: 2, LCPMs: 3, CLS: 0.4, DOMContentLoadedMs: 5, LoadMs: 6})
	want := BrowserTimingMetrics{TimeToFirstByte: 1, FirstContentfulPaint: 2, LargestContentfulPaint: 3, CumulativeLayoutShift: 0.4, DOMContentLoaded: 5, Load: 6}
	if got != want {
		t.Errorf("MapTimingMetrics = %+v, want %+v", got, want)
	}
}
