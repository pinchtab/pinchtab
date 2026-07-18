package scrape

import (
	"fmt"
	"strings"
)

// RenderMarkdown renders the report as a single Markdown digest: site
// overview, the page tree by URL pattern, then each page's content.
func RenderMarkdown(r Report) []byte {
	var b strings.Builder
	title := r.Site.Title
	if title == "" {
		title = r.Site.BaseURL
	}
	fmt.Fprintf(&b, "# Scrape Report — %s\n\n", title)
	fmt.Fprintf(&b, "- Base URL: %s\n", r.Site.BaseURL)
	fmt.Fprintf(&b, "- Pages: %d sampled of %d discovered\n", r.Site.SampledPages, r.Site.TotalDiscovered)
	fmt.Fprintf(&b, "- Sources: %d http · %d browser-rendered · %d failed\n",
		r.Summary.HTTPPages, r.Summary.BrowserPages, r.Summary.FailedPages)
	fmt.Fprintf(&b, "- Generated: %s\n\n", r.GeneratedAt.Format("2006-01-02 15:04:05 UTC"))

	if len(r.Summary.Recommendations) > 0 {
		b.WriteString("## Recommendations\n\n")
		for _, rec := range r.Summary.Recommendations {
			fmt.Fprintf(&b, "- %s\n", rec)
		}
		b.WriteString("\n")
	}

	if len(r.PageGroups) > 0 {
		b.WriteString("## Page Tree\n\n")
		for _, g := range r.PageGroups {
			fmt.Fprintf(&b, "- `%s` (%d sampled of %d)\n", g.Pattern, g.Sampled, g.Total)
			for _, u := range g.URLs {
				fmt.Fprintf(&b, "  - %s\n", u)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## Pages\n\n")
	for _, p := range r.Pages {
		heading := p.Title
		if heading == "" {
			heading = p.URL
		}
		fmt.Fprintf(&b, "### %s\n\n", heading)
		fmt.Fprintf(&b, "%s · source: %s", p.URL, p.Source)
		if p.StatusCode != 0 {
			fmt.Fprintf(&b, " · status: %d", p.StatusCode)
		}
		b.WriteString("\n\n")
		if p.Error != "" {
			fmt.Fprintf(&b, "> Failed: %s\n\n", p.Error)
			continue
		}
		if p.BrowserError != "" {
			fmt.Fprintf(&b, "> Browser enrichment failed (HTTP content kept): %s\n\n", p.BrowserError)
		}
		if md := strings.TrimSpace(p.Markdown); md != "" {
			b.WriteString(md)
			b.WriteString("\n\n")
		} else if p.Snippet != "" {
			// Preview: the body was withheld — show its size and a snippet.
			fmt.Fprintf(&b, "_Preview · %d chars_\n\n> %s\n\n", p.CharCount, p.Snippet)
		}
	}
	return []byte(b.String())
}
