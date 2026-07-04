package pinchtabaudit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnrichPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audit/page" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["url"] != "http://fixtures/clean.html" {
			t.Errorf("body url = %v", body["url"])
		}
		opts, _ := body["options"].(map[string]any)
		if opts["screenshot"] != false {
			t.Errorf("options = %v", opts)
		}
		_, _ = w.Write([]byte(`{
			"url": "http://fixtures/clean.html",
			"title": "Clean",
			"accessibilityScore": 100,
			"timingMetrics": {"ttfbMs": 3.5, "loadMs": 20},
			"interactiveElements": [{"ref": "e1", "role": "link", "name": "Home"}]
		}`))
	}))
	defer srv.Close()

	client := New(srv.URL, "tok")
	page, err := client.EnrichPage(context.Background(), "http://fixtures/clean.html", &PageOptions{Screenshot: Bool(false)})
	if err != nil {
		t.Fatalf("EnrichPage: %v", err)
	}
	if page.Title != "Clean" || page.AccessibilityScore != 100 || page.TimingMetrics.Load != 20 {
		t.Errorf("page = %+v", page)
	}
	if len(page.InteractiveElements) != 1 || page.InteractiveElements[0].Ref != "e1" {
		t.Errorf("elements = %+v", page.InteractiveElements)
	}
}

func TestEnrichWithBrowser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audit" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["sitemapUrl"] != "http://fixtures/sitemap.xml" || body["concurrency"] != float64(3) {
			t.Errorf("body = %v", body)
		}
		_, _ = w.Write([]byte(`{
			"schemaVersion": "1.0",
			"generatedAt": "2026-07-04T12:00:00Z",
			"pages": [{"url": "http://fixtures/a.html", "browser": {"timingMetrics": {}}}],
			"summaryScore": 100
		}`))
	}))
	defer srv.Close()

	report, err := New(srv.URL, "").EnrichWithBrowser(context.Background(),
		AuditInput{SitemapURL: "http://fixtures/sitemap.xml"},
		&RunOptions{Concurrency: 3})
	if err != nil {
		t.Fatalf("EnrichWithBrowser: %v", err)
	}
	if report.SchemaVersion != "1.0" || len(report.Pages) != 1 || report.SummaryScore != 100 {
		t.Errorf("report = %+v", report)
	}
}

func TestEnrichWithBrowserRequiresInput(t *testing.T) {
	if _, err := New("http://localhost:1", "").EnrichWithBrowser(context.Background(), AuditInput{}, nil); err == nil {
		t.Error("empty input should error before any request")
	}
}

func TestHTTPErrorsSurfaceBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"urls required"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "").EnrichPage(context.Background(), "http://x/", nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "urls required") {
		t.Errorf("err = %v, want HTTP 400 with body", err)
	}
}
