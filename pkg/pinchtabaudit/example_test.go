package pinchtabaudit_test

import (
	"context"
	"fmt"
	"log"

	"github.com/pinchtab/pinchtab/pkg/pinchtabaudit"
)

// Enrich a single page through a running pinchtab server and inspect the
// browser-level data HTTP scraping cannot see.
func ExampleClient_EnrichPage() {
	client := pinchtabaudit.New("http://localhost:9867", "my-token")

	page, err := client.EnrichPage(context.Background(), "https://example.com", &pinchtabaudit.PageOptions{
		Screenshot: pinchtabaudit.Bool(false),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("accessibility %d/100, %d broken assets, load %.0f ms\n",
		page.AccessibilityScore, len(page.BrokenAssets), page.TimingMetrics.Load)
}

// Run a whole-site audit from a sitemap — the library form of
// `pinchtab audit --sitemap`.
func ExampleClient_EnrichWithBrowser() {
	client := pinchtabaudit.New("http://localhost:9867", "my-token")

	report, err := client.EnrichWithBrowser(context.Background(),
		pinchtabaudit.AuditInput{SitemapURL: "https://example.com/sitemap.xml"},
		&pinchtabaudit.RunOptions{Concurrency: 3, SampleSize: 2},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("audited %d pages, summary score %d/100\n", len(report.Pages), report.SummaryScore)
}
