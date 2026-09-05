package actions

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/scrape"
)

func TestScrapeListingMarksEveryPageTheSummaryCountsAsFailed(t *testing.T) {
	cases := []struct {
		name string
		page scrape.Page
		want string
	}{
		{"a 404 is marked", scrape.Page{StatusCode: 404, Source: scrape.SourceHTTP}, "failed: http 404"},
		{"a transport error keeps its wording", scrape.Page{Error: "tls handshake failure", Source: scrape.SourceHTTP}, "error: tls handshake failure"},
		{"a 200 is plain", scrape.Page{StatusCode: 200, Source: scrape.SourceHTTP}, "source: http"},
		{"a recovered 403 says so", scrape.Page{StatusCode: 403, Source: scrape.SourceBrowser}, "source: browser · http 403 recovered by the browser"},
		{"a browser failure rides along", scrape.Page{StatusCode: 200, Source: scrape.SourceHTTP, BrowserError: "tab crashed"}, "source: http · browser failed: tab crashed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scrapePageStatus(tc.page)
			if got != tc.want {
				t.Fatalf("scrapePageStatus = %q, want %q", got, tc.want)
			}
			if marked := got != "source: http" && got != "source: browser"; scrape.Failed(tc.page) && !marked {
				t.Fatalf("the summary counts this page as failed but the listing shows it plain: %q", got)
			}
		})
	}
}
